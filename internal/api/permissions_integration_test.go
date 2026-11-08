package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file covers the §1.3 permission helpers against real PostgreSQL rows
// (#69 harness) and the `RequireActivityHost` HOST check against a fake hub.
//
// Every call goes through a probe handler that follows the documented pattern:
// the helper writes the §1.5 envelope on failure and the handler returns early.
// The probe sets X-Handler-Continued only when the helper passed, so a failed
// check proves the handler wrote nothing further.

const helperContinuedHeader = "X-Handler-Continued"

// permissionFixture is one migrated schema holding every relationship the
// checks distinguish: leader, active member, left member, outsider, an admin
// and a staff user who are ordinary members of another guild, activities in
// two guilds, and two mistakes/selections with different owners.
type permissionFixture struct {
	db  *gorm.DB
	api *API
	hub *fakeHostHub

	guild      *domain.Guild
	otherGuild *domain.Guild

	leader     *domain.User
	member     *domain.User
	leftMember *domain.User
	outsider   *domain.User
	admin      *domain.User
	staff      *domain.User

	practiceActivity *domain.Activity
	reviewActivity   *domain.Activity
	doneActivity     *domain.Activity
	otherActivity    *domain.Activity

	practiceEvent *domain.Event

	ownMistake  *domain.Mistake
	peerMistake *domain.Mistake

	ownSelection  *domain.SelectedSentence
	peerSelection *domain.SelectedSentence
}

func newPermissionFixture(t *testing.T) *permissionFixture {
	t.Helper()

	db := testutil.NewPostgresDB(t)
	api := NewAPI("", "", &http.Transport{}, db, "", nil)
	t.Cleanup(api.Close)

	f := &permissionFixture{db: db, api: api, hub: newFakeHostHub()}
	// permissions() reads the hub live, so the fake replaces the real Hub for
	// the HOST checks.
	api.webrtcHub = f.hub

	f.guild = f.newGuild(t, "permission guild")
	f.otherGuild = f.newGuild(t, "another guild")

	f.leader = f.newUser(t, domain.UserRoleUser)
	f.member = f.newUser(t, domain.UserRoleUser)
	f.leftMember = f.newUser(t, domain.UserRoleUser)
	f.outsider = f.newUser(t, domain.UserRoleUser)
	f.admin = f.newUser(t, domain.UserRoleAdmin)
	f.staff = f.newUser(t, domain.UserRoleStaff)

	f.join(t, f.guild.ID, f.leader.ID, domain.GuildAttendeeRoleMaster, false)
	f.join(t, f.guild.ID, f.member.ID, domain.GuildAttendeeRoleMember, false)
	f.join(t, f.guild.ID, f.leftMember.ID, domain.GuildAttendeeRoleMember, true)
	// The admin and the staff user are plain members of a different guild: they
	// hold no standing in the guild under test, and being an admin/staff must
	// not create any.
	f.join(t, f.otherGuild.ID, f.admin.ID, domain.GuildAttendeeRoleMember, false)
	f.join(t, f.otherGuild.ID, f.staff.ID, domain.GuildAttendeeRoleMember, false)

	f.practiceActivity = f.newActivity(t, f.guild.ID, 1, domain.ActivityStatusInPractice)
	f.reviewActivity = f.newActivity(t, f.guild.ID, 2, domain.ActivityStatusInReview)
	f.doneActivity = f.newActivity(t, f.guild.ID, 3, domain.ActivityStatusDone)
	f.otherActivity = f.newActivity(t, f.otherGuild.ID, 1, domain.ActivityStatusInPractice)

	f.practiceEvent = f.newEvent(t, f.practiceActivity.ID)
	f.ownMistake = f.newMistake(t, f.practiceEvent.ID, f.member.ID)
	f.peerMistake = f.newMistake(t, f.practiceEvent.ID, f.leader.ID)
	f.ownSelection = f.newSelection(t, f.practiceActivity.ID, f.ownMistake.ID, f.member.ID)
	f.peerSelection = f.newSelection(t, f.practiceActivity.ID, f.peerMistake.ID, f.leader.ID)

	return f
}

func (f *permissionFixture) newUser(t *testing.T, role domain.UserRole) *domain.User {
	t.Helper()

	user := &domain.User{
		ID:    uuid.New(),
		Email: uuid.NewString() + "@example.com",
		Name:  "permission fixture " + string(role),
		Role:  role,
	}
	require.NoError(t, f.db.Create(user).Error)
	return user
}

func (f *permissionFixture) newGuild(t *testing.T, name string) *domain.Guild {
	t.Helper()

	guild := &domain.Guild{ID: uuid.New(), Name: name}
	require.NoError(t, f.db.Create(guild).Error)
	return guild
}

func (f *permissionFixture) join(t *testing.T, guildID, userID uuid.UUID, role domain.GuildAttendeeRole, left bool) {
	t.Helper()

	joinedAt := time.Now().UTC().Add(-time.Hour)
	attendee := &domain.GuildAttendee{
		ID:       uuid.New(),
		GuildID:  guildID,
		UserID:   userID,
		Role:     role,
		JoinedAt: &joinedAt,
	}
	if left {
		leftAt := time.Now().UTC()
		attendee.LeftAt = &leftAt
	}
	require.NoError(t, f.db.Create(attendee).Error)
}

func (f *permissionFixture) newActivity(t *testing.T, guildID uuid.UUID, sequence int, status domain.ActivityStatus) *domain.Activity {
	t.Helper()

	activity := &domain.Activity{
		ID:             uuid.New(),
		GuildID:        guildID,
		SequenceNumber: sequence,
		Status:         status,
		Mode:           domain.ActivityModeReport,
	}
	require.NoError(t, f.db.Create(activity).Error)
	return activity
}

func (f *permissionFixture) newEvent(t *testing.T, activityID uuid.UUID) *domain.Event {
	t.Helper()

	event := &domain.Event{
		ID:         uuid.New(),
		Title:      "permission fixture practice",
		StartTime:  time.Now().UTC(),
		ActivityID: activityID,
		Kind:       domain.EventKindPractice,
		Mode:       domain.EventModeReport,
	}
	require.NoError(t, f.db.Create(event).Error)
	return event
}

func (f *permissionFixture) newMistake(t *testing.T, eventID, userID uuid.UUID) *domain.Mistake {
	t.Helper()

	mistake := &domain.Mistake{
		ID:         uuid.New(),
		EventID:    eventID,
		UserID:     userID,
		OriginText: "origin",
		FixedText:  "fixed",
	}
	require.NoError(t, f.db.Create(mistake).Error)
	return mistake
}

func (f *permissionFixture) newSelection(t *testing.T, activityID, mistakeID, userID uuid.UUID) *domain.SelectedSentence {
	t.Helper()

	selected := &domain.SelectedSentence{
		ID:               uuid.New(),
		ActivityID:       activityID,
		MistakeID:        mistakeID,
		SelectedByUserID: userID,
	}
	require.NoError(t, f.db.Create(selected).Error)
	return selected
}

// fakeHostHub records which channel the HOST check asked about. The rest of
// domain.WebRTCHub is unused by the permission checks and stays inert.
type fakeHostHub struct {
	hosts map[string]uuid.UUID
	asked []string
}

func newFakeHostHub() *fakeHostHub {
	return &fakeHostHub{hosts: map[string]uuid.UUID{}}
}

func (h *fakeHostHub) AddClient(*domain.Client)                {}
func (h *fakeHostHub) RemoveClient(string)                     {}
func (h *fakeHostHub) GetClient(string) (*domain.Client, bool) { return nil, false }
func (h *fakeHostHub) ListUsers() []domain.OnlineUser          { return nil }
func (h *fakeHostHub) BroadcastExcept(string, string, any)     {}

func (h *fakeHostHub) GetHost(channelID string) (uuid.UUID, bool) {
	h.asked = append(h.asked, channelID)
	host, ok := h.hosts[channelID]
	return host, ok
}

func (h *fakeHostHub) setHost(channelID string, userID uuid.UUID) {
	h.hosts[channelID] = userID
}

func (h *fakeHostHub) clearHost(channelID string) {
	delete(h.hosts, channelID)
}

func (h *fakeHostHub) reset() {
	h.hosts = map[string]uuid.UUID{}
	h.asked = nil
}

// probeRequest mounts check behind a handler that follows the §1.3 pattern:
// the helper writes the envelope and the handler returns without writing
// anything else. X-Handler-Continued marks that the helper passed.
func probeRequest(t *testing.T, caller *domain.User, check func(*gin.Context) error) *httptest.ResponseRecorder {
	t.Helper()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		if caller != nil {
			c.Set("user", caller)
		}
	})
	router.GET("/probe", func(c *gin.Context) {
		if err := check(c); err != nil {
			return
		}
		c.Header(helperContinuedHeader, "true")
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe", nil))
	return rec
}

// assertProbeOutcome checks the single response the probe produced. A denied
// call must carry exactly one §1.5 envelope and no trace of the handler's own
// success path.
func assertProbeOutcome(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	require.Equal(t, wantStatus, rec.Code, rec.Body.String())

	if wantCode == "" {
		assert.Equal(t, "true", rec.Header().Get(helperContinuedHeader))
		assert.Empty(t, rec.Body.String())
		return
	}

	assert.Empty(t, rec.Header().Get(helperContinuedHeader),
		"the handler must return immediately once the helper failed")
	env := decodeEnvelope(t, rec) // also asserts the body is exactly one envelope
	assert.Equal(t, wantCode, env.Error.Code)
	assert.Empty(t, env.Error.Details)
}

type helperProbe struct {
	name       string
	caller     *domain.User // nil means the middleware attached no caller
	check      func(*gin.Context) error
	wantStatus int
	wantCode   string // empty means the helper allowed the call
}

func runHelperProbes(t *testing.T, probes []helperProbe) {
	t.Helper()

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			rec := probeRequest(t, probe.caller, probe.check)
			assertProbeOutcome(t, rec, probe.wantStatus, probe.wantCode)
		})
	}
}

// TestPermissionHelpers covers GUILD, LEADER, ACTIVITY and the user-scoped
// OWNER check against real rows.
func TestPermissionHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newPermissionFixture(t)

	probes := []helperProbe{
		// GUILD
		{"guild member is allowed", f.member, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, http.StatusNoContent, ""},
		{"guild leader is a member too", f.leader, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, http.StatusNoContent, ""},
		{"left member is not a member", f.leftMember, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"non-member is denied", f.outsider, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"missing guild is a 404", f.member, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},

		// LEADER
		{"leader is allowed", f.leader, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.guild.ID)
		}, http.StatusNoContent, ""},
		{"plain member is not a leader", f.member, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"left member is not a leader", f.leftMember, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"leader of another guild is denied", f.leader, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.otherGuild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"missing guild is a 404", f.leader, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},

		// ACTIVITY
		{"member of the activity's guild is allowed", f.member, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.practiceActivity.ID)
		}, http.StatusNoContent, ""},
		{"leader of the activity's guild is allowed", f.leader, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.reviewActivity.ID)
		}, http.StatusNoContent, ""},
		{"left member is denied", f.leftMember, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.practiceActivity.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"non-member is denied", f.outsider, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.practiceActivity.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"activity in another guild is denied", f.leader, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.otherActivity.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"missing activity is a 404", f.member, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},

		// OWNER (self or guild leader)
		{"self is allowed", f.outsider, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.outsider.ID, f.guild.ID)
		}, http.StatusNoContent, ""},
		{"self is allowed without a guild", f.outsider, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.outsider.ID, uuid.Nil)
		}, http.StatusNoContent, ""},
		{"leader acting for a member is allowed", f.leader, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.member.ID, f.guild.ID)
		}, http.StatusNoContent, ""},
		{"plain member acting for another is denied", f.member, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.leader.ID, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"non-member acting for another is denied", f.outsider, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.member.ID, f.guild.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"acting for another outside a guild is denied", f.member, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.leader.ID, uuid.Nil)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"missing guild is a 404", f.leader, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.member.ID, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},
	}

	runHelperProbes(t, probes)
}

// TestPermissionOwnershipHelpers covers the two record-scoped OWNER checks:
// NOT_MISTAKE_OWNER for a mistake and the generic 403 for a selected sentence.
func TestPermissionOwnershipHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newPermissionFixture(t)

	probes := []helperProbe{
		{"mistake owner is allowed", f.member, func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, f.ownMistake.ID)
		}, http.StatusNoContent, ""},
		{"someone else's mistake is NOT_MISTAKE_OWNER", f.member, func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, f.peerMistake.ID)
		}, http.StatusForbidden, "NOT_MISTAKE_OWNER"},
		{"the guild leader is not the owner", f.leader, func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, f.ownMistake.ID)
		}, http.StatusForbidden, "NOT_MISTAKE_OWNER"},
		{"missing mistake is a 404", f.member, func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},

		{"selected sentence owner is allowed", f.member, func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, f.ownSelection.ID)
		}, http.StatusNoContent, ""},
		{"someone else's selected sentence is forbidden", f.member, func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, f.peerSelection.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"the guild leader is not the selection owner", f.leader, func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, f.ownSelection.ID)
		}, http.StatusForbidden, "FORBIDDEN"},
		{"missing selected sentence is a 404", f.member, func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, uuid.New())
		}, http.StatusNotFound, "NOT_FOUND"},
	}

	runHelperProbes(t, probes)
}

// TestRequireActivityHost covers the HOST marker with a fake hub: host is the
// caller, host is another user, no host at all, and the practice/review channel
// selection of §4.2.
func TestRequireActivityHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newPermissionFixture(t)

	practiceChannel := practiceChannelPrefix + f.practiceActivity.ID.String()
	reviewChannel := reviewChannelPrefix + f.reviewActivity.ID.String()
	doneChannel := reviewChannelPrefix + f.doneActivity.ID.String()

	tests := []struct {
		name        string
		caller      *domain.User
		activityID  uuid.UUID
		arrange     func(*fakeHostHub)
		wantStatus  int
		wantCode    string
		wantChannel string // channel the hub must have been asked about; "" for none
	}{
		{
			name: "practice room host is allowed", caller: f.member, activityID: f.practiceActivity.ID,
			arrange:     func(h *fakeHostHub) { h.setHost(practiceChannel, f.member.ID) },
			wantStatus:  http.StatusNoContent,
			wantChannel: practiceChannel,
		},
		{
			name: "review room host is allowed", caller: f.member, activityID: f.reviewActivity.ID,
			arrange:     func(h *fakeHostHub) { h.setHost(reviewChannel, f.member.ID) },
			wantStatus:  http.StatusNoContent,
			wantChannel: reviewChannel,
		},
		{
			name: "host of another user is denied", caller: f.member, activityID: f.practiceActivity.ID,
			arrange:     func(h *fakeHostHub) { h.setHost(practiceChannel, f.leader.ID) },
			wantStatus:  http.StatusForbidden,
			wantCode:    "FORBIDDEN",
			wantChannel: practiceChannel,
		},
		{
			name: "no host is denied", caller: f.member, activityID: f.practiceActivity.ID,
			wantStatus:  http.StatusForbidden,
			wantCode:    "FORBIDDEN",
			wantChannel: practiceChannel,
		},
		{
			name: "nil host is denied", caller: f.member, activityID: f.practiceActivity.ID,
			arrange:     func(h *fakeHostHub) { h.setHost(practiceChannel, uuid.Nil) },
			wantStatus:  http.StatusForbidden,
			wantCode:    "FORBIDDEN",
			wantChannel: practiceChannel,
		},
		{
			name: "practice host does not govern the review room", caller: f.member, activityID: f.reviewActivity.ID,
			arrange:     func(h *fakeHostHub) { h.setHost(practiceChannel, f.member.ID) },
			wantStatus:  http.StatusForbidden,
			wantCode:    "FORBIDDEN",
			wantChannel: reviewChannel,
		},
		{
			name: "a terminal activity has no room", caller: f.member, activityID: f.doneActivity.ID,
			arrange:    func(h *fakeHostHub) { h.setHost(doneChannel, f.member.ID) },
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
		{
			name: "missing activity is a 404", caller: f.member, activityID: uuid.New(),
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f.hub.reset()
			if tc.arrange != nil {
				tc.arrange(f.hub)
			}

			rec := probeRequest(t, tc.caller, func(c *gin.Context) error {
				return f.api.RequireActivityHost(c, tc.activityID)
			})
			assertProbeOutcome(t, rec, tc.wantStatus, tc.wantCode)

			if tc.wantChannel == "" {
				assert.Empty(t, f.hub.asked, "the hub must not be consulted")
				return
			}
			assert.Equal(t, []string{tc.wantChannel}, f.hub.asked)
		})
	}
}

// TestPermissionHelpersRequireCaller pins the 401 every helper answers without
// an attached caller, before any lookup happens.
func TestPermissionHelpersRequireCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newPermissionFixture(t)

	probes := []helperProbe{
		{"RequireGuildMember", nil, func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireGuildLeader", nil, func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.guild.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireActivityMember", nil, func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.practiceActivity.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireSelfOrGuildLeader", nil, func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.member.ID, f.guild.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireMistakeOwner", nil, func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, f.ownMistake.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireSelectedSentenceOwner", nil, func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, f.ownSelection.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"RequireActivityHost", nil, func(c *gin.Context) error {
			return f.api.RequireActivityHost(c, f.practiceActivity.ID)
		}, http.StatusUnauthorized, "UNAUTHORIZED"},
	}

	runHelperProbes(t, probes)
}

// TestPermissionHelpersDenyAdminsWithoutStanding is the no-global-admin-bypass
// criterion: an admin and a staff user who are members only of another guild,
// own nothing here and hold no host role are denied by every helper.
func TestPermissionHelpersDenyAdminsWithoutStanding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := newPermissionFixture(t)

	hostChannel := practiceChannelPrefix + f.practiceActivity.ID.String()

	helpers := []struct {
		name     string
		check    func(*gin.Context) error
		wantCode string
	}{
		{"RequireGuildMember", func(c *gin.Context) error {
			return f.api.RequireGuildMember(c, f.guild.ID)
		}, "FORBIDDEN"},
		{"RequireGuildLeader", func(c *gin.Context) error {
			return f.api.RequireGuildLeader(c, f.guild.ID)
		}, "FORBIDDEN"},
		{"RequireActivityMember", func(c *gin.Context) error {
			return f.api.RequireActivityMember(c, f.practiceActivity.ID)
		}, "FORBIDDEN"},
		{"RequireSelfOrGuildLeader", func(c *gin.Context) error {
			return f.api.RequireSelfOrGuildLeader(c, f.member.ID, f.guild.ID)
		}, "FORBIDDEN"},
		{"RequireMistakeOwner", func(c *gin.Context) error {
			return f.api.RequireMistakeOwner(c, f.ownMistake.ID)
		}, "NOT_MISTAKE_OWNER"},
		{"RequireSelectedSentenceOwner", func(c *gin.Context) error {
			return f.api.RequireSelectedSentenceOwner(c, f.ownSelection.ID)
		}, "FORBIDDEN"},
		{"RequireActivityHost", func(c *gin.Context) error {
			return f.api.RequireActivityHost(c, f.practiceActivity.ID)
		}, "FORBIDDEN"},
	}

	for _, caller := range []struct {
		name string
		user *domain.User
	}{
		{"admin", f.admin},
		{"staff", f.staff},
	} {
		for _, helper := range helpers {
			t.Run(caller.name+" "+helper.name, func(t *testing.T) {
				f.hub.reset()
				f.hub.setHost(hostChannel, f.member.ID)

				rec := probeRequest(t, caller.user, helper.check)
				assertProbeOutcome(t, rec, http.StatusForbidden, helper.wantCode)
			})
		}
	}
}

// TestActivityRoomChannel pins the documented §4.2 channel selection rule.
func TestActivityRoomChannel(t *testing.T) {
	activityID := uuid.New()

	tests := []struct {
		status  domain.ActivityStatus
		want    string
		wantSet bool
	}{
		{domain.ActivityStatusPendingPractice, practiceChannelPrefix + activityID.String(), true},
		{domain.ActivityStatusInPractice, practiceChannelPrefix + activityID.String(), true},
		{domain.ActivityStatusAnalyzing, practiceChannelPrefix + activityID.String(), true},
		{domain.ActivityStatusInFeedback, reviewChannelPrefix + activityID.String(), true},
		{domain.ActivityStatusInReview, reviewChannelPrefix + activityID.String(), true},
		{domain.ActivityStatusDone, "", false},
		{domain.ActivityStatusAborted, "", false},
		{domain.ActivityStatus(""), "", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			channel, ok := ActivityRoomChannel(activityID, tc.status)
			assert.Equal(t, tc.wantSet, ok)
			assert.Equal(t, tc.want, channel)
		})
	}
}

// TestCheckChannelHostWithoutHub proves the core HOST check fails closed when
// the Hub is absent, which is what #80 relies on for an uninitialized hub.
func TestCheckChannelHostWithoutHub(t *testing.T) {
	caller := &domain.User{ID: uuid.New()}
	perms := NewPermissions(nil, nil, nil, nil, nil, nil)

	err := perms.CheckChannelHost(caller, practiceChannelPrefix+uuid.NewString())
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrForbidden)
}
