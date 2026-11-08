package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// testOutboxIntent builds a valid intent with the given dedup key. Status and
// AttemptCount are left at their zero values on purpose: the write side must
// fall back to the column defaults.
func testOutboxIntent(kind, aggregateType string, aggregateID, recipientUserID uuid.UUID, dedupKey string) *domain.NotificationOutbox {
	return &domain.NotificationOutbox{
		Kind:            kind,
		AggregateType:   aggregateType,
		AggregateID:     aggregateID,
		RecipientUserID: recipientUserID,
		Payload:         datatypes.JSON([]byte(`{"deeplink":"/v1/activities/` + aggregateID.String() + `"}`)),
		DedupKey:        dedupKey,
	}
}

// countOutboxRows counts the outbox rows stored under dedupKey.
func countOutboxRows(t *testing.T, db *gorm.DB, dedupKey string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&domain.NotificationOutbox{}).
		Where("dedup_key = ?", dedupKey).
		Count(&count).Error)
	return count
}

func TestNotificationOutboxIntentCommitsAndRollsBackWithBusinessWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("commit", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)

		user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox commit user"}
		guild := domain.Guild{ID: uuid.New(), Name: "outbox commit guild"}
		intent := testOutboxIntent(
			domain.NotificationKindMemberJoined, "guild", guild.ID, user.ID,
			domain.NotificationDedupKey(domain.NotificationKindMemberJoined, guild.ID, user.ID),
		)

		require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
			if err := r.UserRepo.Create(ctx, &user); err != nil {
				return err
			}
			if err := r.GuildRepo.Create(ctx, &guild); err != nil {
				return err
			}
			return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent})
		}))

		// The state change and its intent are both visible after the commit.
		require.NoError(t, db.First(&domain.User{}, "id = ?", user.ID).Error)
		require.NoError(t, db.First(&domain.Guild{}, "id = ?", guild.ID).Error)
		assert.EqualValues(t, 1, countOutboxRows(t, db, intent.DedupKey))
	})

	t.Run("rollback", func(t *testing.T) {
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		boom := errors.New("fail the transaction after the intent")

		user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox rollback user"}
		guild := domain.Guild{ID: uuid.New(), Name: "outbox rollback guild"}
		intent := testOutboxIntent(
			domain.NotificationKindActivityScheduled, "activity", guild.ID, user.ID,
			domain.NotificationDedupKey(domain.NotificationKindActivityScheduled, guild.ID, user.ID),
		)

		err := store.Tx(ctx, func(r RepoSet) error {
			require.NoError(t, r.UserRepo.Create(ctx, &user))
			require.NoError(t, r.GuildRepo.Create(ctx, &guild))
			require.NoError(t, r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent}))
			return boom
		})
		require.ErrorIs(t, err, boom)

		// The intent and the business writes disappear together.
		assert.Zero(t, countOutboxRows(t, db, intent.DedupKey))
		assert.Zero(t, countRows(t, db, &domain.User{}, "id = ?", user.ID))
		assert.Zero(t, countRows(t, db, &domain.Guild{}, "id = ?", guild.ID))
	})
}

func TestNotificationOutboxCreateIntentsInsertsOneRowPerRecipient(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	recipients := []domain.User{
		{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox recipient one"},
		{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox recipient two"},
	}
	require.NoError(t, db.Create(&recipients).Error)

	aggregateID := uuid.New()
	intents := make([]*domain.NotificationOutbox, 0, len(recipients))
	for _, recipient := range recipients {
		intents = append(intents, testOutboxIntent(
			domain.NotificationKindActivityStartingSoon, "activity", aggregateID, recipient.ID,
			domain.NotificationDedupKeyWithOccurrence(
				domain.NotificationKindActivityStartingSoon, aggregateID, time.Now(), recipient.ID),
		))
	}

	require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
		return r.NotificationOutboxRepo.CreateIntents(ctx, intents)
	}))

	var count int64
	require.NoError(t, db.Model(&domain.NotificationOutbox{}).
		Where("aggregate_id = ?", aggregateID).
		Count(&count).Error)
	assert.EqualValues(t, len(recipients), count)
	for _, intent := range intents {
		assert.NotEqual(t, uuid.Nil, intent.ID, "the repository assigned the primary key")
	}
}

func TestNotificationOutboxDuplicateDedupKeyIsNoOp(t *testing.T) {
	ctx := context.Background()

	newFixture := func(t *testing.T) (*gorm.DB, *Store, *domain.User) {
		t.Helper()
		db := testutil.NewPostgresDB(t)
		store := NewStore(db)
		user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox duplicate user"}
		require.NoError(t, db.Create(&user).Error)
		return db, store, &user
	}

	t.Run("in one batch", func(t *testing.T) {
		db, store, user := newFixture(t)
		aggregateID := uuid.New()
		dedupKey := domain.NotificationDedupKey(domain.NotificationKindMemberJoined, aggregateID, user.ID)

		intents := []*domain.NotificationOutbox{
			testOutboxIntent(domain.NotificationKindMemberJoined, "guild", aggregateID, user.ID, dedupKey),
			testOutboxIntent(domain.NotificationKindMemberJoined, "guild", aggregateID, user.ID, dedupKey),
		}

		require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
			return r.NotificationOutboxRepo.CreateIntents(ctx, intents)
		}))
		assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey))
	})

	t.Run("in one transaction", func(t *testing.T) {
		db, store, user := newFixture(t)
		aggregateID := uuid.New()
		dedupKey := domain.NotificationDedupKey(domain.NotificationKindAnalysisCompleted, aggregateID, user.ID)

		require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
			first := testOutboxIntent(domain.NotificationKindAnalysisCompleted, "activity", aggregateID, user.ID, dedupKey)
			if err := r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{first}); err != nil {
				return err
			}
			second := testOutboxIntent(domain.NotificationKindAnalysisCompleted, "activity", aggregateID, user.ID, dedupKey)
			return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{second})
		}))
		assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey))
	})

	t.Run("across transactions", func(t *testing.T) {
		db, store, user := newFixture(t)
		aggregateID := uuid.New()
		dedupKey := domain.NotificationDedupKey(domain.NotificationKindFeedAnnouncementPosted, aggregateID, user.ID)

		for attempt := 0; attempt < 2; attempt++ {
			require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
				intent := testOutboxIntent(domain.NotificationKindFeedAnnouncementPosted, "feed_post", aggregateID, user.ID, dedupKey)
				return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent})
			}), "duplicate intent attempt %d must not fail", attempt)
		}
		assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey))
	})
}

func TestNotificationOutboxConcurrentDuplicateDedupKeyCommitsOnce(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Keep the pool small so the concurrent transactions really contend on
	// separate connections instead of queueing on one.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)

	recipient := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox concurrent user"}
	require.NoError(t, db.Create(&recipient).Error)

	aggregateID := uuid.New()
	dedupKey := domain.NotificationDedupKey(domain.NotificationKindMemberJoined, aggregateID, recipient.ID)

	const writers = 2
	start := make(chan struct{})
	results := make(chan error, writers)
	var ready sync.WaitGroup
	ready.Add(writers)

	for i := 0; i < writers; i++ {
		go func() {
			ready.Done()
			<-start
			results <- store.Tx(ctx, func(r RepoSet) error {
				intent := testOutboxIntent(
					domain.NotificationKindMemberJoined, "guild", aggregateID, recipient.ID, dedupKey)
				return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent})
			})
		}()
	}

	ready.Wait()
	close(start)

	for i := 0; i < writers; i++ {
		require.NoError(t, <-results, "both concurrent transactions must commit")
	}
	assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey))
}

func TestNotificationOutboxIntentDefaultsToPending(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	recipient := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox default user"}
	require.NoError(t, db.Create(&recipient).Error)

	aggregateID := uuid.New()
	intent := testOutboxIntent(
		domain.NotificationKindAnalysisCompleted, "activity", aggregateID, recipient.ID,
		domain.NotificationDedupKey(domain.NotificationKindAnalysisCompleted, aggregateID, recipient.ID),
	)
	require.NoError(t, store.Tx(ctx, func(r RepoSet) error {
		return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent})
	}))

	var stored domain.NotificationOutbox
	require.NoError(t, db.First(&stored, "id = ?", intent.ID).Error)
	assert.Equal(t, domain.NotificationOutboxStatusPending, stored.Status)
	assert.Zero(t, stored.AttemptCount)
	assert.Nil(t, stored.NextAttemptAt)
	assert.Nil(t, stored.LastError)
	assert.Equal(t, domain.NotificationKindAnalysisCompleted, stored.Kind)
	assert.JSONEq(t, string(intent.Payload), string(stored.Payload))
}

func TestNotificationOutboxRecurringOccurrenceInsertsOneRowPerOccurrence(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	store := NewStore(db)
	ctx := context.Background()

	recipient := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "outbox recurring user"}
	require.NoError(t, db.Create(&recipient).Error)

	activityID := uuid.New()
	practiceAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	dedupKey := func(occurrence time.Time) string {
		return domain.NotificationDedupKeyWithOccurrence(
			domain.NotificationKindActivityStartingSoon, activityID, occurrence, recipient.ID)
	}
	write := func(occurrence time.Time) error {
		return store.Tx(ctx, func(r RepoSet) error {
			intent := testOutboxIntent(
				domain.NotificationKindActivityStartingSoon, "activity", activityID, recipient.ID, dedupKey(occurrence))
			return r.NotificationOutboxRepo.CreateIntents(ctx, []*domain.NotificationOutbox{intent})
		})
	}

	require.NoError(t, write(practiceAt))
	assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey(practiceAt)))

	// The same instant in another time zone is the same occurrence: no new row.
	require.NoError(t, write(practiceAt.In(time.FixedZone("JST", 9*60*60))))
	assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey(practiceAt)))

	// A rescheduled occurrence gets its own row next to the original one.
	rescheduled := practiceAt.Add(48 * time.Hour)
	require.NoError(t, write(rescheduled))
	assert.EqualValues(t, 1, countOutboxRows(t, db, dedupKey(rescheduled)))

	var occurrences int64
	require.NoError(t, db.Model(&domain.NotificationOutbox{}).
		Where("kind = ? AND aggregate_id = ? AND recipient_user_id = ?",
			domain.NotificationKindActivityStartingSoon, activityID, recipient.ID).
		Count(&occurrences).Error)
	assert.EqualValues(t, 2, occurrences)
}
