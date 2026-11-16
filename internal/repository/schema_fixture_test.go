package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

// schemaFixture holds one persisted parent row per table that other tables
// point at, so a constraint test can build an otherwise valid child row and
// break exactly one column.
type schemaFixture struct {
	user1    uuid.UUID
	user2    uuid.UUID
	guild    uuid.UUID
	topic    uuid.UUID
	activity uuid.UUID
	event    uuid.UUID
	mistake  uuid.UUID
	outbox   uuid.UUID
	post     uuid.UUID
	session  uuid.UUID
}

// ptr returns a pointer to v; the nullable foreign keys and nullable time
// columns need one.
func ptr[T any](v T) *T { return &v }

// mustCreate persists row and fails the test if PostgreSQL rejects it.
func mustCreate[T any](t *testing.T, db *gorm.DB, row *T) *T {
	t.Helper()
	require.NoError(t, db.WithContext(context.Background()).Create(row).Error,
		"create fixture row %T", *row)
	return row
}

// newSchemaFixture migrates nothing itself: the caller already ran
// testutil.NewPostgresDB, and the fixture only fills the schema with parents.
func newSchemaFixture(t *testing.T, db *gorm.DB) *schemaFixture {
	t.Helper()

	f := &schemaFixture{
		user1: uuid.New(),
		user2: uuid.New(),
		guild: uuid.New(),
	}

	mustCreate(t, db, &domain.User{ID: f.user1, Email: uuid.NewString() + "@example.com", Name: "schema fixture one"})
	mustCreate(t, db, &domain.User{ID: f.user2, Email: uuid.NewString() + "@example.com", Name: "schema fixture two"})
	mustCreate(t, db, &domain.Guild{ID: f.guild, Name: "schema fixture guild"})

	topic := f.newTopic()
	mustCreate(t, db, topic)
	f.topic = topic.ID

	activity := f.newActivity(1)
	mustCreate(t, db, activity)
	f.activity = activity.ID

	event := f.newEvent(f.activity, domain.EventKindPractice)
	mustCreate(t, db, event)
	f.event = event.ID

	mistake := f.newMistake()
	mustCreate(t, db, mistake)
	f.mistake = mistake.ID

	outbox := f.newOutbox()
	mustCreate(t, db, outbox)
	f.outbox = outbox.ID

	post := f.newFeedPost()
	mustCreate(t, db, post)
	f.post = post.ID

	session := f.newRecordingSession()
	mustCreate(t, db, session)
	f.session = session.ID

	return f
}

// newTopic returns the topic row the fixture stores.
func (f *schemaFixture) newTopic() *domain.Topic {
	return &domain.Topic{
		ID:      uuid.New(),
		Kind:    domain.TopicKindAnnounce,
		TitleJP: "フィクスチャのお題",
	}
}

// newActivity returns an unsaved Activity in the fixture's guild. Only the
// unique (guild_id, sequence_number) pair varies.
func (f *schemaFixture) newActivity(sequenceNumber int) *domain.Activity {
	return &domain.Activity{
		ID:             uuid.New(),
		GuildID:        f.guild,
		SequenceNumber: sequenceNumber,
		Mode:           domain.ActivityModeReport,
	}
}

func (f *schemaFixture) newEvent(activityID uuid.UUID, kind domain.EventKind) *domain.Event {
	return &domain.Event{
		ID:         uuid.New(),
		ActivityID: activityID,
		Kind:       kind,
		Title:      "fixture event",
		StartTime:  time.Now().UTC(),
	}
}

func (f *schemaFixture) newMistake() *domain.Mistake {
	return &domain.Mistake{
		ID:         uuid.New(),
		EventID:    f.event,
		UserID:     f.user1,
		Type:       domain.MistakeTypeGrammar,
		OriginText: "これはです",
		FixedText:  "これはです。",
	}
}

func (f *schemaFixture) newOutbox() *domain.NotificationOutbox {
	return &domain.NotificationOutbox{
		ID:              uuid.New(),
		DedupKey:        uuid.NewString(),
		Kind:            "activity_created",
		AggregateType:   "activity",
		AggregateID:     f.activity,
		RecipientUserID: f.user1,
		Payload:         datatypes.JSON(`{"deeplink":"/activities/1"}`),
	}
}

func (f *schemaFixture) newNotification(userID, outboxID uuid.UUID) *domain.Notification {
	return &domain.Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Kind:     "activity_created",
		Payload:  datatypes.JSON(`{"deeplink":"/activities/1"}`),
		OutboxID: outboxID,
	}
}

func (f *schemaFixture) newFeedPost() *domain.FeedPost {
	return &domain.FeedPost{
		ID:      uuid.New(),
		GuildID: f.guild,
		Type:    domain.FeedPostTypeSystem,
		Content: "fixture system post",
	}
}

func (f *schemaFixture) newFeedComment(postID, authorID uuid.UUID) *domain.FeedComment {
	return &domain.FeedComment{
		ID:           uuid.New(),
		PostID:       postID,
		AuthorUserID: authorID,
		Content:      "fixture comment",
	}
}

func (f *schemaFixture) newRecordingSession() *domain.RecordingSession {
	return &domain.RecordingSession{
		ID:               uuid.New(),
		ActivityID:       f.activity,
		EventID:          f.event,
		Provider:         "livekit",
		ProviderRoomID:   uuid.NewString(),
		ProviderEgressID: uuid.NewString(),
		Status:           domain.RecordingSessionStatusRecording,
	}
}

func (f *schemaFixture) newRecordingAsset(sessionID uuid.UUID, participantID *uuid.UUID) *domain.RecordingAsset {
	return &domain.RecordingAsset{
		ID:                 uuid.New(),
		RecordingSessionID: sessionID,
		ParticipantUserID:  participantID,
		Kind:               domain.RecordingAssetKindAudioTrack,
		ObjectKey:          "recordings/" + uuid.NewString() + ".wav",
		Status:             domain.RecordingAssetStatusUploading,
	}
}

func (f *schemaFixture) newSelectedSentence(activityID, mistakeID, selectedBy uuid.UUID) *domain.SelectedSentence {
	return &domain.SelectedSentence{
		ID:               uuid.New(),
		ActivityID:       activityID,
		MistakeID:        mistakeID,
		SelectedByUserID: selectedBy,
	}
}

func (f *schemaFixture) newFeedbackProgress(activityID, userID uuid.UUID) *domain.FeedbackProgress {
	return &domain.FeedbackProgress{ActivityID: activityID, UserID: userID}
}

func (f *schemaFixture) newPracticeNote(activityID, userID uuid.UUID) *domain.PracticeNote {
	return &domain.PracticeNote{ActivityID: activityID, UserID: userID, Content: "fixture note"}
}

func (f *schemaFixture) newActivityReporter(activityID, userID uuid.UUID) *domain.ActivityReporter {
	return &domain.ActivityReporter{ActivityID: activityID, UserID: userID}
}

func (f *schemaFixture) newReviewRecord(activityID uuid.UUID) *domain.ReviewRecord {
	return &domain.ReviewRecord{ActivityID: activityID, Stage2Summary: datatypes.JSON(`{"patterns":[]}`)}
}

func (f *schemaFixture) newGuildDefaultSlot(guildID uuid.UUID) *domain.GuildDefaultSlot {
	return &domain.GuildDefaultSlot{
		GuildID:   guildID,
		DayOfWeek: 3,
		TimeOfDay: datatypes.NewTime(20, 0, 0, 0),
	}
}

func (f *schemaFixture) newJoinRequest(guildID, userID uuid.UUID) *domain.JoinRequest {
	return &domain.JoinRequest{ID: uuid.New(), GuildID: guildID, UserID: userID}
}

func (f *schemaFixture) newPushSubscription(userID uuid.UUID) *domain.PushSubscription {
	return &domain.PushSubscription{
		ID:       uuid.New(),
		UserID:   userID,
		Endpoint: "https://push.example.com/" + uuid.NewString(),
		Keys:     datatypes.JSON(`{"p256dh":"key","auth":"secret"}`),
	}
}

func (f *schemaFixture) newChatbotUsage(userID uuid.UUID, activityID *uuid.UUID) *domain.ChatbotUsage {
	return &domain.ChatbotUsage{
		ID:           uuid.New(),
		UserID:       userID,
		ActivityID:   activityID,
		Kind:         domain.ChatbotUsageKindFeedback,
		MessageCount: 3,
		TokensIn:     100,
		TokensOut:    200,
	}
}

func (f *schemaFixture) newAIAnalysisSubmission(activityID uuid.UUID) *domain.AIAnalysisSubmission {
	return &domain.AIAnalysisSubmission{
		SubmissionID:    uuid.New(),
		ActivityID:      activityID,
		RequestPayload:  datatypes.JSON(`{"activity_id":"` + activityID.String() + `"}`),
		RequestHash:     uuid.NewString(),
		ResponsePayload: datatypes.JSON(`{"ok":true}`),
	}
}

func (f *schemaFixture) newInviteLink(guildID, userID uuid.UUID) *domain.InviteLink {
	return &domain.InviteLink{
		ID:              uuid.New(),
		GuildID:         guildID,
		Token:           uuid.NewString()[:32],
		ExpiresAt:       time.Now().Add(7 * 24 * time.Hour),
		CreatedByUserID: userID,
	}
}
