package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// newInviteLinkFixture creates the guild and user an InviteLink row references.
func newInviteLinkFixture(t *testing.T, db *gorm.DB) (domain.Guild, domain.User) {
	t.Helper()

	guild := domain.Guild{ID: uuid.New(), Name: "invite link schema test"}
	require.NoError(t, db.Create(&guild).Error)
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "invite link creator"}
	require.NoError(t, db.Create(&user).Error)

	return guild, user
}

func newInviteLink(guildID, userID uuid.UUID) *domain.InviteLink {
	return &domain.InviteLink{
		ID:              uuid.New(),
		GuildID:         guildID,
		Token:           strings.ReplaceAll(uuid.NewString(), "-", ""),
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
		CreatedByUserID: userID,
	}
}

// TestInviteLinkTableShape pins spec §2.2 for InviteLink against a real
// database: the table name, that the legacy guild_invite table is gone, the
// NOT NULL columns, the unique token index and the (guild_id, expires_at)
// index. AutoMigrate must also be a no-op the second time.
func TestInviteLinkTableShape(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	schema := requireCurrentSchema(t, db)

	require.NoError(t, db.WithContext(ctx).AutoMigrate(testutil.BaselineModels()...), "second AutoMigrate must be a no-op")

	var tables []string
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = ? AND table_name IN ('invite_link', 'guild_invite')`, schema).
		Scan(&tables).Error)
	assert.Equal(t, []string{"invite_link"}, tables, "only the spec table exists; the legacy guild_invite table must not be created")

	type column struct {
		ColumnName string
		IsNullable string
	}
	var columns []column
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT column_name, is_nullable FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = 'invite_link'`, schema).
		Scan(&columns).Error)
	nullable := map[string]string{}
	for _, c := range columns {
		nullable[c.ColumnName] = c.IsNullable
	}
	for _, name := range []string{"id", "guild_id", "token", "expires_at", "created_by_user_id"} {
		assert.Equal(t, "NO", nullable[name], "invite_link.%s must be NOT NULL", name)
	}
	assert.Contains(t, nullable, "created_at")
	assert.NotContains(t, nullable, "code", "the legacy code column must not exist")
	assert.NotContains(t, nullable, "ttl_seconds", "ttl_seconds is request-only and must not be a column")

	var indexDefs []string
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT indexdef FROM pg_indexes WHERE schemaname = ? AND tablename = 'invite_link'`, schema).
		Scan(&indexDefs).Error)
	var uniqueToken, guildExpires bool
	for _, def := range indexDefs {
		if strings.Contains(def, "UNIQUE") && strings.Contains(def, "(token)") {
			uniqueToken = true
		}
		if !strings.Contains(def, "UNIQUE") && strings.Contains(def, "(guild_id, expires_at)") {
			guildExpires = true
		}
	}
	assert.True(t, uniqueToken, "token must have a unique index: %v", indexDefs)
	assert.True(t, guildExpires, "(guild_id, expires_at) must be indexed: %v", indexDefs)
}

// TestInviteLinkForeignKeyConstraints verifies both foreign keys exist, point at
// the right tables and use ON UPDATE CASCADE / ON DELETE RESTRICT (our decision,
// see domain.InviteLink).
func TestInviteLinkForeignKeyConstraints(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	schema := requireCurrentSchema(t, db)

	type foreignKey struct {
		ColumnName   string
		RefTable     string
		RefColumn    string
		UpdateAction string
		DeleteAction string
	}
	var fks []foreignKey
	require.NoError(t, db.WithContext(context.Background()).Raw(
		`SELECT a.attname AS column_name, rt.relname AS ref_table, ra.attname AS ref_column,
		        c.confupdtype::text AS update_action, c.confdeltype::text AS delete_action
		 FROM pg_constraint c
		 JOIN pg_class t ON t.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 JOIN pg_class rt ON rt.oid = c.confrelid
		 JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		 JOIN pg_attribute ra ON ra.attrelid = c.confrelid AND ra.attnum = c.confkey[1]
		 WHERE c.contype = 'f' AND t.relname = 'invite_link' AND n.nspname = ?
		 ORDER BY a.attname`, schema).
		Scan(&fks).Error)

	// confupdtype 'c' = CASCADE, confdeltype 'r' = RESTRICT.
	assert.Equal(t, []foreignKey{
		{ColumnName: "created_by_user_id", RefTable: "user", RefColumn: "id", UpdateAction: "c", DeleteAction: "r"},
		{ColumnName: "guild_id", RefTable: "guild", RefColumn: "id", UpdateAction: "c", DeleteAction: "r"},
	}, fks)
}

// TestInviteLinkConstraintViolations triggers the constraints with real inserts
// and checks the errors reach callers as the domain sentinels.
func TestInviteLinkConstraintViolations(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, user := newInviteLinkFixture(t, db)

	first := newInviteLink(guild.ID, user.ID)
	require.NoError(t, db.WithContext(ctx).Create(first).Error)

	t.Run("duplicate token", func(t *testing.T) {
		duplicate := newInviteLink(guild.ID, user.ID)
		duplicate.Token = first.Token
		err := MapGormError(db.WithContext(ctx).Create(duplicate).Error)
		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
	})

	t.Run("unknown guild", func(t *testing.T) {
		err := MapGormError(db.WithContext(ctx).Create(newInviteLink(uuid.New(), user.ID)).Error)
		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
	})

	t.Run("unknown creator", func(t *testing.T) {
		err := MapGormError(db.WithContext(ctx).Create(newInviteLink(guild.ID, uuid.New())).Error)
		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
	})

	t.Run("expires_at is NOT NULL", func(t *testing.T) {
		err := db.WithContext(ctx).Exec(
			`INSERT INTO invite_link (id, guild_id, token, expires_at, created_by_user_id, created_at)
			 VALUES (?, ?, ?, NULL, ?, now())`,
			uuid.New(), guild.ID, strings.ReplaceAll(uuid.NewString(), "-", ""), user.ID).Error
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "want a pgx/v5 PgError, got %T: %v", err, err)
		assert.Equal(t, "23502", pgErr.Code, "not_null_violation")
		assert.Equal(t, "expires_at", pgErr.ColumnName)
	})

	t.Run("a guild with invite links cannot be hard-deleted", func(t *testing.T) {
		err := MapGormError(db.WithContext(ctx).Unscoped().Delete(&domain.Guild{}, "id = ?", guild.ID).Error)
		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
	})

	t.Run("a user who created invite links cannot be hard-deleted", func(t *testing.T) {
		rawErr := db.WithContext(ctx).Unscoped().Delete(&domain.User{}, "id = ?", user.ID).Error
		var pgErr *pgconn.PgError
		require.True(t, errors.As(rawErr, &pgErr), "want a pgx/v5 PgError, got %T: %v", rawErr, rawErr)
		assert.Equal(t, "23503", pgErr.Code, "foreign_key_violation")
		assert.NotEmpty(t, pgErr.ConstraintName)
		assert.ErrorIs(t, MapGormError(rawErr), domain.ErrHasRelatedRecords)

		var stored []domain.User
		require.NoError(t, db.WithContext(ctx).Unscoped().Where("id = ?", user.ID).Find(&stored).Error)
		require.Len(t, stored, 1, "the rejected hard delete must not remove the user row")
	})
}

// TestCreateInviteLinkWithTxErrors goes through the repository method instead
// of a raw insert, so it also proves the method wraps its errors in
// MapGormError and that a failed insert rolls the rotation back.
func TestCreateInviteLinkWithTxErrors(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, user := newInviteLinkFixture(t, db)
	repo := NewGormGuildRepository(db)

	now := time.Now()
	current := newInviteLink(guild.ID, user.ID)
	require.NoError(t, repo.CreateInviteLinkWithTx(ctx, guild.ID, current, now))

	requireCurrentLinkSurvives := func(t *testing.T) {
		t.Helper()
		active, err := repo.GetActiveInviteByGuildID(ctx, guild.ID, now)
		require.NoError(t, err)
		require.NotNil(t, active, "a failed rotation must not expire the current link")
		assert.Equal(t, current.Token, active.Token)
	}

	t.Run("unknown guild is not found at the rotation lock", func(t *testing.T) {
		missing := uuid.New()
		err := repo.CreateInviteLinkWithTx(ctx, missing, newInviteLink(missing, user.ID), now)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("duplicate token", func(t *testing.T) {
		duplicate := newInviteLink(guild.ID, user.ID)
		duplicate.Token = current.Token
		err := repo.CreateInviteLinkWithTx(ctx, guild.ID, duplicate, now)
		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		requireCurrentLinkSurvives(t)
	})

	t.Run("unknown creator", func(t *testing.T) {
		err := repo.CreateInviteLinkWithTx(ctx, guild.ID, newInviteLink(guild.ID, uuid.New()), now)
		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
		requireCurrentLinkSurvives(t)
	})
}

// TestCreateInviteLinkWithTxIgnoresPopulatedAssociations proves the Guild and
// CreatedByUser belongs-to fields on InviteLink never make the repository write
// to the parent tables or overwrite guild_id / created_by_user_id. Without
// Omit(clause.Associations) GORM upserts the referenced parents on Create.
func TestCreateInviteLinkWithTxIgnoresPopulatedAssociations(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, user := newInviteLinkFixture(t, db)

	ghostGuild := &domain.Guild{ID: uuid.New(), Name: "ghost guild"}
	ghostUser := &domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "ghost user"}

	link := newInviteLink(guild.ID, user.ID)
	link.Guild = ghostGuild
	link.CreatedByUser = ghostUser
	require.NoError(t, NewGormGuildRepository(db).CreateInviteLinkWithTx(ctx, guild.ID, link, time.Now()))

	var stored domain.InviteLink
	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", link.ID).Error)
	assert.Equal(t, guild.ID, stored.GuildID, "guild_id must not follow the association")
	assert.Equal(t, user.ID, stored.CreatedByUserID, "created_by_user_id must not follow the association")

	var ghostGuilds, ghostUsers int64
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.Guild{}).
		Where("id = ?", ghostGuild.ID).Count(&ghostGuilds).Error)
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.User{}).
		Where("id = ?", ghostUser.ID).Count(&ghostUsers).Error)
	assert.Zero(t, ghostGuilds, "a populated Guild association must not insert a parent row")
	assert.Zero(t, ghostUsers, "a populated CreatedByUser association must not insert a parent row")
}

// TestGetActiveInviteIgnoresExpiredLinks checks the repository query now that
// expires_at is NOT NULL: an expired link is never returned, the newest
// unexpired one is.
func TestGetActiveInviteIgnoresExpiredLinks(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, user := newInviteLinkFixture(t, db)
	repo := NewGormGuildRepository(db)

	now := time.Now()
	expired := newInviteLink(guild.ID, user.ID)
	expired.ExpiresAt = now.Add(-time.Minute)
	require.NoError(t, db.WithContext(ctx).Create(expired).Error)

	got, err := repo.GetActiveInviteByGuildID(ctx, guild.ID, now)
	require.NoError(t, err)
	assert.Nil(t, got, "an expired link is not active")

	active := newInviteLink(guild.ID, user.ID)
	require.NoError(t, db.WithContext(ctx).Create(active).Error)

	got, err = repo.GetActiveInviteByGuildID(ctx, guild.ID, now)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, active.Token, got.Token)
	assert.Equal(t, user.ID, got.CreatedByUserID)
}
