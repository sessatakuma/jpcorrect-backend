package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNotificationDedupKeyOneOffFormat(t *testing.T) {
	aggregateID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	recipientUserID := uuid.MustParse("22222222-2222-4222-8222-222222222222")

	assert.Equal(t,
		"analysis.completed:11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222",
		NotificationDedupKey(NotificationKindAnalysisCompleted, aggregateID, recipientUserID),
		"spec §6.5 one-off key is {kind}:{aggregate_id}:{recipient_user_id}",
	)
}

func TestNotificationDedupKeyOneOffIgnoresOccurrence(t *testing.T) {
	aggregateID := uuid.New()
	recipientUserID := uuid.New()

	assert.Equal(t,
		NotificationDedupKey(NotificationKindMemberJoined, aggregateID, recipientUserID),
		NotificationDedupKey(NotificationKindMemberJoined, aggregateID, recipientUserID),
		"the one-off builder has no occurrence input",
	)
}

func TestNotificationDedupKeyWithOccurrenceFormat(t *testing.T) {
	aggregateID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	recipientUserID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	occurrence := time.Date(2026, 3, 14, 9, 30, 15, 123456000, time.UTC)

	assert.Equal(t,
		"activity.starting_soon:11111111-1111-4111-8111-111111111111:2026-03-14T09:30:15.123456Z:22222222-2222-4222-8222-222222222222",
		NotificationDedupKeyWithOccurrence(NotificationKindActivityStartingSoon, aggregateID, occurrence, recipientUserID),
		"spec §6.5 recurring key is {kind}:{aggregate_id}:{occurrence}:{recipient_user_id}",
	)
}

func TestNotificationDedupKeyWithOccurrenceIsTimeZoneInvariant(t *testing.T) {
	aggregateID := uuid.New()
	recipientUserID := uuid.New()
	occurrence := time.Date(2026, 3, 14, 9, 30, 15, 123456000, time.UTC)

	want := NotificationDedupKeyWithOccurrence(
		NotificationKindFeedbackDeadlineApproaching, aggregateID, occurrence, recipientUserID)

	// The same instant, expressed in zones on both sides of UTC.
	for _, location := range []*time.Location{
		time.UTC,
		time.FixedZone("JST", 9*60*60),
		time.FixedZone("HST", -10*60*60),
	} {
		got := NotificationDedupKeyWithOccurrence(
			NotificationKindFeedbackDeadlineApproaching, aggregateID, occurrence.In(location), recipientUserID)
		assert.Equal(t, want, got, "occurrence in %s changed the key", location)
	}
}

func TestNotificationDedupKeyWithOccurrenceChangesWhenRescheduled(t *testing.T) {
	aggregateID := uuid.New()
	recipientUserID := uuid.New()
	practiceAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	original := NotificationDedupKeyWithOccurrence(
		NotificationKindActivityStartingSoon, aggregateID, practiceAt, recipientUserID)
	rescheduled := NotificationDedupKeyWithOccurrence(
		NotificationKindActivityStartingSoon, aggregateID, practiceAt.Add(48*time.Hour), recipientUserID)

	assert.NotEqual(t, original, rescheduled,
		"a rescheduled occurrence must produce a new key so the reminder is not suppressed")
}

func TestNotificationDedupKeyWithOccurrenceUsesDatabasePrecision(t *testing.T) {
	aggregateID := uuid.New()
	recipientUserID := uuid.New()
	occurrence := time.Date(2026, 5, 1, 10, 0, 0, 123456789, time.UTC)

	// PostgreSQL stores timestamptz at microsecond precision, so the key must
	// not depend on the sub-microsecond digits: a value read back from the
	// database has to reproduce the key of the value it was written from.
	assert.Equal(t,
		NotificationDedupKeyWithOccurrence(NotificationKindActivityStartingSoon, aggregateID, occurrence, recipientUserID),
		NotificationDedupKeyWithOccurrence(NotificationKindActivityStartingSoon, aggregateID, occurrence.Truncate(time.Microsecond), recipientUserID),
	)
}

func TestNotificationKindIsRecurring(t *testing.T) {
	// Spec §6.5 names starting_soon and deadline_approaching; the remaining
	// cron-triggered reminders and the manual deadline reminder repeat the same
	// way.
	for _, kind := range []string{
		NotificationKindActivityStuckWarning,
		NotificationKindActivityStartingSoon,
		NotificationKindFeedbackDeadlineApproaching,
		NotificationKindFeedbackDeadlinePassed,
		NotificationKindInactivityNoNextActivity,
		NotificationKindInactivityInviteLinkExpiring,
		NotificationKindInactivityNoMemberJoined,
		NotificationKindInactivityNotJoinedGuild,
	} {
		assert.True(t, NotificationKindIsRecurring(kind), "%s must use an occurrence-qualified key", kind)
	}

	for _, kind := range []string{
		NotificationKindActivityScheduled,
		NotificationKindActivityUpdated,
		NotificationKindActivityAborted,
		NotificationKindAnalysisStarted,
		NotificationKindAnalysisCompleted,
		NotificationKindYouTubeUploadCompleted,
		NotificationKindApplicationSubmitted,
		NotificationKindApplicationApproved,
		NotificationKindApplicationRejected,
		NotificationKindMemberJoined,
		NotificationKindMemberRemoved,
		NotificationKindFeedAnnouncementPosted,
		NotificationKindFeedCommentPosted,
	} {
		assert.False(t, NotificationKindIsRecurring(kind), "%s is a one-off event", kind)
	}

	assert.False(t, NotificationKindIsRecurring(""), "an unknown kind is never recurring")
}
