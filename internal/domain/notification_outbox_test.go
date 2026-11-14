package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	// cron-triggered reminders, the manual deadline reminder and
	// activity.updated (sent again for every change) repeat the same way.
	for _, kind := range []string{
		NotificationKindActivityUpdated,
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
		NotificationKindActivityAborted,
		NotificationKindActivityPracticeStarted,
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

// TestNotificationDedupKeyRejectsTheWrongBuilder pins the guard: the one-off
// builder refuses every recurring kind and the occurrence builder refuses every
// kind that is not recurring, so a recurring notification cannot be written
// with a key that would drop its next occurrence.
func TestNotificationDedupKeyRejectsTheWrongBuilder(t *testing.T) {
	aggregateID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	recipientUserID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	occurrence := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

	assert.NotEmpty(t, recurringNotificationKinds)
	for kind := range recurringNotificationKinds {
		assert.Panics(t, func() { NotificationDedupKey(kind, aggregateID, recipientUserID) },
			"%s is recurring and must not accept the one-off key", kind)
		assert.NotPanics(t, func() {
			NotificationDedupKeyWithOccurrence(kind, aggregateID, occurrence, recipientUserID)
		}, "%s is recurring and must accept the occurrence key", kind)
	}

	for _, kind := range []string{
		NotificationKindActivityScheduled,
		NotificationKindActivityPracticeStarted,
		NotificationKindMemberJoined,
		NotificationKindFeedCommentPosted,
		"not.a.known.kind",
	} {
		assert.Panics(t, func() {
			NotificationDedupKeyWithOccurrence(kind, aggregateID, occurrence, recipientUserID)
		}, "%s is not recurring and must not accept an occurrence", kind)
		assert.NotPanics(t, func() { NotificationDedupKey(kind, aggregateID, recipientUserID) },
			"%s is one-off and must accept the one-off key", kind)
	}
}

// notificationKindValues returns the value of every notification kind constant
// declared in the non-test files of this package, so the catalog tests below
// follow the real declarations instead of a list they carry themselves.
func notificationKindValues(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var values []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range valueSpec.Names {
					if !strings.HasPrefix(ident.Name, "NotificationKind") || i >= len(valueSpec.Values) {
						continue
					}
					lit, ok := valueSpec.Values[i].(*ast.BasicLit)
					require.True(t, ok, "notification kind %s must be a string literal", ident.Name)
					unquoted, err := strconv.Unquote(lit.Value)
					require.NoError(t, err)
					values = append(values, unquoted)
				}
			}
		}
	}
	sort.Strings(values)
	return values
}

// TestNotificationKindCatalogMatchesSpec pins the catalog to spec §5.4, which
// lists 22 notification kinds. It fails when a kind is added, renamed or
// dropped without the matching spec change, and when the same kind value is
// declared twice.
func TestNotificationKindCatalogMatchesSpec(t *testing.T) {
	assert.Equal(t, []string{
		"activity.aborted",
		"activity.practice_started",
		"activity.scheduled",
		"activity.starting_soon",
		"activity.stuck_warning",
		"activity.updated",
		"analysis.completed",
		"analysis.started",
		"application.approved",
		"application.rejected",
		"application.submitted",
		"feed.announcement_posted",
		"feed.comment_posted",
		"feedback.deadline_approaching",
		"feedback.deadline_passed",
		"inactivity.invite_link_expiring",
		"inactivity.no_member_joined",
		"inactivity.no_next_activity",
		"inactivity.not_joined_guild",
		"member.joined",
		"member.removed",
		"youtube.upload_completed",
	}, notificationKindValues(t), "spec §5.4 has exactly these 22 kinds")
}

// TestNotificationKindCatalogHasOneValidBuilder pins that every §5.4 kind
// belongs to exactly one class: a kind must not accept both dedup key builders
// (an ambiguous classification) and must not reject both (no key at all).
func TestNotificationKindCatalogHasOneValidBuilder(t *testing.T) {
	aggregateID := uuid.New()
	recipientUserID := uuid.New()
	occurrence := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)

	for _, kind := range notificationKindValues(t) {
		oneOffAccepts := !didPanic(func() {
			NotificationDedupKey(kind, aggregateID, recipientUserID)
		})
		recurringAccepts := !didPanic(func() {
			NotificationDedupKeyWithOccurrence(kind, aggregateID, occurrence, recipientUserID)
		})

		assert.True(t, oneOffAccepts != recurringAccepts,
			"%s must accept exactly one dedup key builder", kind)
		assert.Equal(t, NotificationKindIsRecurring(kind), recurringAccepts,
			"the builders and NotificationKindIsRecurring disagree about %s", kind)
	}
}

// didPanic reports whether call panicked, without recording a test failure.
func didPanic(call func()) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	call()
	return false
}
