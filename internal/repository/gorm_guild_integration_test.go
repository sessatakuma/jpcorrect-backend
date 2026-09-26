package repository

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

func TestCreateWithMasterConcurrentLimit(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL integration test")
	}

	db, err := database.NewGormDB(databaseURL)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&domain.User{},
		&domain.Guild{},
		&domain.GuildAttendee{},
	))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() {
		assert.NoError(t, sqlDB.Close())
	})

	user := domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "concurrent guild master test",
	}
	require.NoError(t, db.Create(&user).Error)

	guilds := []domain.Guild{
		{ID: uuid.New(), Name: "concurrent guild one"},
		{ID: uuid.New(), Name: "concurrent guild two"},
	}
	attendees := []domain.GuildAttendee{
		{ID: uuid.New(), UserID: user.ID},
		{ID: uuid.New(), UserID: user.ID},
	}

	t.Cleanup(func() {
		assert.NoError(t, db.Where(
			"id IN ?",
			[]uuid.UUID{attendees[0].ID, attendees[1].ID},
		).Delete(&domain.GuildAttendee{}).Error)
		assert.NoError(t, db.Unscoped().Where(
			"id IN ?",
			[]uuid.UUID{guilds[0].ID, guilds[1].ID},
		).Delete(&domain.Guild{}).Error)
		assert.NoError(t, db.Unscoped().Delete(&domain.User{}, "id = ?", user.ID).Error)
	})

	repo := NewGormGuildRepository(db)
	start := make(chan struct{})
	results := make(chan error, len(guilds))
	var ready sync.WaitGroup
	ready.Add(len(guilds))

	for i := range guilds {
		go func(i int) {
			ready.Done()
			<-start
			results <- repo.CreateWithMaster(
				context.Background(),
				&guilds[i],
				&attendees[i],
			)
		}(i)
	}

	ready.Wait()
	close(start)

	var succeeded, limitReached int
	for range guilds {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrGuildLimitReached):
			limitReached++
		default:
			t.Fatalf("unexpected CreateWithMaster error: %v", err)
		}
	}

	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, limitReached)

	var activeMasterCount int64
	require.NoError(t, db.Model(&domain.GuildAttendee{}).
		Where("user_id = ? AND role = ? AND left_at IS NULL", user.ID, domain.GuildAttendeeRoleMaster).
		Count(&activeMasterCount).Error)
	assert.EqualValues(t, 1, activeMasterCount)

	var guildCount int64
	require.NoError(t, db.Model(&domain.Guild{}).
		Where("id IN ?", []uuid.UUID{guilds[0].ID, guilds[1].ID}).
		Count(&guildCount).Error)
	assert.EqualValues(t, 1, guildCount)
}
