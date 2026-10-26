package repository

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"
)

func TestGuildApplicationConcurrentReview(t *testing.T) {
	for _, test := range []struct {
		name        string
		mixedReview bool
	}{
		{name: "two approvals"},
		{name: "approval and rejection", mixedReview: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, app := setupGuildApplicationReview(t)
			ctx := context.Background()
			repo := NewGuildApplicationRepository(db)
			approve := func() error {
				appSnapshot := *app
				attendee := &domain.GuildAttendee{
					GuildID: app.GuildID,
					UserID:  app.UserID,
					Role:    domain.GuildAttendeeRoleMember,
				}
				return repo.ApproveWithTx(ctx, &appSnapshot, attendee)
			}
			reject := func() error { return repo.RejectPending(ctx, app.ID, app.GuildID) }
			secondReview := approve
			if test.mixedReview {
				secondReview = reject
			}

			start := make(chan struct{})
			results := make(chan error, 2)
			var ready sync.WaitGroup
			ready.Add(2)
			for _, review := range []func() error{approve, secondReview} {
				go func(review func() error) {
					ready.Done()
					<-start
					results <- review()
				}(review)
			}
			ready.Wait()
			close(start)

			var succeeded, noLongerPending int
			for range 2 {
				result := <-results
				switch {
				case result == nil:
					succeeded++
				case errors.Is(result, domain.ErrApplicationNotPending):
					noLongerPending++
				default:
					t.Fatalf("unexpected review error: %v", result)
				}
			}
			require.Equal(t, 1, succeeded)
			require.Equal(t, 1, noLongerPending)

			storedApp, err := repo.GetByID(ctx, app.ID)
			require.NoError(t, err)
			var attendeeCount int64
			require.NoError(t, db.WithContext(ctx).Model(&domain.GuildAttendee{}).
				Where("guild_id = ? AND user_id = ?", app.GuildID, app.UserID).
				Count(&attendeeCount).Error)
			if storedApp.Status == domain.GuildApplicationStatusApproved {
				require.EqualValues(t, 1, attendeeCount)
			} else {
				require.True(t, test.mixedReview)
				require.Equal(t, domain.GuildApplicationStatusRejected, storedApp.Status)
				require.Zero(t, attendeeCount)
			}
		})
	}
}

func setupGuildApplicationReview(t *testing.T) (*gorm.DB, *domain.GuildApplication) {
	t.Helper()
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
	require.NoError(t, db.WithContext(ctx).AutoMigrate(
		&domain.User{}, &domain.Guild{}, &domain.GuildAttendee{}, &domain.GuildApplication{},
	))

	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "guild applicant"}
	guild := domain.Guild{ID: uuid.New(), Name: "application review test"}
	app := &domain.GuildApplication{
		ID: uuid.New(), GuildID: guild.ID, UserID: user.ID, Status: domain.GuildApplicationStatusPending,
	}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)
	require.NoError(t, db.WithContext(ctx).Create(app).Error)
	t.Cleanup(func() {
		require.NoError(t, db.WithContext(ctx).Where("guild_id = ? AND user_id = ?", guild.ID, user.ID).
			Delete(&domain.GuildAttendee{}).Error)
		require.NoError(t, db.WithContext(ctx).Delete(&domain.GuildApplication{}, "id = ?", app.ID).Error)
		require.NoError(t, db.WithContext(ctx).Unscoped().Delete(&guild).Error)
		require.NoError(t, db.WithContext(ctx).Unscoped().Delete(&user).Error)
	})
	return db, app
}
