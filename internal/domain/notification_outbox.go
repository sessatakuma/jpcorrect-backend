package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Notification kinds of the spec §5.4 event list. They are the `kind` column
// value of NotificationOutbox and Notification (§2.2).
const (
	NotificationKindActivityScheduled            = "activity.scheduled"
	NotificationKindActivityUpdated              = "activity.updated"
	NotificationKindActivityAborted              = "activity.aborted"
	NotificationKindActivityStuckWarning         = "activity.stuck_warning"
	NotificationKindActivityStartingSoon         = "activity.starting_soon"
	NotificationKindActivityPracticeStarted      = "activity.practice_started"
	NotificationKindAnalysisStarted              = "analysis.started"
	NotificationKindAnalysisCompleted            = "analysis.completed"
	NotificationKindYouTubeUploadCompleted       = "youtube.upload_completed"
	NotificationKindFeedbackDeadlineApproaching  = "feedback.deadline_approaching"
	NotificationKindFeedbackDeadlinePassed       = "feedback.deadline_passed"
	NotificationKindApplicationSubmitted         = "application.submitted"
	NotificationKindApplicationApproved          = "application.approved"
	NotificationKindApplicationRejected          = "application.rejected"
	NotificationKindMemberJoined                 = "member.joined"
	NotificationKindMemberRemoved                = "member.removed"
	NotificationKindFeedAnnouncementPosted       = "feed.announcement_posted"
	NotificationKindFeedCommentPosted            = "feed.comment_posted"
	NotificationKindInactivityNoNextActivity     = "inactivity.no_next_activity"
	NotificationKindInactivityInviteLinkExpiring = "inactivity.invite_link_expiring"
	NotificationKindInactivityNoMemberJoined     = "inactivity.no_member_joined"
	NotificationKindInactivityNotJoinedGuild     = "inactivity.not_joined_guild"
)

// recurringNotificationKinds lists the §5.4 kinds whose dedup key must carry an
// occurrence (spec §6.5, "recurring / reschedulable events"). They are the
// notifications that a reschedule, a later update or a later scan must be able
// to deliver again: the cron-triggered reminders, the manual feedback reminders
// and activity.updated, which is sent again for every change of an activity.
// Every other §5.4 kind describes a single state transition of its aggregate
// and uses the one-off key.
//
// The occurrence is the instant the notification is about — practice_at for
// activity.starting_soon, the feedback deadline for the feedback reminders, the
// reminder bucket for the inactivity scans, the update time (or the new
// schedule) for activity.updated — so a changed instant yields a new key
// instead of being suppressed by the old one.
//
// activity.practice_started is not in this set: it reports the single
// transition of one activity into progress, so it takes the one-off key.
var recurringNotificationKinds = map[string]struct{}{
	NotificationKindActivityUpdated:              {},
	NotificationKindActivityStuckWarning:         {},
	NotificationKindActivityStartingSoon:         {},
	NotificationKindFeedbackDeadlineApproaching:  {},
	NotificationKindFeedbackDeadlinePassed:       {},
	NotificationKindInactivityNoNextActivity:     {},
	NotificationKindInactivityInviteLinkExpiring: {},
	NotificationKindInactivityNoMemberJoined:     {},
	NotificationKindInactivityNotJoinedGuild:     {},
}

// NotificationKindIsRecurring reports whether kind is a recurring or
// reschedulable reminder, that is, whether its dedup key must be built with
// NotificationDedupKeyWithOccurrence instead of NotificationDedupKey (spec
// §6.5). It is case-sensitive: kinds are the exact §5.4 strings.
func NotificationKindIsRecurring(kind string) bool {
	_, recurring := recurringNotificationKinds[kind]
	return recurring
}

// notificationOccurrenceLayout is the one canonical rendering of a dedup-key
// occurrence: UTC, fixed width, microsecond precision. Microseconds are
// PostgreSQL's timestamp precision, so a value read back from the database
// reproduces the same key as the in-memory value it was written from.
const notificationOccurrenceLayout = "2006-01-02T15:04:05.000000Z"

// NotificationOutboxRepository is the write side of NotificationOutbox (spec
// §2.2, §6.5). It only writes intents; claiming, dispatching, retrying and
// delivering them belong to the dispatcher, which is not part of this
// repository.
//
// Write intents through the RepoSet of a Store.Tx callback, in the same
// transaction as the state change that causes them (spec §1.4, §6.5). Never
// send a WebSocket message, a push notification or an email inside that
// transaction: those are not transactional resources, so a later rollback
// could not undo them. Sending belongs after Store.Tx has returned nil.
type NotificationOutboxRepository interface {
	// CreateIntents inserts one NotificationOutbox row per intent, in the
	// order given, with the initial dispatcher state status=pending,
	// attempt_count=0, next_attempt_at=NULL and last_error=NULL.
	//
	// The initial state is forced, not taken from the caller: a nonzero
	// status, attempt count, retry time or error on an intent is reset before
	// the insert, so a caller cannot create a row that the dispatcher skips
	// or delays. The reset and the generated primary key are written back to
	// the caller's intents.
	//
	// Only the intent row is written. A populated RecipientUser association
	// is ignored: it never inserts or updates a User row and never replaces
	// the scalar RecipientUserID.
	//
	// The insert is INSERT … ON CONFLICT (dedup_key) DO NOTHING: an intent
	// whose dedup key is already stored — because an earlier transaction,
	// the same transaction, or a concurrent one wrote it — is a successful
	// no-op, not an error. Intents therefore must carry a dedup key built by
	// NotificationDedupKey or NotificationDedupKeyWithOccurrence.
	//
	// It performs no dispatch: no WebSocket, push or email send and no worker
	// enqueue happens here or in the transaction that calls it (spec §1.7,
	// §6.5).
	CreateIntents(ctx context.Context, intents []*NotificationOutbox) error
}

// NotificationDedupKey returns the §6.5 dedup key of a one-off notification:
//
//	{kind}:{aggregate_id}:{recipient_user_id}
//
// Use it for every kind for which NotificationKindIsRecurring is false.
//
// It panics for a recurring kind. Such a key has no occurrence, so a second
// notification of the same kind for the same aggregate would be dropped as a
// duplicate by the unique dedup_key, silently and without any error. That is a
// programming error, so it fails at the first call instead of at runtime.
func NotificationDedupKey(kind string, aggregateID, recipientUserID uuid.UUID) string {
	if NotificationKindIsRecurring(kind) {
		panic(fmt.Sprintf("notification kind %q is recurring: build its dedup key with NotificationDedupKeyWithOccurrence", kind))
	}

	return fmt.Sprintf("%s:%s:%s", kind, aggregateID, recipientUserID)
}

// NotificationDedupKeyWithOccurrence returns the §6.5 dedup key of a recurring
// or reschedulable reminder:
//
//	{kind}:{aggregate_id}:{occurrence}:{recipient_user_id}
//
// occurrence is rendered in UTC with a fixed layout, so the same instant given
// in different time zones produces the same key, while a rescheduled occurrence
// produces a different key and therefore its own row. Use it for every kind for
// which NotificationKindIsRecurring is true.
//
// It panics for any other kind, including a kind that is not in §5.4: a
// one-off notification must not carry an occurrence, or a retried write would
// produce a second key and a second notification.
func NotificationDedupKeyWithOccurrence(kind string, aggregateID uuid.UUID, occurrence time.Time, recipientUserID uuid.UUID) string {
	if !NotificationKindIsRecurring(kind) {
		panic(fmt.Sprintf("notification kind %q is not recurring: build its dedup key with NotificationDedupKey", kind))
	}

	return fmt.Sprintf("%s:%s:%s:%s",
		kind,
		aggregateID,
		occurrence.UTC().Truncate(time.Microsecond).Format(notificationOccurrenceLayout),
		recipientUserID,
	)
}
