package cmd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

type legacyGuildDefaultSlot struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey"`
	GuildID   uuid.UUID      `gorm:"type:uuid;uniqueIndex"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (legacyGuildDefaultSlot) TableName() string { return "guild_default_slot" }

func TestMigrateSchemaClearsSoftDeletedDefaultSlots(t *testing.T) {
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

	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	require.NoError(t, tx.WithContext(ctx).AutoMigrate(&legacyGuildDefaultSlot{}))
	deletedAt := time.Now()
	legacy := legacyGuildDefaultSlot{
		ID: uuid.New(), GuildID: uuid.New(), DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true},
	}
	require.NoError(t, tx.WithContext(ctx).Create(&legacy).Error)
	active := legacyGuildDefaultSlot{ID: uuid.New(), GuildID: uuid.New()}
	require.NoError(t, tx.WithContext(ctx).Create(&active).Error)

	require.NoError(t, migrateSchema(ctx, tx))
	var remaining int64
	require.NoError(t, tx.WithContext(ctx).Unscoped().Model(&domain.GuildDefaultSlot{}).
		Where("id = ?", legacy.ID).Count(&remaining).Error)
	require.Zero(t, remaining)
	var activeCount int64
	require.NoError(t, tx.WithContext(ctx).Model(&domain.GuildDefaultSlot{}).
		Where("id = ?", active.ID).Count(&activeCount).Error)
	require.EqualValues(t, 1, activeCount)

	replacement := domain.GuildDefaultSlot{ID: uuid.New(), GuildID: legacy.GuildID}
	require.NoError(t, tx.WithContext(ctx).Create(&replacement).Error)
}
