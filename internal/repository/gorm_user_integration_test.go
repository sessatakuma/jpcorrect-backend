package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// TestCreateIfAbsentBySupabaseUserID pins the §6.3 idempotent insert against
// PostgreSQL: the first call creates the row, every later call leaves it alone,
// and a different account cannot take the same email.
func TestCreateIfAbsentBySupabaseUserID(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	repo := NewGormUserRepository(db)
	ctx := context.Background()

	sub := uuid.New()
	first := domain.User{ID: uuid.New(), Email: "first@example.com", Name: "first", SupabaseUserID: &sub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &first))

	stored, err := repo.GetBySupabaseUserID(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, first.ID, stored.ID)
	assert.Equal(t, "first@example.com", stored.Email)

	// A retry with a different candidate identity keeps the original row.
	retry := domain.User{ID: uuid.New(), Email: "retry@example.com", Name: "retry", SupabaseUserID: &sub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &retry))

	again, err := repo.GetBySupabaseUserID(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "the retry must not replace the row")
	assert.Equal(t, "first@example.com", again.Email)

	assert.Equal(t, int64(1), countRows(t, db, &domain.User{}, "supabase_user_id = ?", sub),
		"exactly one row per sub")
	assert.Equal(t, int64(0), countRows(t, db, &domain.User{}, "email = ?", "retry@example.com"),
		"the discarded candidate is not inserted either")

	// GetBySupabaseUserID answers ErrNotFound while the caller is unknown.
	_, err = repo.GetBySupabaseUserID(ctx, uuid.New())
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

// TestCreateIfAbsentBySupabaseUserIDEmailCollision is the defensive default of
// #73: the unique email index still holds, so a new sub claiming an existing
// email fails with domain.ErrDuplicateEntry instead of linking accounts.
func TestCreateIfAbsentBySupabaseUserIDEmailCollision(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	repo := NewGormUserRepository(db)
	ctx := context.Background()

	ownerSub := uuid.New()
	owner := domain.User{ID: uuid.New(), Email: "owner@example.com", SupabaseUserID: &ownerSub}
	require.NoError(t, repo.Create(ctx, &owner))

	newSub := uuid.New()
	claimant := domain.User{ID: uuid.New(), Email: "owner@example.com", SupabaseUserID: &newSub}

	err := repo.CreateIfAbsentBySupabaseUserID(ctx, &claimant)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrDuplicateEntry),
		"an email collision is a duplicate, not an internal error: %v", err)

	assert.Equal(t, int64(1), countRows(t, db, &domain.User{}, "email = ?", "owner@example.com"))
	assert.Equal(t, int64(0), countRows(t, db, &domain.User{}, "supabase_user_id = ?", newSub))

	stored, err := repo.GetByID(ctx, owner.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.SupabaseUserID)
	assert.Equal(t, ownerSub, *stored.SupabaseUserID, "the existing account is not relinked")
}

// TestCreateIfAbsentBySupabaseUserIDConcurrent hammers the idempotent insert
// from several connections at once: the unique index serializes them and every
// call succeeds against a single row.
func TestCreateIfAbsentBySupabaseUserIDConcurrent(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	// Keep the pool small so the calls really do contend on separate
	// connections instead of queueing inside the pool.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	repo := NewGormUserRepository(db)
	ctx := context.Background()
	sub := uuid.New()

	const calls = 20
	start := make(chan struct{})
	results := make(chan error, calls)
	var ready sync.WaitGroup
	ready.Add(calls)

	for range calls {
		go func() {
			ready.Done()
			<-start
			// Every caller proposes its own ID and email; the first one wins.
			candidate := domain.User{
				ID:             uuid.New(),
				Email:          uuid.NewString() + "@example.com",
				SupabaseUserID: &sub,
			}
			results <- repo.CreateIfAbsentBySupabaseUserID(ctx, &candidate)
		}()
	}

	ready.Wait()
	close(start)

	for range calls {
		require.NoError(t, <-results)
	}

	assert.Equal(t, int64(1), countRows(t, db, &domain.User{}, "supabase_user_id = ?", sub),
		"concurrent inserts must leave exactly one row")

	// The row is readable by sub, so a handler can return it after the insert.
	stored, err := repo.GetBySupabaseUserID(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, sub, *stored.SupabaseUserID)
}

// TestGetBySupabaseUserIDSkipsSoftDeleted documents the interaction the handler
// relies on: a soft-deleted account is invisible to the lookup, yet its unique
// supabase_user_id still blocks a replacement insert.
func TestGetBySupabaseUserIDSkipsSoftDeleted(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	repo := NewGormUserRepository(db)
	ctx := context.Background()

	sub := uuid.New()
	user := domain.User{ID: uuid.New(), Email: "gone@example.com", SupabaseUserID: &sub}
	require.NoError(t, repo.Create(ctx, &user))
	require.NoError(t, repo.Delete(ctx, user.ID))

	_, err := repo.GetBySupabaseUserID(ctx, sub)
	assert.True(t, errors.Is(err, domain.ErrNotFound))

	// The soft-deleted row still owns the sub, so re-initializing writes nothing.
	candidate := domain.User{ID: uuid.New(), Email: "gone@example.com", SupabaseUserID: &sub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &candidate))

	var total int64
	require.NoError(t, db.Unscoped().Model(&domain.User{}).Where("supabase_user_id = ?", sub).Count(&total).Error)
	assert.Equal(t, int64(1), total)
}

// newProfileFixture stores one fully populated user row and returns the
// repository and the values every test compares against.
func newProfileFixture(t *testing.T) (domain.UserRepository, *gorm.DB, *domain.User) {
	t.Helper()

	db := testutil.NewPostgresDB(t)
	repo := NewGormUserRepository(db)
	ctx := context.Background()

	sub := uuid.New()
	avatar := "https://example.com/avatar.png"
	user := domain.User{
		ID:              uuid.New(),
		Email:           "profile@example.com",
		Name:            "before",
		AvatarURL:       &avatar,
		IsEmailVerified: true,
		Role:            domain.UserRoleUser,
		Status:          domain.UserStatusActive,
		Timezone:        "Asia/Taipei",
		SupabaseUserID:  &sub,
		Points:          42,
		Level:           3,
	}
	require.NoError(t, repo.Create(ctx, &user))

	return repo, db, &user
}

// TestUpdateProfileWritesOnlyRequestedColumns is the column scope of the
// users/me write against PostgreSQL: name and avatar_url change, every other
// column keeps its stored value, and the returned row is the stored one.
func TestUpdateProfileWritesOnlyRequestedColumns(t *testing.T) {
	repo, _, seeded := newProfileFixture(t)
	ctx := context.Background()

	avatar := "https://example.com/new.png"
	name := "after"

	stored, err := repo.UpdateProfile(ctx, seeded.ID, &name, &avatar)
	require.NoError(t, err)

	require.NotNil(t, stored.AvatarURL)
	assert.Equal(t, name, stored.Name)
	assert.Equal(t, avatar, *stored.AvatarURL)

	// Server-owned columns are exactly as inserted, in the returned row and in
	// the table.
	reread, err := repo.GetByID(ctx, seeded.ID)
	require.NoError(t, err)
	for _, row := range []*domain.User{stored, reread} {
		assert.Equal(t, seeded.Email, row.Email)
		assert.Equal(t, seeded.Role, row.Role)
		assert.Equal(t, seeded.Status, row.Status)
		assert.Equal(t, seeded.Points, row.Points)
		assert.Equal(t, seeded.Level, row.Level)
		assert.Equal(t, seeded.Timezone, row.Timezone)
		assert.True(t, row.IsEmailVerified)
		require.NotNil(t, row.SupabaseUserID)
		assert.Equal(t, *seeded.SupabaseUserID, *row.SupabaseUserID)
		assert.Equal(t, seeded.ID, row.ID)
	}

	assert.True(t, stored.UpdatedAt.After(seeded.UpdatedAt),
		"a real write bumps updated_at: %v -> %v", seeded.UpdatedAt, stored.UpdatedAt)
}

// TestUpdateProfileLeavesConcurrentServerWrites pins the regression: a value
// another writer committed is still there after the owner renames themselves.
// A read-modify-save would restore the snapshot the request loaded and undo the
// promotion, the ban, the points award and the email change.
func TestUpdateProfileLeavesConcurrentServerWrites(t *testing.T) {
	repo, _, seeded := newProfileFixture(t)
	ctx := context.Background()

	// Another request commits first: a promotion, a ban, a points award and an
	// email change, all server-owned columns.
	concurrent := *seeded
	concurrent.Role = domain.UserRoleStaff
	concurrent.Status = domain.UserStatusSuspended
	concurrent.Points = 777
	concurrent.Email = "moved@example.com"
	require.NoError(t, repo.Update(ctx, &concurrent))

	name := "renamed"
	stored, err := repo.UpdateProfile(ctx, seeded.ID, &name, nil)
	require.NoError(t, err)

	reread, err := repo.GetByID(ctx, seeded.ID)
	require.NoError(t, err)
	for _, row := range []*domain.User{stored, reread} {
		assert.Equal(t, "renamed", row.Name)
		assert.Equal(t, domain.UserRoleStaff, row.Role, "the promotion survives")
		assert.Equal(t, domain.UserStatusSuspended, row.Status, "the suspension survives")
		assert.Equal(t, 777, row.Points, "the points award survives")
		assert.Equal(t, "moved@example.com", row.Email, "the email change survives")
	}
}

// TestUpdateProfileNilLeavesColumnAlone is the allowlist half of the contract:
// a nil pointer is an omission, not a write, so a single-field body cannot
// clear or reset the other column.
func TestUpdateProfileNilLeavesColumnAlone(t *testing.T) {
	repo, _, seeded := newProfileFixture(t)
	ctx := context.Background()

	// No field at all: nothing is written and the row is returned as stored.
	stored, err := repo.UpdateProfile(ctx, seeded.ID, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, seeded.Name, stored.Name)
	assert.Equal(t, seeded.UpdatedAt.Unix(), stored.UpdatedAt.Unix(), "an empty request must not bump updated_at")

	// Name only: the avatar keeps its stored value.
	name := "renamed"
	stored, err = repo.UpdateProfile(ctx, seeded.ID, &name, nil)
	require.NoError(t, err)
	require.NotNil(t, stored.AvatarURL)
	assert.Equal(t, *seeded.AvatarURL, *stored.AvatarURL)

	// An empty avatar_url clears the column to the empty string, the same value
	// the previous read-modify-save path wrote.
	empty := ""
	stored, err = repo.UpdateProfile(ctx, seeded.ID, nil, &empty)
	require.NoError(t, err)
	require.NotNil(t, stored.AvatarURL)
	assert.Empty(t, *stored.AvatarURL)
	assert.Equal(t, "renamed", stored.Name)
}

// TestUpdateProfileUnknownOrDeletedRow answers ErrNotFound and writes nothing:
// a soft-deleted account cannot be brought back by writing its profile.
func TestUpdateProfileUnknownOrDeletedRow(t *testing.T) {
	repo, db, seeded := newProfileFixture(t)
	ctx := context.Background()

	name := "renamed"

	_, err := repo.UpdateProfile(ctx, uuid.New(), &name, nil)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "unknown id: %v", err)

	_, err = repo.UpdateProfile(ctx, uuid.Nil, &name, nil)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "nil id: %v", err)

	require.NoError(t, repo.Delete(ctx, seeded.ID))
	_, err = repo.UpdateProfile(ctx, seeded.ID, &name, nil)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "soft-deleted row: %v", err)

	// The refused write never landed: the soft-deleted row still holds its name.
	var deleted domain.User
	require.NoError(t, db.Unscoped().Where("id = ?", seeded.ID).First(&deleted).Error)
	assert.Equal(t, seeded.Name, deleted.Name, "a refused write must not touch a soft-deleted row")
}
