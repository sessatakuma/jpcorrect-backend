package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// newActivityFixture creates the guild and users an Activity row references.
func newActivityFixture(t *testing.T, db *gorm.DB) (domain.Guild, domain.User) {
	t.Helper()
	ctx := context.Background()

	guild := domain.Guild{ID: uuid.New(), Name: "activity schema test"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "activity owner"}
	require.NoError(t, db.WithContext(ctx).Create(&user).Error)

	return guild, user
}

// newActivity returns an unsaved Activity that satisfies the NOT NULL columns.
func newActivity(guildID uuid.UUID, sequence int) *domain.Activity {
	return &domain.Activity{
		ID:             uuid.New(),
		GuildID:        guildID,
		SequenceNumber: sequence,
		Mode:           domain.ActivityModeReport,
	}
}

// TestActivityStatusEnteredAt covers the engineering note of #72: the internal
// status_entered_at column must move when the status changes and must not move
// on any other update. #98 relies on that to measure how long an Activity has
// been stuck in in_practice / analyzing / in_review.
func TestActivityStatusEnteredAt(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, _ := newActivityFixture(t, db)

	activity := newActivity(guild.ID, 1)
	activity.Status = domain.ActivityStatusPendingPractice
	require.NoError(t, db.WithContext(ctx).Create(activity).Error)
	require.False(t, activity.StatusEnteredAt.IsZero(), "creating an Activity must record when it entered its status")

	reload := func(t *testing.T) domain.Activity {
		t.Helper()
		var stored domain.Activity
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", activity.ID).Error)
		return stored
	}

	// PostgreSQL timestamps have microsecond resolution; make every transition
	// measurably later than the previous stamp.
	advance := func() {
		time.Sleep(2 * time.Millisecond)
	}

	t.Run("create stamps the status entry time", func(t *testing.T) {
		assert.WithinDuration(t, time.Now(), reload(t).StatusEnteredAt, time.Minute)
	})

	t.Run("a map update that changes the status restamps it", func(t *testing.T) {
		before := reload(t).StatusEnteredAt
		advance()
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("id = ?", activity.ID).
			Updates(map[string]any{"status": domain.ActivityStatusInPractice}).Error)

		after := reload(t)
		assert.Equal(t, domain.ActivityStatusInPractice, after.Status)
		assert.True(t, after.StatusEnteredAt.After(before), "status_entered_at must move on a status change: %s -> %s",
			before, after.StatusEnteredAt)
	})

	t.Run("a struct update that changes the status restamps it", func(t *testing.T) {
		before := reload(t).StatusEnteredAt
		advance()
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("id = ?", activity.ID).
			Updates(&domain.Activity{Status: domain.ActivityStatusAnalyzing}).Error)

		after := reload(t)
		assert.Equal(t, domain.ActivityStatusAnalyzing, after.Status)
		assert.True(t, after.StatusEnteredAt.After(before), "status_entered_at must move on a status change: %s -> %s",
			before, after.StatusEnteredAt)
	})

	t.Run("Save that changes the status restamps it", func(t *testing.T) {
		stored := reload(t)
		before := stored.StatusEnteredAt
		advance()
		stored.Status = domain.ActivityStatusInFeedback
		require.NoError(t, db.WithContext(ctx).Save(&stored).Error)

		after := reload(t)
		assert.Equal(t, domain.ActivityStatusInFeedback, after.Status)
		assert.True(t, after.StatusEnteredAt.After(before), "status_entered_at must move on a status change: %s -> %s",
			before, after.StatusEnteredAt)
	})

	t.Run("updating another column does not restamp it", func(t *testing.T) {
		before := reload(t).StatusEnteredAt
		advance()
		youtubeURL := "https://example.com/unlisted"
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("id = ?", activity.ID).
			Update("youtube_url", youtubeURL).Error)

		after := reload(t)
		require.NotNil(t, after.YouTubeURL)
		assert.Equal(t, youtubeURL, *after.YouTubeURL)
		assert.WithinDuration(t, before, after.StatusEnteredAt, time.Microsecond,
			"an unrelated update must not move status_entered_at")
	})

	t.Run("Save that changes another column does not restamp it", func(t *testing.T) {
		stored := reload(t)
		before := stored.StatusEnteredAt
		advance()
		theme := "updated theme"
		stored.Theme = &theme
		require.NoError(t, db.WithContext(ctx).Save(&stored).Error)

		after := reload(t)
		require.NotNil(t, after.Theme)
		assert.Equal(t, theme, *after.Theme)
		assert.WithinDuration(t, before, after.StatusEnteredAt, time.Microsecond,
			"a Save that keeps the status must not move status_entered_at")
	})
}
