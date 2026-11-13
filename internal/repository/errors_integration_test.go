package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// guildAttendeeForeignKey is one column-level row of the pg_constraint lookup
// in listGuildAttendeeForeignKeys.
type guildAttendeeForeignKey struct {
	ConstraintName   string `gorm:"column:constraint_name"`
	TableSchema      string `gorm:"column:table_schema"`
	TableName        string `gorm:"column:table_name"`
	ColumnName       string `gorm:"column:column_name"`
	ReferencedSchema string `gorm:"column:referenced_schema"`
	ReferencedTable  string `gorm:"column:referenced_table"`
	ReferencedColumn string `gorm:"column:referenced_column"`
	UpdateAction     string `gorm:"column:update_action"` // pg_constraint.confupdtype
	DeleteAction     string `gorm:"column:delete_action"` // pg_constraint.confdeltype
}

// listGuildAttendeeForeignKeys returns the FOREIGN KEY constraints that
// PostgreSQL actually created on guild_attendee (contype = 'f' in
// pg_constraint), one row per constrained column.
//
// The query is deliberately pinned to current_schema(): the integration helper
// gives every test its own schema, and an unscoped catalog lookup would happily
// report the foreign keys of a table left behind by another test (or by an
// earlier run) and hide a missing constraint. Both ends of the constraint must
// live in that same schema.
func listGuildAttendeeForeignKeys(t *testing.T, db *gorm.DB) []guildAttendeeForeignKey {
	t.Helper()

	const query = `
SELECT c.conname      AS constraint_name,
       tn.nspname     AS table_schema,
       t.relname      AS table_name,
       a.attname      AS column_name,
       fn.nspname     AS referenced_schema,
       fc.relname     AS referenced_table,
       fa.attname     AS referenced_column,
       c.confupdtype  AS update_action,
       c.confdeltype  AS delete_action
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace tn ON tn.oid = t.relnamespace
JOIN pg_class fc ON fc.oid = c.confrelid
JOIN pg_namespace fn ON fn.oid = fc.relnamespace
JOIN LATERAL unnest(c.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN LATERAL unnest(c.confkey) WITH ORDINALITY AS fk(attnum, ord) ON fk.ord = k.ord
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = fk.attnum
WHERE t.relname = 'guild_attendee'
  AND tn.nspname = current_schema()
  AND fn.nspname = current_schema()
  AND c.contype = 'f'
ORDER BY c.conname, k.ord`

	var fks []guildAttendeeForeignKey
	err := db.WithContext(context.Background()).Raw(query).Scan(&fks).Error
	require.NoError(t, err, "query pg_constraint for guild_attendee foreign keys")

	return fks
}

// requireForeignKey fails the test unless fks contains a foreign key from
// column to schema.referencedTable(referencedColumn). The schema is part of the
// match so a constraint pointing at another schema's table cannot satisfy it.
func requireForeignKey(t *testing.T, fks []guildAttendeeForeignKey, schema, column, referencedTable, referencedColumn string) {
	t.Helper()

	for _, fk := range fks {
		if fk.ColumnName == column &&
			fk.TableSchema == schema &&
			fk.ReferencedSchema == schema &&
			fk.ReferencedTable == referencedTable &&
			fk.ReferencedColumn == referencedColumn {
			return
		}
	}

	t.Errorf("guild_attendee is missing the foreign key %s.%s -> %s.%s(%s); constraints found: %+v",
		schema, column, schema, referencedTable, referencedColumn, fks)
}

// TestGuildAttendeeForeignKeys inspects the schema AutoMigrate actually
// produced. Issue #69 depends on guild_attendee.guild_id and
// guild_attendee.user_id being enforced by real PostgreSQL foreign keys.
func TestGuildAttendeeForeignKeys(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	var currentSchema string
	require.NoError(t, db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&currentSchema))

	fks := listGuildAttendeeForeignKeys(t, db)
	t.Logf("current_schema() = %s", currentSchema)
	for _, fk := range fks {
		t.Logf("fk %s on %s.%s.%s -> %s.%s.%s",
			fk.ConstraintName,
			fk.TableSchema, fk.TableName, fk.ColumnName,
			fk.ReferencedSchema, fk.ReferencedTable, fk.ReferencedColumn)
	}
	t.Logf("guild_attendee foreign key count: %d", len(fks))

	for _, fk := range fks {
		assert.Equal(t, currentSchema, fk.TableSchema,
			"guild_attendee must live in the per-test schema, not %s", fk.TableSchema)
		assert.Equal(t, currentSchema, fk.ReferencedSchema,
			"foreign key %s must reference a table in the per-test schema, not %s",
			fk.ConstraintName, fk.ReferencedSchema)
	}

	requireForeignKey(t, fks, currentSchema, "guild_id", "guild", "id")
	requireForeignKey(t, fks, currentSchema, "user_id", "user", "id")
}

// TestGuildAttendeeCreateWithMissingGuild checks the behaviour the error
// mapping in errors.go claims: inserting an attendee for a guild that does not
// exist must be rejected by the database and surface as
// domain.ErrHasRelatedRecords.
func TestGuildAttendeeCreateWithMissingGuild(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "guild attendee fk test",
	}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error, "create a valid user")

	repo := NewGormGuildAttendeeRepository(db)

	// Control: the same insert against an existing guild must succeed, so a
	// failure below can only be blamed on the missing guild.
	guild := domain.Guild{ID: uuid.New(), Name: "guild attendee fk test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error, "create a valid guild")

	valid := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: user.ID}
	require.NoError(t, repo.Create(ctx, &valid), "inserting an attendee for an existing guild must succeed")

	// Reproducer: nonexistent guild, otherwise valid user.
	orphan := domain.GuildAttendee{ID: uuid.New(), GuildID: uuid.New(), UserID: user.ID}
	err := repo.Create(ctx, &orphan)
	if err == nil {
		t.Fatalf("Create accepted an attendee for a nonexistent guild %s; "+
			"guild_attendee.guild_id has no enforced foreign key", orphan.GuildID)
	}
	assert.ErrorIs(t, err, domain.ErrHasRelatedRecords,
		"foreign key violation should map to domain.ErrHasRelatedRecords, got %v", err)
}

// TestGuildAttendeeCreateWithMissingUser is the user_id counterpart of
// TestGuildAttendeeCreateWithMissingGuild.
func TestGuildAttendeeCreateWithMissingUser(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	guild := domain.Guild{ID: uuid.New(), Name: "guild attendee fk test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error, "create a valid guild")

	repo := NewGormGuildAttendeeRepository(db)

	// Control: existing user, so a failure below can only be the missing user.
	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "guild attendee fk test",
	}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error, "create a valid user")

	valid := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: user.ID}
	require.NoError(t, repo.Create(ctx, &valid), "inserting an attendee for an existing user must succeed")

	// Reproducer: nonexistent user, otherwise valid guild.
	orphan := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: uuid.New()}
	err := repo.Create(ctx, &orphan)
	require.Error(t, err, "Create accepted an attendee for a nonexistent user %s", orphan.UserID)
	assert.ErrorIs(t, err, domain.ErrHasRelatedRecords,
		"foreign key violation should map to domain.ErrHasRelatedRecords, got %v", err)
}

// TestGuildAttendeeForeignKeyViolationIsPostgresError asserts the rejection is
// PostgreSQL's own SQLSTATE 23503 with the catalog's constraint name, and that
// MapGormError translates that raw driver error. Checking only the mapped
// domain error would pass even if the constraint were emulated in Go, so the
// raw pgconn.PgError is asserted first and the repository path is verified
// separately.
func TestGuildAttendeeForeignKeyViolationIsPostgresError(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	fks := listGuildAttendeeForeignKeys(t, db)
	var guildConstraint string
	for _, fk := range fks {
		if fk.ColumnName == "guild_id" {
			guildConstraint = fk.ConstraintName
		}
	}
	require.NotEmpty(t, guildConstraint, "guild_attendee.guild_id has no foreign key: %+v", fks)

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "raw fk error test",
	}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)

	// Raw GORM insert, bypassing the repository, to keep the driver error intact.
	orphan := domain.GuildAttendee{ID: uuid.New(), GuildID: uuid.New(), UserID: user.ID}
	rawErr := db.WithContext(ctx).Create(&orphan).Error
	require.Error(t, rawErr, "raw insert for a nonexistent guild must be rejected")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(rawErr, &pgErr), "want *pgconn.PgError, got %T: %v", rawErr, rawErr)
	assert.Equal(t, "23503", pgErr.Code, "want foreign_key_violation, got %s", pgErr.Code)
	assert.Equal(t, guildConstraint, pgErr.ConstraintName,
		"violated constraint should be the guild_attendee -> guild foreign key")
	t.Logf("raw driver error: code=%s constraint=%s schema=%s table=%s detail=%s",
		pgErr.Code, pgErr.ConstraintName, pgErr.SchemaName, pgErr.TableName, pgErr.Detail)

	// Same violation, mapped the way production code maps it.
	assert.ErrorIs(t, MapGormError(rawErr), domain.ErrHasRelatedRecords,
		"MapGormError must translate SQLSTATE 23503 to domain.ErrHasRelatedRecords")

	// And independently through the repository path.
	repo := NewGormGuildAttendeeRepository(db)
	repoOrphan := domain.GuildAttendee{ID: uuid.New(), GuildID: uuid.New(), UserID: user.ID}
	err := repo.Create(ctx, &repoOrphan)
	require.Error(t, err, "repository Create accepted an attendee for a nonexistent guild")
	assert.ErrorIs(t, err, domain.ErrHasRelatedRecords,
		"repository Create should return domain.ErrHasRelatedRecords, got %v", err)
}

// TestGuildAttendeeBlocksHardDeleteOfUser pins the ON DELETE RESTRICT half of
// the user_id foreign key: a hard delete of a referenced user must be refused
// and must leave the row fully intact (not even soft-deleted).
func TestGuildAttendeeBlocksHardDeleteOfUser(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "user deletion restrict test",
	}
	guild := domain.Guild{ID: uuid.New(), Name: "user deletion restrict test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)

	attendee := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: user.ID}
	require.NoError(t, NewGormGuildAttendeeRepository(db).Create(ctx, &attendee))

	// Raw DELETE first, bypassing the repository, so the refusal can only come
	// from the database's ON DELETE RESTRICT and not from Go-level checks.
	var userConstraint string
	for _, fk := range listGuildAttendeeForeignKeys(t, db) {
		if fk.ColumnName == "user_id" {
			userConstraint = fk.ConstraintName
		}
	}
	require.NotEmpty(t, userConstraint, "guild_attendee.user_id has no foreign key")

	rawErr := db.WithContext(ctx).Unscoped().Delete(&domain.User{}, "id = ?", user.ID).Error
	require.Error(t, rawErr, "raw hard delete of a referenced user must be rejected")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(rawErr, &pgErr), "want *pgconn.PgError, got %T: %v", rawErr, rawErr)
	assert.Equal(t, "23503", pgErr.Code, "want foreign_key_violation, got %s", pgErr.Code)
	assert.Equal(t, userConstraint, pgErr.ConstraintName,
		"violated constraint should be the guild_attendee -> user foreign key")
	assert.ErrorIs(t, MapGormError(rawErr), domain.ErrHasRelatedRecords,
		"MapGormError must translate SQLSTATE 23503 to domain.ErrHasRelatedRecords")

	// Unscoped() makes the repository issue a real DELETE, which only the
	// database-level ON DELETE RESTRICT can stop.
	hardDelete := NewGormUserRepository(db.Unscoped())
	err := hardDelete.Delete(ctx, user.ID)
	require.Error(t, err, "hard delete of a user referenced by guild_attendee must fail")
	assert.ErrorIs(t, err, domain.ErrHasRelatedRecords,
		"ON DELETE RESTRICT should map to domain.ErrHasRelatedRecords, got %v", err)

	var stored []domain.User
	require.NoError(t, db.WithContext(ctx).Unscoped().Where("id = ?", user.ID).Find(&stored).Error)
	require.Len(t, stored, 1, "the rejected hard delete must not remove the user row")
	assert.False(t, stored[0].DeletedAt.Valid, "the rejected hard delete must not soft-delete the user")

	var attendees int64
	require.NoError(t, db.WithContext(ctx).Model(&domain.GuildAttendee{}).
		Where("id = ?", attendee.ID).Count(&attendees).Error)
	assert.EqualValues(t, 1, attendees, "the referencing attendee must survive")
}

// TestUserRepositoryDeleteStaysSoft documents the scope of the new foreign key:
// the production repository still soft-deletes, so deleting a user that a
// guild_attendee references keeps working and never reaches SQLSTATE 23503.
func TestUserRepositoryDeleteStaysSoft(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "soft delete unchanged test",
	}
	guild := domain.Guild{ID: uuid.New(), Name: "soft delete unchanged test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)
	require.NoError(t, NewGormGuildAttendeeRepository(db).Create(ctx, &domain.GuildAttendee{
		ID: uuid.New(), GuildID: guild.ID, UserID: user.ID,
	}))

	require.NoError(t, NewGormUserRepository(db).Delete(ctx, user.ID),
		"soft delete of a referenced user must keep working")

	var stored domain.User
	require.NoError(t, db.WithContext(ctx).Unscoped().First(&stored, "id = ?", user.ID).Error,
		"soft delete must keep the row")
	assert.True(t, stored.DeletedAt.Valid, "soft delete must set deleted_at")

	assert.ErrorIs(t, db.WithContext(ctx).First(&domain.User{}, "id = ?", user.ID).Error,
		gorm.ErrRecordNotFound, "a soft-deleted user must stay invisible to normal queries")
}

// TestPostgresHelperConnectionSearchPath proves the helper's isolation claim:
// every connection the pool hands out resolves unqualified names inside the
// per-test schema, including when several queries run concurrently.
func TestPostgresHelperConnectionSearchPath(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	var expected string
	require.NoError(t, db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&expected))
	require.NotEmpty(t, expected)
	require.NotEqual(t, "public", expected, "test schema must not be public")

	const workers = 8
	schemas := make([]string, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&schemas[i])
		}(i)
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i], "concurrent current_schema() query %d", i)
		assert.Equal(t, expected, schemas[i], "connection %d used the wrong search_path", i)
	}
}

// TestGuildAttendeeBlocksHardDeleteOfGuild pins the ON DELETE RESTRICT half of
// the guild_id foreign key. GuildRepository.Delete refuses in Go with a count
// check before it soft-deletes, so only a raw hard delete exercises the
// database constraint itself.
func TestGuildAttendeeBlocksHardDeleteOfGuild(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "guild deletion restrict test",
	}
	guild := domain.Guild{ID: uuid.New(), Name: "guild deletion restrict test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)

	attendee := domain.GuildAttendee{ID: uuid.New(), GuildID: guild.ID, UserID: user.ID}
	require.NoError(t, NewGormGuildAttendeeRepository(db).Create(ctx, &attendee))

	var guildConstraint string
	for _, fk := range listGuildAttendeeForeignKeys(t, db) {
		if fk.ColumnName == "guild_id" {
			guildConstraint = fk.ConstraintName
		}
	}
	require.NotEmpty(t, guildConstraint, "guild_attendee.guild_id has no foreign key")

	rawErr := db.WithContext(ctx).Unscoped().Delete(&domain.Guild{}, "id = ?", guild.ID).Error
	require.Error(t, rawErr, "raw hard delete of a referenced guild must be rejected")

	var pgErr *pgconn.PgError
	require.True(t, errors.As(rawErr, &pgErr), "want *pgconn.PgError, got %T: %v", rawErr, rawErr)
	assert.Equal(t, "23503", pgErr.Code, "want foreign_key_violation, got %s", pgErr.Code)
	assert.Equal(t, guildConstraint, pgErr.ConstraintName,
		"violated constraint should be the guild_attendee -> guild foreign key")
	assert.ErrorIs(t, MapGormError(rawErr), domain.ErrHasRelatedRecords,
		"MapGormError must translate SQLSTATE 23503 to domain.ErrHasRelatedRecords")

	var stored []domain.Guild
	require.NoError(t, db.WithContext(ctx).Unscoped().Where("id = ?", guild.ID).Find(&stored).Error)
	require.Len(t, stored, 1, "the rejected hard delete must not remove the guild row")
	assert.False(t, stored[0].DeletedAt.Valid, "the rejected hard delete must not soft-delete the guild")

	var attendees int64
	require.NoError(t, db.WithContext(ctx).Model(&domain.GuildAttendee{}).
		Where("id = ?", attendee.ID).Count(&attendees).Error)
	assert.EqualValues(t, 1, attendees, "the membership row must survive the rejected delete")
}

// TestGuildAttendeeRepositoriesIgnorePopulatedAssociations proves in code, not
// by convention, that the Guild and User belongs-to fields on GuildAttendee
// never make a repository write to the parent tables. Without
// Omit(clause.Associations) GORM would upsert the referenced Guild and User on
// Create and Save.
func TestGuildAttendeeRepositoriesIgnorePopulatedAssociations(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "association test user"}
	guild := domain.Guild{ID: uuid.New(), Name: "association test guild"}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)

	requireParentsUntouched := func(t *testing.T, ghostUserID, ghostGuildID uuid.UUID) {
		t.Helper()

		var storedGuild domain.Guild
		require.NoError(t, db.WithContext(ctx).First(&storedGuild, "id = ?", guild.ID).Error)
		assert.Equal(t, guild.Name, storedGuild.Name, "a populated Guild association must not rewrite the guild")

		var storedUser domain.User
		require.NoError(t, db.WithContext(ctx).First(&storedUser, "id = ?", user.ID).Error)
		assert.Equal(t, user.Name, storedUser.Name, "a populated User association must not rewrite the user")

		var ghostUsers, ghostGuilds int64
		require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.User{}).
			Where("id = ?", ghostUserID).Count(&ghostUsers).Error)
		require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.Guild{}).
			Where("id = ?", ghostGuildID).Count(&ghostGuilds).Error)
		assert.Zero(t, ghostUsers, "a populated User association must not insert a parent row")
		assert.Zero(t, ghostGuilds, "a populated Guild association must not insert a parent row")
	}

	t.Run("attendee Create and Update", func(t *testing.T) {
		ghostUser := &domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "ghost user"}
		ghostGuild := &domain.Guild{ID: uuid.New(), Name: "ghost guild"}
		repo := NewGormGuildAttendeeRepository(db)

		attendee := domain.GuildAttendee{
			ID: uuid.New(), GuildID: guild.ID, UserID: user.ID,
			Guild: &domain.Guild{ID: guild.ID, Name: "renamed through the association"},
			User:  &domain.User{ID: user.ID, Email: user.Email, Name: "renamed through the association"},
		}
		require.NoError(t, repo.Create(ctx, &attendee))
		requireParentsUntouched(t, ghostUser.ID, ghostGuild.ID)

		attendee.Role = domain.GuildAttendeeRoleMaster
		attendee.Guild = ghostGuild
		attendee.User = ghostUser
		require.NoError(t, repo.Update(ctx, &attendee))
		requireParentsUntouched(t, ghostUser.ID, ghostGuild.ID)

		var stored domain.GuildAttendee
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", attendee.ID).Error)
		assert.Equal(t, domain.GuildAttendeeRoleMaster, stored.Role, "the scalar columns must still be written")
		assert.Equal(t, guild.ID, stored.GuildID, "guild_id must not follow the association")
		assert.Equal(t, user.ID, stored.UserID, "user_id must not follow the association")
	})

	t.Run("CreateWithMaster", func(t *testing.T) {
		master := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "association test master"}
		require.NoError(t, db.WithContext(ctx).Create(&master).Error)
		ghostUser := &domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "ghost user"}
		ghostGuild := &domain.Guild{ID: uuid.New(), Name: "ghost guild"}

		newGuild := domain.Guild{ID: uuid.New(), Name: "created with a master"}
		attendee := domain.GuildAttendee{UserID: master.ID, Guild: ghostGuild, User: ghostUser}
		require.NoError(t, NewGormGuildRepository(db).CreateWithMaster(ctx, &newGuild, &attendee))
		requireParentsUntouched(t, ghostUser.ID, ghostGuild.ID)

		var stored domain.GuildAttendee
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", attendee.ID).Error)
		assert.Equal(t, newGuild.ID, stored.GuildID)
		assert.Equal(t, master.ID, stored.UserID)
	})
}
