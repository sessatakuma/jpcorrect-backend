package repository

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/testutil"
)

// specTables is the complete spec §2 schema, in the spec §2.3 migration order.
// Keep it next to BaselineModels: the two lists must be identical.
var specTables = []string{
	"user",
	"guild",
	"guild_attendee",
	"topic",
	"report_theme_suggestion",
	"activity",
	"ai_analysis_submission",
	"event",
	"event_attendee",
	"transcript",
	"mistake",
	"activity_reporter",
	"guild_default_slot",
	"invite_link",
	"join_request",
	"feedback_progress",
	"selected_sentence",
	"review_record",
	"practice_note",
	"feed_post",
	"feed_comment",
	"notification_outbox",
	"notification",
	"push_subscription",
	"chatbot_usage",
	"recording_session",
	"recording_asset",
}

// TestBaselineModelsMatchSpecOrder pins the migration list itself: it must be
// the 27 spec tables in the spec §2.3 order. Reordering it can break the
// foreign-key ordering that lets one AutoMigrate build an empty database.
func TestBaselineModelsMatchSpecOrder(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	models := testutil.BaselineModels()
	require.Len(t, models, len(specTables), "the migration list must hold all 27 spec tables")

	tables := make([]string, len(models))
	for i, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model), "parse migration model %T", model)
		tables[i] = stmt.Schema.Table
	}

	assert.Equal(t, specTables, tables, "the AutoMigrate list must be the spec §2.3 order")
}

// TestBaselineMigrationsAreForeignKeyOrdered proves the order really is usable
// on an empty database: every belongs-to target is migrated before the table
// that depends on it, which is what makes the single AutoMigrate call succeed.
func TestBaselineMigrationsAreForeignKeyOrdered(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	models := testutil.BaselineModels()
	positions := make(map[string]int, len(models))
	for i, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		positions[stmt.Schema.Table] = i
	}

	for i, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))

		for _, rel := range stmt.Schema.Relationships.BelongsTo {
			require.NotNil(t, rel.FieldSchema, "%s belongs-to relation has no target schema", rel.Name)
			target := rel.FieldSchema.Table

			pos, ok := positions[target]
			require.True(t, ok, "%s.%s references %s, which is not in the migration list",
				stmt.Schema.Table, rel.Name, target)
			assert.Less(t, pos, i, "%s.%s references %s, so %s must be migrated before %s",
				stmt.Schema.Table, rel.Name, target, target, stmt.Schema.Table)
		}
	}
}

// TestBaselineSchemaCreatesAllSpecTables checks the tables a fresh AutoMigrate
// actually created: exactly the 27 spec tables, no legacy guild_invite.
func TestBaselineSchemaCreatesAllSpecTables(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	schema := requireCurrentSchema(t, db)

	var tables []string
	require.NoError(t, db.WithContext(context.Background()).Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = ? AND table_type = 'BASE TABLE'`, schema).
		Scan(&tables).Error)

	sort.Strings(tables)
	want := append([]string(nil), specTables...)
	sort.Strings(want)
	assert.Equal(t, want, tables, "the migrated schema must be exactly the spec §2 tables")

	assert.NotContains(t, tables, "guild_invite", "the legacy guild_invite table must not be created")
	assert.NotContains(t, tables, "schema_migrations", "the schema has no migration bookkeeping table")
}

// TestBaselineAutoMigrateIsIdempotent runs the full AutoMigrate a second time
// and requires the resulting schema to be identical: no new table, constraint
// or index, and no error.
func TestBaselineAutoMigrateIsIdempotent(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	schema := requireCurrentSchema(t, db)

	before := schemaFingerprint(t, db, schema)

	require.NoError(t, db.WithContext(ctx).AutoMigrate(testutil.BaselineModels()...),
		"a second full AutoMigrate must succeed")
	assert.Equal(t, schema, requireCurrentSchema(t, db), "AutoMigrate must not change the search_path")

	assert.Equal(t, before, schemaFingerprint(t, db, schema),
		"a second full AutoMigrate must not add, drop or rename tables, constraints or indexes")
}

// schemaFingerprint renders every table, constraint and index of the schema in
// a comparable, sorted form.
func schemaFingerprint(t *testing.T, db *gorm.DB, schema string) []string {
	t.Helper()
	ctx := context.Background()

	var tables []string
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = ? AND table_type = 'BASE TABLE'`, schema).Scan(&tables).Error)

	var constraints []string
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT c.conname || ' ' || pg_get_constraintdef(c.oid) FROM pg_constraint c
		 JOIN pg_class t ON t.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE n.nspname = ?`, schema).Scan(&constraints).Error)

	var indexes []string
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT indexname || ' ' || regexp_replace(indexdef, 'test_[0-9a-f]{32}\.', '', 'g')
		 FROM pg_indexes WHERE schemaname = ?`, schema).Scan(&indexes).Error)

	fingerprint := make([]string, 0, len(tables)+len(constraints)+len(indexes))
	for _, table := range tables {
		fingerprint = append(fingerprint, "table "+table)
	}
	for _, constraint := range constraints {
		fingerprint = append(fingerprint, "constraint "+constraint)
	}
	for _, index := range indexes {
		fingerprint = append(fingerprint, "index "+index)
	}
	sort.Strings(fingerprint)

	return fingerprint
}

// TestLegacyActiveMasterIndexIsDropped simulates a database created before the
// leader-quota index was retired: it creates idx_guild_attendee_active_master by
// hand, runs the production migration and requires the index to be gone. The
// index tag is all that AutoMigrate sees, so removing the tag alone would leave
// the index in every existing database; internal/cmd/api.go therefore drops it
// explicitly (issue #72, spec §3.3.1 row 38).
func TestLegacyActiveMasterIndexIsDropped(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	schema := requireCurrentSchema(t, db)

	const legacyIndex = `CREATE UNIQUE INDEX idx_guild_attendee_active_master
		ON guild_attendee (user_id) WHERE role = 'master' AND left_at IS NULL`
	require.NoError(t, db.WithContext(ctx).Exec(legacyIndex).Error,
		"create the legacy index that an older release left behind")
	require.Contains(t, tableIndexes(t, db, schema, "guild_attendee"),
		"idx_guild_attendee_active_master", "the legacy index must exist before the migration")

	// The production order: AutoMigrate, then the explicit drop.
	require.NoError(t, db.WithContext(ctx).AutoMigrate(testutil.BaselineModels()...))
	require.NoError(t, database.DropLegacyIndexes(ctx, db))

	assert.NotContains(t, tableIndexes(t, db, schema, "guild_attendee"),
		"idx_guild_attendee_active_master",
		"the migration must drop the legacy leader-quota index")
	assert.Contains(t, tableIndexes(t, db, schema, "guild_attendee"),
		"idx_guild_attendee_guild_user",
		"the migration must keep the (guild_id, user_id) unique index")

	// A second run is a no-op: DROP INDEX IF EXISTS succeeds and the schema
	// fingerprint does not move.
	before := schemaFingerprint(t, db, schema)
	require.NoError(t, db.WithContext(ctx).AutoMigrate(testutil.BaselineModels()...),
		"a second migration run must succeed")
	require.NoError(t, database.DropLegacyIndexes(ctx, db),
		"dropping an absent legacy index must succeed")
	assert.Equal(t, before, schemaFingerprint(t, db, schema),
		"a second migration run must not change tables, constraints or indexes")
}
