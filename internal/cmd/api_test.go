package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

func TestMigrateSchemaCreatesGuildApplicationConstraints(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}

	db, err := database.NewGormDB(databaseURL)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	ctx := context.Background()
	require.NoError(t, migrateSchema(ctx, db))
	require.True(t, db.WithContext(ctx).Migrator().HasTable(&domain.GuildApplication{}))
	require.True(t, db.WithContext(ctx).Migrator().HasTable(&domain.GuildDefaultSlot{}))

	var indexDefinition string
	err = db.WithContext(ctx).Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'guild_application' AND indexname = 'idx_guild_application_pending'`).Scan(&indexDefinition).Error
	require.NoError(t, err)
	require.Contains(t, indexDefinition, "UNIQUE INDEX")
	require.Contains(t, indexDefinition, "(guild_id, user_id)")
	require.Contains(t, strings.ToLower(indexDefinition), "where")
	require.Contains(t, indexDefinition, "pending")

	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	guildID, userID := uuid.New(), uuid.New()
	first := domain.GuildApplication{ID: uuid.New(), GuildID: guildID, UserID: userID, Status: domain.GuildApplicationStatusPending}
	require.NoError(t, tx.Create(&first).Error)

	duplicate := domain.GuildApplication{ID: uuid.New(), GuildID: guildID, UserID: userID, Status: domain.GuildApplicationStatusPending}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&duplicate)
	require.NoError(t, result.Error)
	require.Zero(t, result.RowsAffected)

	approved := domain.GuildApplication{ID: uuid.New(), GuildID: guildID, UserID: userID, Status: domain.GuildApplicationStatusApproved}
	require.NoError(t, tx.Create(&approved).Error)
}
