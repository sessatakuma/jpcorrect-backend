package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

func TestGuildDefaultSlotRecreateAfterDelete(t *testing.T) {
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
	require.NoError(t, db.WithContext(ctx).AutoMigrate(&domain.Guild{}, &domain.GuildDefaultSlot{}))

	guild := domain.Guild{ID: uuid.New(), Name: "default slot recreation test"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)
	t.Cleanup(func() {
		require.NoError(t, db.WithContext(ctx).Unscoped().Where("guild_id = ?", guild.ID).
			Delete(&domain.GuildDefaultSlot{}).Error)
		require.NoError(t, db.WithContext(ctx).Unscoped().Delete(&guild).Error)
	})

	repo := NewGuildDefaultSlotRepository(db)
	first := &domain.GuildDefaultSlot{
		GuildID: guild.ID, DayOfWeek: 1, StartTime: "10:00", EndTime: "11:00",
	}
	require.NoError(t, repo.Upsert(ctx, first))
	require.NoError(t, repo.DeleteByGuildID(ctx, guild.ID))

	missing, err := repo.GetByGuildID(ctx, guild.ID)
	require.Nil(t, missing)
	require.ErrorIs(t, err, domain.ErrNotFound)
	var remaining int64
	require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.GuildDefaultSlot{}).
		Where("guild_id = ?", guild.ID).Count(&remaining).Error)
	require.Zero(t, remaining)

	replacement := &domain.GuildDefaultSlot{
		GuildID: guild.ID, DayOfWeek: 2, StartTime: "12:00", EndTime: "13:00",
	}
	require.NoError(t, repo.Upsert(ctx, replacement))
	stored, err := repo.GetByGuildID(ctx, guild.ID)
	require.NoError(t, err)
	require.Equal(t, replacement.ID, stored.ID)
	require.NotEqual(t, first.ID, stored.ID)
	require.Equal(t, replacement.DayOfWeek, stored.DayOfWeek)
	require.Equal(t, replacement.StartTime, stored.StartTime)
	require.Equal(t, replacement.EndTime, stored.EndTime)
}
