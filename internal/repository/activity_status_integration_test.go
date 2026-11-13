package repository

import (
	"context"
	"sync"
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

// TestActivityStatusEnteredAt covers the engineering note of #72 and spec §2.2
// row 30: status_entered_at moves when the status changes through
// TransitionStatus and on no other write. #98 relies on that to measure how long
// an Activity has been stuck in in_practice / analyzing / in_review.
func TestActivityStatusEnteredAt(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	guild, _ := newActivityFixture(t, db)
	repo := NewGormActivityStatusRepository(db)

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

	t.Run("a transition changes the status and restamps it", func(t *testing.T) {
		before := reload(t).StatusEnteredAt
		advance()

		moved, err := repo.TransitionStatus(ctx, activity.ID,
			[]domain.ActivityStatus{domain.ActivityStatusPendingPractice}, domain.ActivityStatusInPractice)
		require.NoError(t, err)
		assert.True(t, moved)

		after := reload(t)
		assert.Equal(t, domain.ActivityStatusInPractice, after.Status)
		assert.True(t, after.StatusEnteredAt.After(before), "status_entered_at must move on a status change: %s -> %s",
			before, after.StatusEnteredAt)
	})

	t.Run("a transition accepts any of several from statuses", func(t *testing.T) {
		before := reload(t).StatusEnteredAt
		advance()

		moved, err := repo.TransitionStatus(ctx, activity.ID,
			[]domain.ActivityStatus{domain.ActivityStatusPendingPractice, domain.ActivityStatusInPractice},
			domain.ActivityStatusAnalyzing)
		require.NoError(t, err)
		assert.True(t, moved)

		after := reload(t)
		assert.Equal(t, domain.ActivityStatusAnalyzing, after.Status)
		assert.True(t, after.StatusEnteredAt.After(before))
	})

	t.Run("a transition from the wrong status changes nothing", func(t *testing.T) {
		before := reload(t)
		advance()

		moved, err := repo.TransitionStatus(ctx, activity.ID,
			[]domain.ActivityStatus{domain.ActivityStatusPendingPractice}, domain.ActivityStatusInFeedback)
		require.NoError(t, err)
		assert.False(t, moved, "the caller is the loser of a race or in the wrong state")

		after := reload(t)
		assert.Equal(t, before.Status, after.Status)
		assert.WithinDuration(t, before.StatusEnteredAt, after.StatusEnteredAt, time.Microsecond)
	})

	t.Run("a transition to the current status is a no-op and does not restamp", func(t *testing.T) {
		before := reload(t)
		advance()

		moved, err := repo.TransitionStatus(ctx, activity.ID,
			[]domain.ActivityStatus{domain.ActivityStatusAnalyzing}, domain.ActivityStatusAnalyzing)
		require.NoError(t, err)
		assert.False(t, moved)
		assert.WithinDuration(t, before.StatusEnteredAt, reload(t).StatusEnteredAt, time.Microsecond)
	})

	t.Run("an empty from list or an unknown activity changes nothing", func(t *testing.T) {
		moved, err := repo.TransitionStatus(ctx, activity.ID, nil, domain.ActivityStatusDone)
		require.NoError(t, err)
		assert.False(t, moved)

		moved, err = repo.TransitionStatus(ctx, uuid.New(),
			[]domain.ActivityStatus{domain.ActivityStatusAnalyzing}, domain.ActivityStatusDone)
		require.NoError(t, err)
		assert.False(t, moved)
	})

	t.Run("updates that do not go through a transition never restamp", func(t *testing.T) {
		stored := reload(t)
		before := stored.StatusEnteredAt
		advance()

		youtubeURL := "https://example.com/unlisted"
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("id = ?", activity.ID).Update("youtube_url", youtubeURL).Error)

		theme := "partial struct update"
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("guild_id = ?", guild.ID).Updates(&domain.Activity{Theme: &theme}).Error)

		// The same status written again by a plain update is not a transition.
		require.NoError(t, db.WithContext(ctx).Model(&domain.Activity{}).
			Where("id = ?", activity.ID).Updates(map[string]any{"status": stored.Status}).Error)

		stored.Theme = ptr("saved theme")
		require.NoError(t, db.WithContext(ctx).Save(&stored).Error)

		after := reload(t)
		assert.Equal(t, "saved theme", *after.Theme)
		assert.WithinDuration(t, before, after.StatusEnteredAt, time.Microsecond,
			"only TransitionStatus may move status_entered_at")
	})

	t.Run("only one of several concurrent transitions wins", func(t *testing.T) {
		racer := newActivity(guild.ID, 2)
		racer.Status = domain.ActivityStatusPendingPractice
		require.NoError(t, db.WithContext(ctx).Create(racer).Error)

		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(8)

		const callers = 8
		wins := make(chan bool, callers)
		start := make(chan struct{})
		var ready sync.WaitGroup
		ready.Add(callers)
		for range callers {
			go func() {
				ready.Done()
				<-start
				moved, err := repo.TransitionStatus(ctx, racer.ID,
					[]domain.ActivityStatus{domain.ActivityStatusPendingPractice}, domain.ActivityStatusInPractice)
				assert.NoError(t, err)
				wins <- moved
			}()
		}
		ready.Wait()
		close(start)

		var winners int
		for range callers {
			if <-wins {
				winners++
			}
		}
		assert.Equal(t, 1, winners, "a compare-and-swap lets exactly one caller win")

		var stored domain.Activity
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", racer.ID).Error)
		assert.Equal(t, domain.ActivityStatusInPractice, stored.Status)
	})
}
