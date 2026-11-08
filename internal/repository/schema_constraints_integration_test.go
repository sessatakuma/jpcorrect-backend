package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// constraintCase is one violating insert. insert returns the raw GORM error;
// the test maps it through MapGormError so the assertion also covers the
// repository error mapping every caller depends on.
type constraintCase struct {
	name   string
	insert func(t *testing.T) error
}

func runConstraintCases(t *testing.T, cases []constraintCase, want error) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := MapGormError(tc.insert(t))
			require.Error(t, err, "inserting a violating row must fail")
			assert.ErrorIs(t, err, want, "got %v, want %v", err, want)
		})
	}
}

// TestUniqueConstraintsRejectDuplicates triggers every unique constraint of the
// spec §2 schema with a real second insert. Each must reach the caller as
// domain.ErrDuplicateEntry.
func TestUniqueConstraintsRejectDuplicates(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	f := newSchemaFixture(t, db)

	cases := []constraintCase{
		{
			name: "user.supabase_user_id",
			insert: func(t *testing.T) error {
				first := mustCreate(t, db, &domain.User{
					ID:             uuid.New(),
					Email:          uuid.NewString() + "@example.com",
					Name:           "supabase one",
					SupabaseUserID: ptr(uuid.NewString()),
				})
				return db.WithContext(ctx).Create(&domain.User{
					ID:             uuid.New(),
					Email:          uuid.NewString() + "@example.com",
					Name:           "supabase duplicate",
					SupabaseUserID: first.SupabaseUserID,
				}).Error
			},
		},
		{
			name: "user.email (unchanged by this issue)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(&domain.User{
					ID:    uuid.New(),
					Email: "same-address@example.com",
					Name:  "email one",
				}).Error
			},
		},
		{
			name: "event (activity_id, kind)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newEvent(f.activity, domain.EventKindPractice)).Error
			},
		},
		{
			name: "activity (guild_id, sequence_number)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newActivity(1)).Error
			},
		},
		{
			name: "invite_link.token",
			insert: func(t *testing.T) error {
				first := mustCreate(t, db, f.newInviteLink(f.guild, f.user1))
				duplicate := f.newInviteLink(f.guild, f.user1)
				duplicate.Token = first.Token
				return db.WithContext(ctx).Create(duplicate).Error
			},
		},
		{
			name: "selected_sentence (mistake_id, selected_by_user_id)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(
					f.newSelectedSentence(f.activity, f.mistake, f.user1)).Error
			},
		},
		{
			name: "notification.outbox_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newNotification(f.user2, f.outbox)).Error
			},
		},
		{
			name: "notification_outbox.dedup_key",
			insert: func(t *testing.T) error {
				duplicate := f.newOutbox()
				duplicate.DedupKey = f.mustOutboxDedupKey(t, db)
				return db.WithContext(ctx).Create(duplicate).Error
			},
		},
		{
			name: "push_subscription.endpoint",
			insert: func(t *testing.T) error {
				first := mustCreate(t, db, f.newPushSubscription(f.user1))
				duplicate := f.newPushSubscription(f.user2)
				duplicate.Endpoint = first.Endpoint
				return db.WithContext(ctx).Create(duplicate).Error
			},
		},
		{
			name: "ai_analysis_submission.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newAIAnalysisSubmission(f.activity)).Error
			},
		},
		{
			name: "activity_reporter primary key",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newActivityReporter(f.activity, f.user1)).Error
			},
		},
		{
			name: "feedback_progress primary key",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newFeedbackProgress(f.activity, f.user1)).Error
			},
		},
		{
			name: "practice_note primary key",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newPracticeNote(f.activity, f.user1)).Error
			},
		},
		{
			name: "guild_default_slot primary key",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newGuildDefaultSlot(f.guild)).Error
			},
		},
		{
			name: "review_record primary key",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newReviewRecord(f.activity)).Error
			},
		},
	}

	// Rows that must already exist for the duplicate inserts above.
	mustCreate(t, db, &domain.User{ID: uuid.New(), Email: "same-address@example.com", Name: "email zero"})
	mustCreate(t, db, f.newSelectedSentence(f.activity, f.mistake, f.user1))
	mustCreate(t, db, f.newNotification(f.user1, f.outbox))
	mustCreate(t, db, f.newAIAnalysisSubmission(f.activity))
	mustCreate(t, db, f.newActivityReporter(f.activity, f.user1))
	mustCreate(t, db, f.newFeedbackProgress(f.activity, f.user1))
	mustCreate(t, db, f.newPracticeNote(f.activity, f.user1))
	mustCreate(t, db, f.newGuildDefaultSlot(f.guild))
	mustCreate(t, db, f.newReviewRecord(f.activity))

	runConstraintCases(t, cases, domain.ErrDuplicateEntry)
}

// TestForeignKeyConstraintsRejectOrphans breaks one foreign key at a time and
// requires PostgreSQL to reject the insert with ErrHasRelatedRecords. The
// nullable foreign keys of #72 are covered too: NULL is allowed, an unknown id
// is not.
func TestForeignKeyConstraintsRejectOrphans(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	f := newSchemaFixture(t, db)

	orphan := uuid.New

	cases := []constraintCase{
		{
			name: "activity.guild_id",
			insert: func(t *testing.T) error {
				row := f.newActivity(11)
				row.GuildID = orphan()
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "activity.announce_topic_id (nullable)",
			insert: func(t *testing.T) error {
				row := f.newActivity(11)
				row.AnnounceTopicID = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "activity.aborted_by_user_id (nullable)",
			insert: func(t *testing.T) error {
				row := f.newActivity(11)
				row.AbortedByUserID = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "event.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newEvent(orphan(), domain.EventKindReview)).Error
			},
		},
		{
			name: "event.recording_started_by (nullable)",
			insert: func(t *testing.T) error {
				row := f.newEvent(f.activity, domain.EventKindReview)
				row.RecordingStartedBy = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "activity_reporter.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newActivityReporter(orphan(), f.user1)).Error
			},
		},
		{
			name: "activity_reporter.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newActivityReporter(f.activity, orphan())).Error
			},
		},
		{
			name: "ai_analysis_submission.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newAIAnalysisSubmission(orphan())).Error
			},
		},
		{
			name: "guild_default_slot.guild_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newGuildDefaultSlot(orphan())).Error
			},
		},
		{
			name: "invite_link.guild_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newInviteLink(orphan(), f.user1)).Error
			},
		},
		{
			name: "invite_link.created_by_user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newInviteLink(f.guild, orphan())).Error
			},
		},
		{
			name: "join_request.guild_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newJoinRequest(orphan(), f.user1)).Error
			},
		},
		{
			name: "join_request.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newJoinRequest(f.guild, orphan())).Error
			},
		},
		{
			name: "feedback_progress.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newFeedbackProgress(orphan(), f.user1)).Error
			},
		},
		{
			name: "feedback_progress.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newFeedbackProgress(f.activity, orphan())).Error
			},
		},
		{
			name: "selected_sentence.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newSelectedSentence(orphan(), f.mistake, f.user1)).Error
			},
		},
		{
			name: "selected_sentence.mistake_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newSelectedSentence(f.activity, orphan(), f.user1)).Error
			},
		},
		{
			name: "selected_sentence.selected_by_user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newSelectedSentence(f.activity, f.mistake, orphan())).Error
			},
		},
		{
			name: "selected_sentence.revealed_by_user_id (nullable)",
			insert: func(t *testing.T) error {
				row := f.newSelectedSentence(f.activity, f.mistake, f.user1)
				row.RevealedByUserID = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "review_record.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newReviewRecord(orphan())).Error
			},
		},
		{
			name: "practice_note.activity_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newPracticeNote(orphan(), f.user1)).Error
			},
		},
		{
			name: "practice_note.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newPracticeNote(f.activity, orphan())).Error
			},
		},
		{
			name: "feed_post.guild_id",
			insert: func(t *testing.T) error {
				row := f.newFeedPost()
				row.GuildID = orphan()
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "feed_post.author_user_id (nullable)",
			insert: func(t *testing.T) error {
				row := f.newFeedPost()
				row.AuthorUserID = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "feed_comment.post_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newFeedComment(orphan(), f.user1)).Error
			},
		},
		{
			name: "feed_comment.author_user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newFeedComment(f.post, orphan())).Error
			},
		},
		{
			name: "notification.user_id",
			insert: func(t *testing.T) error {
				outbox := mustCreate(t, db, f.newOutbox())
				return db.WithContext(ctx).Create(f.newNotification(orphan(), outbox.ID)).Error
			},
		},
		{
			name: "notification.outbox_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newNotification(f.user2, orphan())).Error
			},
		},
		{
			name: "notification_outbox.recipient_user_id",
			insert: func(t *testing.T) error {
				row := f.newOutbox()
				row.RecipientUserID = orphan()
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "push_subscription.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newPushSubscription(orphan())).Error
			},
		},
		{
			name: "chatbot_usage.user_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newChatbotUsage(orphan(), nil)).Error
			},
		},
		{
			name: "chatbot_usage.activity_id (nullable)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newChatbotUsage(f.user1, ptr(orphan()))).Error
			},
		},
		{
			name: "recording_session.activity_id",
			insert: func(t *testing.T) error {
				row := f.newRecordingSession()
				row.ActivityID = orphan()
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "recording_session.event_id",
			insert: func(t *testing.T) error {
				row := f.newRecordingSession()
				row.EventID = orphan()
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "recording_session.partial_approved_by_user_id (nullable)",
			insert: func(t *testing.T) error {
				row := f.newRecordingSession()
				row.PartialApprovedByUserID = ptr(orphan())
				return db.WithContext(ctx).Create(row).Error
			},
		},
		{
			name: "recording_asset.recording_session_id",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newRecordingAsset(orphan(), nil)).Error
			},
		},
		{
			name: "recording_asset.participant_user_id (nullable)",
			insert: func(t *testing.T) error {
				return db.WithContext(ctx).Create(f.newRecordingAsset(f.session, ptr(orphan()))).Error
			},
		},
	}

	runConstraintCases(t, cases, domain.ErrHasRelatedRecords)
}

// TestNullableForeignKeysAcceptNull is the other half of the nullable foreign
// keys: NULL must be accepted where #72 lists a nullable foreign key, and a
// valid id must be accepted as well.
func TestNullableForeignKeysAcceptNull(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	f := newSchemaFixture(t, db)

	create := func(t *testing.T, name string, row any) {
		t.Helper()
		require.NoError(t, db.WithContext(ctx).Create(row).Error, name)
	}

	// Each nullable foreign key is stored once as NULL and once with a valid id.
	t.Run("activity.announce_topic_id", func(t *testing.T) {
		create(t, "null", f.newActivity(2))
		withTopic := f.newActivity(3)
		withTopic.AnnounceTopicID = ptr(f.topic)
		create(t, "valid", withTopic)
	})

	t.Run("activity.aborted_by_user_id", func(t *testing.T) {
		create(t, "null", f.newActivity(4))
		aborted := f.newActivity(5)
		aborted.AbortedByUserID = ptr(f.user1)
		create(t, "valid", aborted)
	})

	t.Run("event.recording_started_by", func(t *testing.T) {
		// The fixture activity already has its practice Event; the review Event
		// carries NULL and a second activity's practice Event a valid user.
		create(t, "null", f.newEvent(f.activity, domain.EventKindReview))
		other := f.newActivity(6)
		create(t, "activity", other)
		started := f.newEvent(other.ID, domain.EventKindPractice)
		started.RecordingStartedBy = ptr(f.user1)
		create(t, "valid", started)
	})

	t.Run("selected_sentence.revealed_by_user_id", func(t *testing.T) {
		create(t, "null", f.newSelectedSentence(f.activity, f.mistake, f.user1))
		revealed := f.newSelectedSentence(f.activity, f.mistake, f.user2)
		revealed.RevealedByUserID = ptr(f.user1)
		create(t, "valid", revealed)
	})

	t.Run("feed_post.author_user_id", func(t *testing.T) {
		create(t, "null", f.newFeedPost())
		authored := f.newFeedPost()
		authored.AuthorUserID = ptr(f.user1)
		create(t, "valid", authored)
	})

	t.Run("chatbot_usage.activity_id", func(t *testing.T) {
		create(t, "null", f.newChatbotUsage(f.user1, nil))
		create(t, "valid", f.newChatbotUsage(f.user1, ptr(f.activity)))
	})

	t.Run("recording_session.partial_approved_by_user_id", func(t *testing.T) {
		create(t, "null", f.newRecordingSession())
		approved := f.newRecordingSession()
		approved.PartialApprovedByUserID = ptr(f.user1)
		create(t, "valid", approved)
	})

	t.Run("recording_asset.participant_user_id", func(t *testing.T) {
		create(t, "null", f.newRecordingAsset(f.session, nil))
		create(t, "valid", f.newRecordingAsset(f.session, ptr(f.user1)))
	})
}

// TestSchemaColumnDefaults verifies the defaults of spec §2 by inserting a row
// that omits them and reading the stored values back.
func TestSchemaColumnDefaults(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	f := newSchemaFixture(t, db)

	reloadActivity := func(t *testing.T, id uuid.UUID) domain.Activity {
		t.Helper()
		var stored domain.Activity
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", id).Error)
		return stored
	}

	t.Run("activity status columns", func(t *testing.T) {
		row := f.newActivity(2)
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		stored := reloadActivity(t, row.ID)
		assert.Equal(t, domain.ActivityStatusPendingPractice, stored.Status)
		assert.Equal(t, domain.AIStatusPending, stored.AIStatus)
		assert.Equal(t, domain.YouTubeStatusPending, stored.YouTubeStatus)
	})

	t.Run("mistake confidence and correction source", func(t *testing.T) {
		row := f.newMistake()
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.Mistake
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", row.ID).Error)
		assert.Equal(t, domain.MistakeConfidenceMedium, stored.Confidence)
		assert.Equal(t, domain.MistakeCorrectionSourceInitial, stored.LatestCorrectionSource)
		assert.Nil(t, stored.LatestCorrectionText)
		assert.Nil(t, stored.InteractedAt)
	})

	t.Run("notification_outbox dispatch columns", func(t *testing.T) {
		row := f.newOutbox()
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.NotificationOutbox
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", row.ID).Error)
		assert.Equal(t, domain.NotificationOutboxStatusPending, stored.Status)
		assert.Zero(t, stored.AttemptCount)
		assert.Nil(t, stored.NextAttemptAt)
		assert.Nil(t, stored.LastError)
	})

	t.Run("topic difficulty", func(t *testing.T) {
		row := f.newTopic()
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.Topic
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", row.ID).Error)
		assert.Equal(t, domain.TopicDifficultyMedium, stored.Difficulty)
	})

	t.Run("feedback_progress status", func(t *testing.T) {
		row := f.newFeedbackProgress(f.activity, f.user2)
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.FeedbackProgress
		require.NoError(t, db.WithContext(ctx).First(&stored,
			"activity_id = ? AND user_id = ?", row.ActivityID, row.UserID).Error)
		assert.Equal(t, domain.FeedbackProgressStatusNotStarted, stored.Status)
		assert.Nil(t, stored.StartedAt)
		assert.Nil(t, stored.CompletedAt)
	})

	t.Run("join_request status", func(t *testing.T) {
		row := f.newJoinRequest(f.guild, f.user2)
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.JoinRequest
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", row.ID).Error)
		assert.Equal(t, domain.JoinRequestStatusPending, stored.Status)
	})

	t.Run("selected_sentence is_degraded", func(t *testing.T) {
		row := f.newSelectedSentence(f.activity, f.mistake, f.user2)
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.SelectedSentence
		require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", row.ID).Error)
		assert.False(t, stored.IsDegraded)
		assert.Nil(t, stored.RevealedAt)
		assert.Nil(t, stored.RevealedByUserID)
	})

	t.Run("guild_default_slot timezone", func(t *testing.T) {
		row := f.newGuildDefaultSlot(f.guild)
		require.NoError(t, db.WithContext(ctx).Create(row).Error)

		var stored domain.GuildDefaultSlot
		require.NoError(t, db.WithContext(ctx).First(&stored, "guild_id = ?", row.GuildID).Error)
		assert.Equal(t, "Asia/Taipei", stored.Timezone)
	})
}

// mustOutboxDedupKey returns the dedup key of the fixture's outbox row, which
// the duplicate insert must reuse.
func (f *schemaFixture) mustOutboxDedupKey(t *testing.T, db *gorm.DB) string {
	t.Helper()

	var stored domain.NotificationOutbox
	require.NoError(t, db.WithContext(context.Background()).First(&stored, "id = ?", f.outbox).Error)
	return stored.DedupKey
}
