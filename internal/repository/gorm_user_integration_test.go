package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
