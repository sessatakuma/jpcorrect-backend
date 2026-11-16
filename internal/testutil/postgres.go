// Package testutil holds helpers shared by the backend's integration tests.
// It is test-only: no production package imports it.
package testutil

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

// TestDatabaseURLEnv is the only environment variable the helpers connect
// with. It is deliberately distinct from DATABASE_URL: an integration test must
// never be able to reach a runtime database, so there is no fallback.
const TestDatabaseURLEnv = "TEST_DATABASE_URL"

// BaselineModels mirrors the AutoMigrate calls in internal/cmd/api.go, in the
// same order (spec §2.3). Tests must run against the production schema, not a
// hand-rolled approximation of it, so keep the two lists in sync.
func BaselineModels() []any {
	return []any{
		&domain.User{},
		&domain.Guild{},
		&domain.GuildAttendee{},
		&domain.Topic{},
		&domain.ReportThemeSuggestion{},
		&domain.Activity{},
		&domain.AIAnalysisSubmission{},
		&domain.Event{},
		&domain.EventAttendee{},
		&domain.Transcript{},
		&domain.Mistake{},
		&domain.ActivityReporter{},
		&domain.GuildDefaultSlot{},
		&domain.InviteLink{},
		&domain.JoinRequest{},
		&domain.FeedbackProgress{},
		&domain.SelectedSentence{},
		&domain.ReviewRecord{},
		&domain.PracticeNote{},
		&domain.FeedPost{},
		&domain.FeedComment{},
		&domain.NotificationOutbox{},
		&domain.Notification{},
		&domain.PushSubscription{},
		&domain.ChatbotUsage{},
		&domain.RecordingSession{},
		&domain.RecordingAsset{},
	}
}

// NewPostgresDB returns a *gorm.DB connected to a freshly created, randomly
// named PostgreSQL schema that has been migrated with BaselineModels. The
// returned pool, and every connection it opens later, is bound to that schema
// via search_path.
//
// Isolation guarantees:
//   - every call creates its own schema, so tests (including parallel ones)
//     never share tables;
//   - search_path travels in the DSN, so each pooled connection resolves
//     unqualified names to the test schema — a query that grabs a different
//     connection from the pool still sees the right tables;
//   - cleanup closes the pool and drops only the schema this call created
//     (DROP SCHEMA ... CASCADE). public and any other schema are never touched.
//
// The test is skipped when TEST_DATABASE_URL is unset; DATABASE_URL is never
// used as a fallback.
func NewPostgresDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv(TestDatabaseURLEnv)
	if dsn == "" {
		t.Skipf("%s is not set, skipping PostgreSQL integration test", TestDatabaseURLEnv)
	}

	// Random per-test schema. UUIDs are hex + dashes, so the result is already
	// a valid unquoted identifier; Sanitize quotes it for the DDL anyway.
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()

	// Setup and teardown use context.Background(): t.Context() is already
	// cancelled by the time cleanup functions run.
	ctx := context.Background()

	// Admin pool: connected without a search_path override, used only for the
	// CREATE/DROP SCHEMA DDL of this test's schema.
	admin, err := database.NewGormDB(dsn)
	require.NoError(t, err, "connect to %s", TestDatabaseURLEnv)
	adminSQL, err := admin.DB()
	require.NoError(t, err)

	err = admin.WithContext(ctx).Exec("CREATE SCHEMA " + quotedSchema).Error
	require.NoError(t, err, "create test schema %s", schema)

	var db *gorm.DB
	t.Cleanup(func() {
		if db != nil {
			if sqlDB, err := db.DB(); err == nil {
				if err := sqlDB.Close(); err != nil {
					t.Errorf("close test pool for schema %s: %v", schema, err)
				}
			}
		}
		if err := admin.WithContext(ctx).Exec("DROP SCHEMA IF EXISTS " + quotedSchema + " CASCADE").Error; err != nil {
			t.Errorf("drop test schema %s: %v", schema, err)
		}
		if err := adminSQL.Close(); err != nil {
			t.Errorf("close admin pool for schema %s: %v", schema, err)
		}
	})

	scoped, err := database.NewGormDB(dsnWithSearchPath(t, dsn, schema))
	require.NoError(t, err, "connect to %s with search_path=%s", TestDatabaseURLEnv, schema)
	db = scoped

	err = db.WithContext(ctx).AutoMigrate(BaselineModels()...)
	require.NoError(t, err, "migrate baseline schema into %s", schema)

	// Mirrors internal/cmd/api.go: AutoMigrate runs first, then the explicit
	// drop of retired indexes. Tests must see the schema the server produces.
	require.NoError(t, database.DropLegacyIndexes(ctx, db), "drop legacy indexes in %s", schema)

	return db
}

// dsnWithSearchPath pins search_path in the DSN so pgx applies it to every
// connection the pool establishes.
func dsnWithSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()

	u, err := url.Parse(dsn)
	require.NoError(t, err, "%s must be a postgres:// URL", TestDatabaseURLEnv)
	require.Contains(t, []string{"postgres", "postgresql"}, u.Scheme,
		"%s must use the postgres:// or postgresql:// scheme", TestDatabaseURLEnv)

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	return u.String()
}
