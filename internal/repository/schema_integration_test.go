package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// requireCurrentSchema returns the schema the pooled connection resolves
// unqualified names in. Every catalog assertion below is scoped to it, so a
// table or constraint from public (or from another test's schema) can never
// satisfy a check.
func requireCurrentSchema(t *testing.T, db *gorm.DB) string {
	t.Helper()

	var schema string
	require.NoError(t, db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&schema))
	require.NotEmpty(t, schema, "current_schema() returned an empty name")
	require.NotEqual(t, "public", schema, "integration tests must not run in public")

	return schema
}

// foreignKeyKeys renders the constraint inventory in a comparable form.
func foreignKeyKeys(fks []guildAttendeeForeignKey) []string {
	keys := make([]string, 0, len(fks))
	for _, fk := range fks {
		keys = append(keys, fmt.Sprintf("%s:%s->%s.%s(%s) upd=%s del=%s",
			fk.ConstraintName, fk.ColumnName,
			fk.ReferencedSchema, fk.ReferencedTable, fk.ReferencedColumn,
			fk.UpdateAction, fk.DeleteAction))
	}
	sort.Strings(keys)
	return keys
}

// TestGuildAttendeeForeignKeyConstraints pins the exact schema shape issue #69
// needs: one real foreign key per scalar column on guild_attendee, both ends in
// the per-test schema, ON UPDATE CASCADE / ON DELETE RESTRICT, and no
// duplicates when AutoMigrate runs a second time against the same schema.
func TestGuildAttendeeForeignKeyConstraints(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	schema := requireCurrentSchema(t, db)
	fks := listGuildAttendeeForeignKeys(t, db)

	for _, fk := range fks {
		t.Logf("fk %s on %s.%s.%s -> %s.%s.%s upd=%s del=%s",
			fk.ConstraintName,
			fk.TableSchema, fk.TableName, fk.ColumnName,
			fk.ReferencedSchema, fk.ReferencedTable, fk.ReferencedColumn,
			fk.UpdateAction, fk.DeleteAction)
	}
	require.Len(t, fks, 2,
		"guild_attendee must have exactly one foreign key per scalar column (guild_id, user_id), got %+v", fks)

	byColumn := make(map[string]guildAttendeeForeignKey, len(fks))
	for _, fk := range fks {
		assert.Equal(t, schema, fk.TableSchema, "constraint %s is not on this test's schema", fk.ConstraintName)
		assert.Equal(t, "guild_attendee", fk.TableName)
		assert.Equal(t, schema, fk.ReferencedSchema,
			"constraint %s must reference a table in %s, not %s",
			fk.ConstraintName, schema, fk.ReferencedSchema)
		require.NotContains(t, byColumn, fk.ColumnName,
			"column %s has more than one foreign key", fk.ColumnName)
		byColumn[fk.ColumnName] = fk
	}

	guildFK, ok := byColumn["guild_id"]
	require.True(t, ok, "guild_attendee.guild_id has no foreign key")
	assert.Equal(t, "guild", guildFK.ReferencedTable)
	assert.Equal(t, "id", guildFK.ReferencedColumn)
	assert.Equal(t, "c", guildFK.UpdateAction, "guild_id foreign key must be ON UPDATE CASCADE")
	assert.Equal(t, "r", guildFK.DeleteAction, "guild_id foreign key must be ON DELETE RESTRICT")

	userFK, ok := byColumn["user_id"]
	require.True(t, ok, "guild_attendee.user_id has no foreign key")
	assert.Equal(t, "user", userFK.ReferencedTable)
	assert.Equal(t, "id", userFK.ReferencedColumn)
	assert.Equal(t, "c", userFK.UpdateAction, "user_id foreign key must be ON UPDATE CASCADE")
	assert.Equal(t, "r", userFK.DeleteAction, "user_id foreign key must be ON DELETE RESTRICT")

	before := foreignKeyKeys(fks)

	// A second full AutoMigrate over the same schema must be idempotent: no new
	// constraints, no renames, no duplicates.
	require.NoError(t, db.WithContext(ctx).AutoMigrate(testutil.BaselineModels()...),
		"re-running the baseline AutoMigrate must succeed")
	assert.Equal(t, schema, requireCurrentSchema(t, db), "AutoMigrate must not change the search_path")

	after := foreignKeyKeys(listGuildAttendeeForeignKeys(t, db))
	require.Len(t, after, 2, "re-running AutoMigrate changed the foreign key count: %v", after)
	assert.Equal(t, before, after, "re-running AutoMigrate must not add, drop or rename foreign keys")
}

// TestGuildAttendeeIndexesPreserved proves adding the belongs-to associations
// did not disturb the pre-existing (guild_id, user_id) unique index.
func TestGuildAttendeeIndexesPreserved(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	schema := requireCurrentSchema(t, db)

	const query = `
SELECT i.relname AS index_name,
       pg_get_indexdef(x.indexrelid) AS index_def
FROM pg_index x
JOIN pg_class i ON i.oid = x.indexrelid
JOIN pg_class t ON t.oid = x.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relname = 'guild_attendee'
  AND n.nspname = current_schema()
ORDER BY i.relname`

	type indexRow struct {
		IndexName string `gorm:"column:index_name"`
		IndexDef  string `gorm:"column:index_def"`
	}

	var rows []indexRow
	require.NoError(t, db.WithContext(context.Background()).Raw(query).Scan(&rows).Error)
	require.NotEmpty(t, rows, "guild_attendee has no indexes")
	for _, row := range rows {
		t.Logf("schema=%s index=%s def=%s", schema, row.IndexName, row.IndexDef)
	}

	indexes := make(map[string]string, len(rows))
	for _, row := range rows {
		indexes[row.IndexName] = row.IndexDef
	}

	uniquePair, ok := indexes["idx_guild_attendee_guild_user"]
	require.True(t, ok, "missing unique index idx_guild_attendee_guild_user, got %v", indexes)
	assert.Contains(t, uniquePair, "UNIQUE")
	assert.Contains(t, uniquePair, "(guild_id, user_id)")

	// The retired leader-quota index must be gone, not merely unused.
	_, retired := indexes["idx_guild_attendee_active_master"]
	assert.False(t, retired, "idx_guild_attendee_active_master must be dropped, got %v", indexes)
}

// TestGuildAttendeeAssociationsAddNoColumns guards the other side of the change:
// declaring the relations must not materialise new database columns.
func TestGuildAttendeeAssociationsAddNoColumns(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	requireCurrentSchema(t, db)

	columnTypes, err := db.WithContext(context.Background()).Migrator().ColumnTypes(&domain.GuildAttendee{})
	require.NoError(t, err)

	names := make([]string, 0, len(columnTypes))
	for _, columnType := range columnTypes {
		names = append(names, columnType.Name())
	}
	sort.Strings(names)

	assert.Equal(t,
		[]string{"guild_id", "id", "joined_at", "left_at", "role", "user_id"},
		names,
		"the belongs-to associations must not add columns")
}

// TestGuildAttendeeAssociationsAreNotSerialized locks the wire contract: the
// new associations are invisible in JSON output and cannot be populated from
// JSON input.
func TestGuildAttendeeAssociationsAreNotSerialized(t *testing.T) {
	attendee := domain.GuildAttendee{
		ID:      uuid.New(),
		GuildID: uuid.New(),
		UserID:  uuid.New(),
		Role:    domain.GuildAttendeeRoleMember,
		Guild:   &domain.Guild{ID: uuid.New(), Name: "response-side guild payload"},
		User:    &domain.User{ID: uuid.New(), Email: "response-side-user@example.com", Name: "response-side user"},
	}

	payload, err := json.Marshal(attendee)
	require.NoError(t, err)
	t.Logf("marshalled attendee: %s", payload)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.NotContains(t, decoded, "guild", "Guild association must not be serialised")
	assert.NotContains(t, decoded, "user", "User association must not be serialised")
	assert.NotContains(t, string(payload), "response-side guild payload")
	assert.NotContains(t, string(payload), "response-side-user@example.com")

	expectedKeys := []string{"guild_attendee_id", "guild_id", "user_id", "role", "joined_at", "left_at"}
	for _, key := range expectedKeys {
		assert.Contains(t, decoded, key, "the existing JSON field %s must be preserved", key)
	}
	assert.Len(t, decoded, len(expectedKeys), "unexpected JSON fields: %v", decoded)

	// Input side: nested parent objects must be ignored, not decoded.
	input := fmt.Sprintf(`{
		"guild_attendee_id": "%s",
		"guild_id": "%s",
		"user_id": "%s",
		"role": "member",
		"guild": {"guild_id": "%s", "name": "request-side guild payload"},
		"user": {"user_id": "%s", "email": "request-side-user@example.com", "name": "request-side user"}
	}`, attendee.ID, attendee.GuildID, attendee.UserID, uuid.New(), uuid.New())

	var fromJSON domain.GuildAttendee
	require.NoError(t, json.Unmarshal([]byte(input), &fromJSON))
	assert.Nil(t, fromJSON.Guild, "Guild association must not be populated from JSON")
	assert.Nil(t, fromJSON.User, "User association must not be populated from JSON")
	assert.Equal(t, attendee.ID, fromJSON.ID)
	assert.Equal(t, attendee.GuildID, fromJSON.GuildID)
	assert.Equal(t, attendee.UserID, fromJSON.UserID)
}

// TestGuildAttendeeJSONCannotInjectParentWrites checks the practical
// consequence of the JSON guarantee: a hostile payload cannot smuggle a nested
// guild or user into a create and have GORM upsert it, because the associations
// stay nil and only the scalar foreign keys are written.
func TestGuildAttendeeJSONCannotInjectParentWrites(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	guild := domain.Guild{ID: uuid.New(), Name: "legit guild"}
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "legit user"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)

	injectedGuildID := uuid.New()
	injectedUserID := uuid.New()
	attendeeID := uuid.New()

	payload := fmt.Sprintf(`{
		"guild_attendee_id": "%s",
		"guild_id": "%s",
		"user_id": "%s",
		"role": "member",
		"guild": {"guild_id": "%s", "name": "injected guild"},
		"user": {"user_id": "%s", "email": "injected@example.com", "name": "injected user"}
	}`, attendeeID, guild.ID, user.ID, injectedGuildID, injectedUserID)

	var attendee domain.GuildAttendee
	require.NoError(t, json.Unmarshal([]byte(payload), &attendee))
	require.Nil(t, attendee.Guild)
	require.Nil(t, attendee.User)

	require.NoError(t, NewGormGuildAttendeeRepository(db).Create(ctx, &attendee))

	var injectedGuilds, injectedUsers int64
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.Guild{}).
		Where("id = ?", injectedGuildID).Count(&injectedGuilds).Error)
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.User{}).
		Where("id = ?", injectedUserID).Count(&injectedUsers).Error)
	assert.Zero(t, injectedGuilds, "a nested guild in the payload must not be persisted")
	assert.Zero(t, injectedUsers, "a nested user in the payload must not be persisted")

	var stored domain.GuildAttendee
	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", attendeeID).Error)
	assert.Equal(t, guild.ID, stored.GuildID)
	assert.Equal(t, user.ID, stored.UserID)
	assert.Nil(t, stored.User)
	// No Preload, so the association is never loaded either.
	assert.Nil(t, stored.Guild)

	var guilds, users int64
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.Guild{}).Count(&guilds).Error)
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.User{}).Count(&users).Error)
	assert.EqualValues(t, 1, guilds, "the create must not write extra guild rows")
	assert.EqualValues(t, 1, users, "the create must not write extra user rows")
}
