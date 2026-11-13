package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func databaseURL(t *testing.T) string {
	t.Helper()

	value := os.Getenv("DATABASE_URL")
	if value == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}
	return value
}

// openIntegrationDB connects, runs the migration the server runs at startup and
// closes the pool when the test ends.
func openIntegrationDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()

	db, err := database.NewGormDB(dsn)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })

	return db
}

// withSearchPath points dsn at one schema, so a test can create and drop a
// private copy of the table without touching the shared schema.
func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	require.NoError(t, err)

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

// legacyUserDDL is the `user` table as `main` created it before the Supabase
// mapping existed: unique email, soft delete, no supabase_user_id.
var legacyUserDDL = []string{
	`CREATE TABLE "user" (
		id uuid PRIMARY KEY,
		email text,
		name text,
		avatar_url text,
		password_hash text,
		is_email_verified boolean DEFAULT false,
		role text DEFAULT 'user',
		status text DEFAULT 'active',
		timezone text DEFAULT 'Asia/Taipei',
		late_streak bigint DEFAULT 0,
		points bigint DEFAULT 0,
		level bigint DEFAULT 0,
		created_at timestamptz,
		updated_at timestamptz,
		deleted_at timestamptz
	)`,
	`CREATE UNIQUE INDEX idx_user_email ON "user"(email)`,
	`CREATE INDEX idx_user_deleted_at ON "user"(deleted_at)`,
}

// TestAutoMigrateKeepsPopulatedLegacyUsers proves the startup migration of this
// change on a table that already holds rows: AutoMigrate adds a nullable uuid
// column and a unique index, the populated rows stay readable, and none of them
// is linked to a Supabase subject.
func TestAutoMigrateKeepsPopulatedLegacyUsers(t *testing.T) {
	dsn := databaseURL(t)
	ctx := context.Background()

	// A private schema keeps the shared one intact; the name is derived from a
	// UUID because it must be a valid identifier.
	schema := "legacy_user_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	admin := openIntegrationDB(t, dsn)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() {
		assert.NoError(t, admin.Exec(`DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`).Error)
	})

	db := openIntegrationDB(t, withSearchPath(t, dsn, schema))

	for _, statement := range legacyUserDDL {
		require.NoError(t, db.Exec(statement).Error)
	}

	legacyOne := uuid.New()
	legacyTwo := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO "user" (id, email, name) VALUES (?, ?, ?)`,
		legacyOne, "legacy-one@example.com", "legacy one",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO "user" (id, email, name) VALUES (?, ?, ?)`,
		legacyTwo, "legacy-two@example.com", "legacy two",
	).Error)

	require.NoError(t, db.AutoMigrate(&domain.User{}),
		"AutoMigrate must work on a populated legacy table")

	var column struct {
		DataType   string
		IsNullable string
	}
	require.NoError(t, db.Raw(
		`SELECT data_type, is_nullable FROM information_schema.columns
		WHERE table_schema = ? AND table_name = 'user' AND column_name = 'supabase_user_id'`,
		schema,
	).Scan(&column).Error)
	assert.Equal(t, "uuid", column.DataType, "supabase_user_id must be a uuid column")
	assert.Equal(t, "YES", column.IsNullable,
		"supabase_user_id must stay nullable: legacy rows have no Supabase subject")

	repo := NewGormUserRepository(db)

	for _, id := range []uuid.UUID{legacyOne, legacyTwo} {
		user, err := repo.GetByID(ctx, id)
		require.NoError(t, err, "a populated legacy row must stay readable after AutoMigrate")
		assert.Nil(t, user.SupabaseUserID, "AutoMigrate must not invent a Supabase mapping")
	}

	// The unique index exists and is usable: one row per subject in the
	// migrated schema.
	freshSub := uuid.New()
	fresh := domain.User{ID: uuid.New(), Email: "fresh@example.com", SupabaseUserID: &freshSub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &fresh))

	readBack, err := repo.GetBySupabaseUserID(ctx, freshSub)
	require.NoError(t, err)
	assert.Equal(t, fresh.ID, readBack.ID)

	// A new subject whose email belongs to a legacy row is refused, and the
	// legacy row keeps its empty mapping: no account takeover through email.
	claimantSub := uuid.New()
	claimant := domain.User{ID: uuid.New(), Email: "legacy-one@example.com", SupabaseUserID: &claimantSub}
	err = repo.CreateIfAbsentBySupabaseUserID(ctx, &claimant)
	assert.ErrorIs(t, err, domain.ErrDuplicateEntry)

	legacy, err := repo.GetByID(ctx, legacyOne)
	require.NoError(t, err)
	assert.Equal(t, legacyOne, legacy.ID)
	assert.Nil(t, legacy.SupabaseUserID, "a matching email must never link the legacy account")

	_, err = repo.GetBySupabaseUserID(ctx, claimantSub)
	assert.ErrorIs(t, err, domain.ErrNotFound, "the refused subject owns no account")

	// A non-pointer subject field silently reads NULL as the zero UUID, which is
	// why the model keeps the column a pointer and reports an unlinked legacy
	// row as nil instead of as a made-up subject.
	var strict strictSupabaseUser
	require.NoError(t, db.First(&strict, "id = ?", legacyOne).Error)
	assert.Equal(t, uuid.Nil, strict.SupabaseUserID,
		"a non-pointer field flattens NULL to the zero UUID")
}

// strictSupabaseUser is the same table read through a non-pointer subject field.
// It exists only to document why *uuid.UUID is the honest representation.
type strictSupabaseUser struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	SupabaseUserID uuid.UUID `gorm:"type:uuid;column:supabase_user_id"`
}

func (strictSupabaseUser) TableName() string { return "user" }

// TestCreateIfAbsentBySupabaseUserIDIsIdempotent pins the first-login sequence:
// the insert keeps the first row and every retry reads that same row back.
func TestCreateIfAbsentBySupabaseUserIDIsIdempotent(t *testing.T) {
	db := openIntegrationDB(t, databaseURL(t))
	require.NoError(t, db.AutoMigrate(&domain.User{}))

	repo := NewGormUserRepository(db)
	ctx := context.Background()
	sub := uuid.New()

	first := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", SupabaseUserID: &sub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &first))
	t.Cleanup(func() {
		assert.NoError(t, db.Unscoped().Delete(&domain.User{}, "supabase_user_id = ?", sub).Error)
	})

	// A retry proposes a different row; the conflict on the subject wins.
	retrySub := sub
	retry := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", SupabaseUserID: &retrySub}
	require.NoError(t, repo.CreateIfAbsentBySupabaseUserID(ctx, &retry))

	stored, err := repo.GetBySupabaseUserID(ctx, sub)
	require.NoError(t, err)
	assert.Equal(t, first.ID, stored.ID, "the retry must not replace the account")

	var count int64
	require.NoError(t, db.Model(&domain.User{}).Where("supabase_user_id = ?", sub).Count(&count).Error)
	assert.EqualValues(t, 1, count, "one account per Supabase subject")
}

// TestCreateIfAbsentBySupabaseUserIDConcurrent runs the first login from many
// connections at once: the unique index serializes the inserts, no caller sees
// an error, and exactly one row survives.
func TestCreateIfAbsentBySupabaseUserIDConcurrent(t *testing.T) {
	db := openIntegrationDB(t, databaseURL(t))
	require.NoError(t, db.AutoMigrate(&domain.User{}))

	// A small pool forces the calls to contend on separate connections.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	repo := NewGormUserRepository(db)
	ctx := context.Background()
	sub := uuid.New()

	t.Cleanup(func() {
		assert.NoError(t, db.Unscoped().Delete(&domain.User{}, "supabase_user_id = ?", sub).Error)
	})

	const calls = 20
	start := make(chan struct{})
	results := make(chan error, calls)
	var ready sync.WaitGroup
	ready.Add(calls)

	for range calls {
		go func() {
			ready.Done()
			<-start
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

	var count int64
	require.NoError(t, db.Model(&domain.User{}).Where("supabase_user_id = ?", sub).Count(&count).Error)
	assert.EqualValues(t, 1, count, "concurrent first logins must leave exactly one account")

	stored, err := repo.GetBySupabaseUserID(ctx, sub)
	require.NoError(t, err)
	require.NotNil(t, stored.SupabaseUserID)
	assert.Equal(t, sub, *stored.SupabaseUserID)
}

// TestGetBySupabaseUserIDMatchesNoLegacyRow documents what an unlinked legacy
// account looks like to the lookup: it is never returned for any subject.
func TestGetBySupabaseUserIDMatchesNoLegacyRow(t *testing.T) {
	db := openIntegrationDB(t, databaseURL(t))
	require.NoError(t, db.AutoMigrate(&domain.User{}))

	repo := NewGormUserRepository(db)
	ctx := context.Background()

	legacy := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com"}
	require.NoError(t, repo.Create(ctx, &legacy))
	t.Cleanup(func() {
		assert.NoError(t, db.Unscoped().Delete(&domain.User{}, "id = ?", legacy.ID).Error)
	})

	_, err := repo.GetBySupabaseUserID(ctx, uuid.New())
	assert.True(t, errors.Is(err, domain.ErrNotFound))

	stored, err := repo.GetByID(ctx, legacy.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.SupabaseUserID)
}
