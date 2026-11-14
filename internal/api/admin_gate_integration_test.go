package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// This file is the behaviour half of #78's access policy (spec §1.3 "System
// administration and legacy raw CRUD", §3.15): the retained legacy raw CRUD
// routes admit only a stored User.Role == admin, in both gin modes, before the
// request body is read.
//
// Every route comes from routePolicies (route_policy_test.go) through
// systemAdminRoutes, so a newly classified admin route is covered by the
// matrix tests below without another list to maintain. The matrices use a real
// PostgreSQL schema (testutil) because the decision under test needs a stored
// role: the caller's own User.Role, never a JWT claim or a request body.
//
// The write tests and the read matrix prove that the gate changed nothing but
// access: an admin still reads every field the raw handlers exposed (including
// another user's Mistake.note / interacted_at) and still writes the same
// mutable fields, with FK/unique violations still mapped to the §1.5
// envelope.

// The raw bodies the matrix tests send. Every request carries a deliberately
// malformed body: whatever answers 403 never parsed it, and whatever answers
// 400 for an admin did.
const malformedJSONBody = `{"broken":`

// adminGateFixture is one migrated schema with every relationship the raw
// routes expose: an admin, a resource owner, a guild master, a staff user, a
// target user, a guild with a practice and a review Event, a mistake and a
// transcript owned by the owner, and one guild/event attendee row.
type adminGateFixture struct {
	db     *gorm.DB
	api    *API
	signer *testSigner

	admin, owner, staff, leader, target *domain.User

	guild       *domain.Guild
	activity    *domain.Activity
	practice    *domain.Event
	review      *domain.Event
	mistake     *domain.Mistake
	peerMistake *domain.Mistake
	transcript  *domain.Transcript

	guildAttendee  *domain.GuildAttendee
	masterAttendee *domain.GuildAttendee
	eventAttendee  *domain.EventAttendee
}

func newAdminGateFixture(t *testing.T) *adminGateFixture {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db := testutil.NewPostgresDB(t)

	api := NewAPI("", "", "", http.DefaultTransport.(*http.Transport).Clone(), db, "", nil)
	t.Cleanup(api.Close)

	signer := newTestSigner(t)
	signer.attachTo(t, api)

	f := &adminGateFixture{db: db, api: api, signer: signer}

	f.admin = f.newUser(t, domain.UserRoleAdmin)
	f.owner = f.newUser(t, domain.UserRoleUser)
	f.staff = f.newUser(t, domain.UserRoleStaff)
	f.leader = f.newUser(t, domain.UserRoleUser)
	f.target = f.newUser(t, domain.UserRoleUser)

	f.guild = f.newGuild(t, "admin gate guild")

	joinedAt := time.Now().UTC().Add(-time.Hour)
	f.masterAttendee = &domain.GuildAttendee{
		ID: uuid.New(), GuildID: f.guild.ID, UserID: f.leader.ID,
		Role: domain.GuildAttendeeRoleMaster, JoinedAt: &joinedAt,
	}
	f.guildAttendee = &domain.GuildAttendee{
		ID: uuid.New(), GuildID: f.guild.ID, UserID: f.owner.ID,
		Role: domain.GuildAttendeeRoleMember, JoinedAt: &joinedAt,
	}
	require.NoError(t, db.Create(f.masterAttendee).Error)
	require.NoError(t, db.Create(f.guildAttendee).Error)

	f.activity = &domain.Activity{
		ID: uuid.New(), GuildID: f.guild.ID, SequenceNumber: 1,
		Status: domain.ActivityStatusInPractice, Mode: domain.ActivityModeReport,
	}
	require.NoError(t, db.Create(f.activity).Error)

	note := "raw management note"
	f.practice = &domain.Event{
		ID: uuid.New(), Title: "admin gate practice", StartTime: time.Now().UTC(),
		ActivityID: f.activity.ID, Kind: domain.EventKindPractice,
		Mode: domain.EventModeReport, Note: &note,
	}
	f.review = &domain.Event{
		ID: uuid.New(), Title: "admin gate review", StartTime: time.Now().UTC(),
		ActivityID: f.activity.ID, Kind: domain.EventKindReview,
		Mode: domain.EventModeReview,
	}
	require.NoError(t, db.Create(f.practice).Error)
	require.NoError(t, db.Create(f.review).Error)

	interactedAt := time.Now().UTC().Add(-30 * time.Minute)
	peerNote := "owner-only personal note"
	f.mistake = &domain.Mistake{
		ID: uuid.New(), EventID: f.practice.ID, UserID: f.owner.ID,
		OriginText: "origin", FixedText: "fixed", Note: &peerNote, InteractedAt: &interactedAt,
	}
	f.peerMistake = &domain.Mistake{
		ID: uuid.New(), EventID: f.practice.ID, UserID: f.leader.ID,
		OriginText: "leader origin", FixedText: "leader fixed", Note: &peerNote,
	}
	require.NoError(t, db.Create(f.mistake).Error)
	require.NoError(t, db.Create(f.peerMistake).Error)

	f.transcript = &domain.Transcript{
		ID: uuid.New(), EventID: f.practice.ID, UserID: f.owner.ID,
		Content: "raw transcript", Accent: datatypes.JSON([]byte(`{"accent":"tokyo"}`)),
		Note: &peerNote,
	}
	require.NoError(t, db.Create(f.transcript).Error)

	f.eventAttendee = &domain.EventAttendee{
		ID: uuid.New(), EventID: f.practice.ID, UserID: f.owner.ID,
		Role: domain.EventAttendeeRoleMember, JoinedAt: &joinedAt,
	}
	require.NoError(t, db.Create(f.eventAttendee).Error)

	return f
}

func (f *adminGateFixture) newUser(t *testing.T, role domain.UserRole) *domain.User {
	t.Helper()

	sub := uuid.New()
	user := &domain.User{
		ID:             uuid.New(),
		Email:          uuid.NewString() + "@example.com",
		Name:           "admin gate " + string(role),
		Role:           role,
		Status:         domain.UserStatusActive,
		SupabaseUserID: &sub,
	}
	require.NoError(t, f.db.Create(user).Error)
	return user
}

func (f *adminGateFixture) newGuild(t *testing.T, name string) *domain.Guild {
	t.Helper()

	guild := &domain.Guild{ID: uuid.New(), Name: name}
	require.NoError(t, f.db.Create(guild).Error)
	return guild
}

// token returns the Authorization header value for user: a signed Supabase JWT
// whose subject resolves to that stored row, exactly like a real caller.
func (f *adminGateFixture) token(t *testing.T, user *domain.User) string {
	t.Helper()

	require.NotNil(t, user.SupabaseUserID, "fixture users always carry a sub")
	return "Bearer " + f.signer.userToken(t, *user.SupabaseUserID, user.Email)
}

// routerFor mounts Register in mode. Each mode gets a fresh engine because the
// route set (Swagger) and the middleware chain depend on it.
func (f *adminGateFixture) routerFor(t *testing.T, mode string) *gin.Engine {
	t.Helper()

	gin.SetMode(mode)
	router := gin.New()
	Register(router, f.api)
	return router
}

// do serves one request through the real router.
func (f *adminGateFixture) do(t *testing.T, router *gin.Engine, method, path, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// assertEnvelopeCode asserts status and the §1.5 code of a rejected request.
func assertEnvelopeCode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	require.Equal(t, wantStatus, rec.Code, rec.Body.String())
	env := decodeEnvelope(t, rec)
	assert.Equal(t, wantCode, env.Error.Code)
	assert.Empty(t, env.Error.Details)
}

// TestSystemAdminRoutesRejectNonAdmins covers the first acceptance criterion:
// every listed route answers 403 for an authenticated plain user, the resource
// owner, a guild master and a staff user, and 401 without a JWT or with an
// invalid one, in both gin modes.
//
// Every request carries a malformed body, so a 403 also proves the gate decided
// before the handler parsed anything: a body-reading route would answer 400.
func TestSystemAdminRoutesRejectNonAdmins(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	adminClaimToken := "Bearer " + f.signer.rawToken(t, jwt.MapClaims{
		"sub":   *f.owner.SupabaseUserID,
		"email": f.owner.Email,
		"role":  "admin", // a custom claim must never grant access
	})

	callers := []struct {
		name          string
		authorization string
		wantStatus    int
		wantCode      string
	}{
		{"no jwt", "", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"invalid jwt", "Bearer not-a-jwt", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"resource owner", f.token(t, f.owner), http.StatusForbidden, "FORBIDDEN"},
		{"guild master", f.token(t, f.leader), http.StatusForbidden, "FORBIDDEN"},
		{"staff", f.token(t, f.staff), http.StatusForbidden, "FORBIDDEN"},
		{"custom admin claim", adminClaimToken, http.StatusForbidden, "FORBIDDEN"},
	}

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			router := f.routerFor(t, mode)

			for _, key := range systemAdminRoutes() {
				method, pattern := splitRouteKey(t, key)
				path := concretePath(t, pattern)

				for _, caller := range callers {
					t.Run(key+" as "+caller.name, func(t *testing.T) {
						rec := f.do(t, router, method, path, caller.authorization, malformedJSONBody)
						assertEnvelopeCode(t, rec, caller.wantStatus, caller.wantCode)
					})
				}
			}
		})
	}
}

// TestSystemAdminRoutesAdmitAdmins is the admin half of the matrix: with a
// stored admin role every listed route gets past the gate, so the handler
// answers on its own terms (a malformed body is a 400 for a create, a missing
// row is a 404 for a read/update/delete) and never 401/403.
func TestSystemAdminRoutesAdmitAdmins(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	authorization := f.token(t, f.admin)

	// A fresh UUID addresses no row, so a create answers on its malformed body
	// (400), a single-resource read/update/delete on the missing row (404), and
	// a by-event/by-user/by-guild list on an empty array (200).
	byParamSegment := func(pattern string) bool {
		return strings.Contains(pattern, "/event/") ||
			strings.Contains(pattern, "/user/") ||
			strings.Contains(pattern, "/guild/")
	}

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			router := f.routerFor(t, mode)

			for _, key := range systemAdminRoutes() {
				method, pattern := splitRouteKey(t, key)

				t.Run(key, func(t *testing.T) {
					rec := f.do(t, router, method, concretePath(t, pattern), authorization, malformedJSONBody)

					switch {
					case method == http.MethodPost:
						assertEnvelopeCode(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
					case method == http.MethodGet && byParamSegment(pattern):
						require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
						assert.JSONEq(t, `[]`, rec.Body.String())
					default:
						assertEnvelopeCode(t, rec, http.StatusNotFound, "NOT_FOUND")
					}
				})
			}
		})
	}
}

// TestNonAdminCannotWriteUsers is the self-promotion criterion: a non-admin
// cannot reach raw POST / PUT /v1/users at all, so no request body it sends can
// create an admin or change a stored role.
func TestNonAdminCannotWriteUsers(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	router := f.routerFor(t, gin.ReleaseMode)

	var usersBefore int64
	require.NoError(t, f.db.Model(&domain.User{}).Count(&usersBefore).Error)

	createBody := fmt.Sprintf(`{"email":%q,"name":"promoted","role":"admin"}`, uuid.NewString()+"@example.com")
	rec := f.do(t, router, http.MethodPost, "/v1/users", f.token(t, f.owner), createBody)
	assertEnvelopeCode(t, rec, http.StatusForbidden, "FORBIDDEN")

	updateBody := fmt.Sprintf(`{"email":%q,"name":"promoted","role":"admin"}`, f.target.Email)
	rec = f.do(t, router, http.MethodPut, "/v1/users/"+f.target.ID.String(), f.token(t, f.owner), updateBody)
	assertEnvelopeCode(t, rec, http.StatusForbidden, "FORBIDDEN")

	var usersAfter int64
	require.NoError(t, f.db.Model(&domain.User{}).Count(&usersAfter).Error)
	assert.Equal(t, usersBefore, usersAfter, "a rejected create wrote nothing")

	// The target row is untouched and nobody was promoted.
	var target domain.User
	require.NoError(t, f.db.First(&target, "id = ?", f.target.ID).Error)
	assert.Equal(t, domain.UserRoleUser, target.Role)

	var promoted int64
	require.NoError(t, f.db.Model(&domain.User{}).Where("role = ?", domain.UserRoleAdmin).Count(&promoted).Error)
	assert.Equal(t, int64(1), promoted, "only the fixture admin holds the admin role")

	// The admin may do the same write through the same route.
	rec = f.do(t, router, http.MethodPut, "/v1/users/"+f.target.ID.String(), f.token(t, f.admin), updateBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, f.db.First(&target, "id = ?", f.target.ID).Error)
	assert.Equal(t, domain.UserRoleAdmin, target.Role, "raw admin management keeps the model's mutable fields")
}

// TestAdminRawReadRoutes covers the raw-read criterion: an admin reads every
// listed raw route with all fields, including another user's personal
// Mistake.note and interacted_at, in both gin modes. That full access stays
// confined to these management routes; TestAdminHasNoProductBypass pins that
// the product routes still filter.
func TestAdminRawReadRoutes(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	authorization := f.token(t, f.admin)

	note := `"note":"owner-only personal note"`
	mistakeID := f.mistake.ID.String()
	transcriptID := f.transcript.ID.String()
	eventAttendeeID := f.eventAttendee.ID.String()
	guildAttendeeID := f.guildAttendee.ID.String()

	routes := []struct {
		name         string
		path         string
		wantContains []string
	}{
		{"mistake", "/v1/mistakes/" + mistakeID, []string{note, `"interacted_at":"`}},
		{"mistakes by event", "/v1/mistakes/event/" + f.practice.ID.String(), []string{note}},
		{"mistakes by user", "/v1/mistakes/user/" + f.owner.ID.String(), []string{mistakeID}},
		{"practice", "/v1/practices/" + f.practice.ID.String(), []string{`"note":"raw management note"`}},
		{"practices by user", "/v1/practices/user/" + f.owner.ID.String(), []string{f.practice.ID.String()}},
		{"guild attendee", "/v1/guild-attendees/" + guildAttendeeID, []string{guildAttendeeID}},
		{"guild attendees by guild", "/v1/guild-attendees/guild/" + f.guild.ID.String(), []string{guildAttendeeID, f.masterAttendee.ID.String()}},
		{"guild attendees by user", "/v1/guild-attendees/user/" + f.leader.ID.String(), []string{f.masterAttendee.ID.String()}},
		{"transcript", "/v1/transcripts/" + transcriptID, []string{note, `"accent":{"accent":"tokyo"}`}},
		{"transcripts by event", "/v1/transcripts/event/" + f.practice.ID.String(), []string{transcriptID}},
		{"transcripts by user", "/v1/transcripts/user/" + f.owner.ID.String(), []string{transcriptID}},
		{"event attendee", "/v1/event-attendees/" + eventAttendeeID, []string{eventAttendeeID}},
		{"event attendees by event", "/v1/event-attendees/event/" + f.practice.ID.String(), []string{eventAttendeeID}},
		{"event attendees by user", "/v1/event-attendees/user/" + f.owner.ID.String(), []string{eventAttendeeID}},
	}

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			router := f.routerFor(t, mode)

			for _, route := range routes {
				t.Run(route.name, func(t *testing.T) {
					rec := f.do(t, router, http.MethodGet, route.path, authorization, "")
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					body := rec.Body.String()
					for _, want := range route.wantContains {
						assert.Contains(t, body, want, "the admin raw read exposes every field")
					}
				})
			}
		})
	}
}

// TestAdminRawWriteSemantics is the regression test for the raw handlers: as an
// admin, each write route still creates, updates and deletes the same mutable
// fields, and existing unique / foreign-key violations still surface as the
// §1.5 envelope.
func TestAdminRawWriteSemantics(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	router := f.routerFor(t, gin.TestMode)
	authorization := f.token(t, f.admin)

	t.Run("users", func(t *testing.T) {
		email := uuid.NewString() + "@example.com"
		rec := f.do(t, router, http.MethodPost, "/v1/users", authorization,
			fmt.Sprintf(`{"email":%q,"name":"raw created","role":"staff"}`, email))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.User
		require.NoError(t, f.db.First(&created, "email = ?", email).Error)
		assert.Equal(t, domain.UserRoleStaff, created.Role, "raw admin create keeps the role field")

		rec = f.do(t, router, http.MethodPut, "/v1/users/"+created.ID.String(), authorization,
			fmt.Sprintf(`{"email":%q,"name":"raw updated","role":"user"}`, email))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, "raw updated", created.Name)
		assert.Equal(t, domain.UserRoleUser, created.Role)

		rec = f.do(t, router, http.MethodDelete, "/v1/users/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
		assert.ErrorIs(t, f.db.First(&domain.User{}, "id = ?", created.ID).Error, gorm.ErrRecordNotFound)
	})

	t.Run("mistakes", func(t *testing.T) {
		note := "created by admin"
		interactedAt := time.Now().UTC().Truncate(time.Second)
		rec := f.do(t, router, http.MethodPost, "/v1/mistakes", authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"origin_text":"o","fixed_text":"f","note":%q,"interacted_at":%q}`,
			f.practice.ID.String(), f.owner.ID.String(), note, interactedAt.Format(time.RFC3339)))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.Mistake
		require.NoError(t, f.db.First(&created, "origin_text = ?", "o").Error)
		require.NotNil(t, created.Note)
		assert.Equal(t, note, *created.Note)

		rec = f.do(t, router, http.MethodPut, "/v1/mistakes/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"origin_text":"o","fixed_text":"f2","note":"changed"}`,
			f.practice.ID.String(), f.owner.ID.String()))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, "f2", created.FixedText)
		require.NotNil(t, created.Note)
		assert.Equal(t, "changed", *created.Note)

		// This table carries no foreign key (spec §2.1 keeps Mistake's
		// event_id/user_id as scalar columns), so an unknown event_id is stored
		// as-is. Only the FK-bearing raw tables answer 409; see the practices
		// and guild-attendees subtests.
		rec = f.do(t, router, http.MethodDelete, "/v1/mistakes/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("practices", func(t *testing.T) {
		activity := &domain.Activity{
			ID: uuid.New(), GuildID: f.guild.ID, SequenceNumber: 9,
			Status: domain.ActivityStatusPendingPractice, Mode: domain.ActivityModeReport,
		}
		require.NoError(t, f.db.Create(activity).Error)

		rec := f.do(t, router, http.MethodPost, "/v1/practices", authorization, fmt.Sprintf(
			`{"title":"raw practice","start_time":%q,"activity_id":%q,"kind":"practice","mode":"report","note":"raw note"}`,
			time.Now().UTC().Format(time.RFC3339), activity.ID.String()))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.Event
		require.NoError(t, f.db.First(&created, "activity_id = ?", activity.ID).Error)
		require.NotNil(t, created.Note)
		assert.Equal(t, "raw note", *created.Note)

		rec = f.do(t, router, http.MethodPut, "/v1/practices/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"title":"raw practice updated","start_time":%q,"activity_id":%q,"kind":"practice","mode":"report"}`,
			time.Now().UTC().Format(time.RFC3339), activity.ID.String()))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, "raw practice updated", created.Title)

		// The (activity_id, kind) unique constraint is still enforced.
		rec = f.do(t, router, http.MethodPost, "/v1/practices", authorization, fmt.Sprintf(
			`{"title":"duplicate","start_time":%q,"activity_id":%q,"kind":"practice","mode":"report"}`,
			time.Now().UTC().Format(time.RFC3339), activity.ID.String()))
		assertEnvelopeCode(t, rec, http.StatusConflict, "CONFLICT")

		// The event.activity_id foreign key is still enforced.
		rec = f.do(t, router, http.MethodPost, "/v1/practices", authorization, fmt.Sprintf(
			`{"title":"orphan","start_time":%q,"activity_id":%q,"kind":"review","mode":"review"}`,
			time.Now().UTC().Format(time.RFC3339), uuid.NewString()))
		assertEnvelopeCode(t, rec, http.StatusConflict, "RELATED_RECORDS_EXIST")

		rec = f.do(t, router, http.MethodDelete, "/v1/practices/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("guild-attendees", func(t *testing.T) {
		joinedAt := time.Now().UTC().Format(time.RFC3339)
		rec := f.do(t, router, http.MethodPost, "/v1/guild-attendees", authorization, fmt.Sprintf(
			`{"guild_id":%q,"user_id":%q,"role":"member","joined_at":%q}`,
			f.guild.ID.String(), f.target.ID.String(), joinedAt))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.GuildAttendee
		require.NoError(t, f.db.First(&created, "user_id = ?", f.target.ID).Error)
		assert.Equal(t, domain.GuildAttendeeRoleMember, created.Role)

		// The target leads another guild. #123 retired the partial unique index
		// that rejected a second leadership, and issue #81 owns the replacement
		// locked query, so this promotion succeeds for now. Turn this back into
		// 409 GUILD_LIMIT_REACHED when #81 lands.
		other := f.newGuild(t, "another guild led by the target")
		require.NoError(t, f.db.Create(&domain.GuildAttendee{
			ID: uuid.New(), GuildID: other.ID, UserID: f.target.ID,
			Role: domain.GuildAttendeeRoleMaster,
		}).Error)

		rec = f.do(t, router, http.MethodPut, "/v1/guild-attendees/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"guild_id":%q,"user_id":%q,"role":"master","joined_at":%q}`,
			f.guild.ID.String(), f.target.ID.String(), joinedAt))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, domain.GuildAttendeeRoleMaster, created.Role,
			"the raw update still stores the promoted role; only the quota is pending")

		rec = f.do(t, router, http.MethodPut, "/v1/guild-attendees/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"guild_id":%q,"user_id":%q,"role":"member","joined_at":%q,"left_at":%q}`,
			f.guild.ID.String(), f.target.ID.String(), joinedAt, time.Now().UTC().Format(time.RFC3339)))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		// The (guild_id, user_id) unique constraint is still enforced.
		rec = f.do(t, router, http.MethodPost, "/v1/guild-attendees", authorization, fmt.Sprintf(
			`{"guild_id":%q,"user_id":%q,"role":"member"}`, f.guild.ID.String(), f.target.ID.String()))
		assertEnvelopeCode(t, rec, http.StatusConflict, "CONFLICT")

		// The guild_attendee.guild_id foreign key is still enforced.
		rec = f.do(t, router, http.MethodPost, "/v1/guild-attendees", authorization, fmt.Sprintf(
			`{"guild_id":%q,"user_id":%q,"role":"member"}`, uuid.NewString(), f.staff.ID.String()))
		assertEnvelopeCode(t, rec, http.StatusConflict, "RELATED_RECORDS_EXIST")

		rec = f.do(t, router, http.MethodDelete, "/v1/guild-attendees/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("transcripts", func(t *testing.T) {
		rec := f.do(t, router, http.MethodPost, "/v1/transcripts", authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"content":"raw","accent":{"accent":"osaka"},"note":"raw note"}`,
			f.practice.ID.String(), f.owner.ID.String()))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.Transcript
		require.NoError(t, f.db.First(&created, "content = ?", "raw").Error)
		require.NotNil(t, created.Note)
		assert.Equal(t, "raw note", *created.Note)
		assert.JSONEq(t, `{"accent":"osaka"}`, string(created.Accent), "the jsonb accent column round-trips")

		rec = f.do(t, router, http.MethodPut, "/v1/transcripts/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"content":"raw updated","accent":{"accent":"osaka"}}`,
			f.practice.ID.String(), f.owner.ID.String()))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, "raw updated", created.Content)

		// transcript has no foreign key on event_id (spec §2.1), so an unknown
		// event_id is stored as-is; see the practices subtest for an FK 409.
		rec = f.do(t, router, http.MethodDelete, "/v1/transcripts/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})

	t.Run("event-attendees", func(t *testing.T) {
		joinedAt := time.Now().UTC().Format(time.RFC3339)
		rec := f.do(t, router, http.MethodPost, "/v1/event-attendees", authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"role":"member","joined_at":%q}`,
			f.review.ID.String(), f.target.ID.String(), joinedAt))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var created domain.EventAttendee
		require.NoError(t, f.db.First(&created, "user_id = ?", f.target.ID).Error)
		assert.Equal(t, domain.EventAttendeeRoleMember, created.Role)

		rec = f.do(t, router, http.MethodPut, "/v1/event-attendees/"+created.ID.String(), authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"role":"emcee"}`, f.review.ID.String(), f.target.ID.String()))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		require.NoError(t, f.db.First(&created, "id = ?", created.ID).Error)
		assert.Equal(t, domain.EventAttendeeRoleEmcee, created.Role)

		// The (event_id, user_id) unique constraint is still enforced.
		rec = f.do(t, router, http.MethodPost, "/v1/event-attendees", authorization, fmt.Sprintf(
			`{"event_id":%q,"user_id":%q,"role":"member"}`, f.review.ID.String(), f.target.ID.String()))
		assertEnvelopeCode(t, rec, http.StatusConflict, "CONFLICT")

		rec = f.do(t, router, http.MethodDelete, "/v1/event-attendees/"+created.ID.String(), authorization, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	})
}

// TestAdminGuildDeleteIsGatedOnly covers the retained DELETE /v1/guilds/:id:
// the gate is new, the delete behavior is not. The §3.3.1 semantics stay as
// they are: a guild without attendees is deleted, a guild with attendees is
// refused with RELATED_RECORDS_EXIST, and a non-admin cannot delete at all.
func TestAdminGuildDeleteIsGatedOnly(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	router := f.routerFor(t, gin.ReleaseMode)

	empty := f.newGuild(t, "empty guild")
	rec := f.do(t, router, http.MethodDelete, "/v1/guilds/"+empty.ID.String(), f.token(t, f.admin), "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	var deleted domain.Guild
	err := f.db.First(&deleted, "id = ?", empty.ID).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "the retained delete keeps its soft-delete behavior")

	withMembers := f.newGuild(t, "guild with members")
	require.NoError(t, f.db.Create(&domain.GuildAttendee{
		ID: uuid.New(), GuildID: withMembers.ID, UserID: f.target.ID,
		Role: domain.GuildAttendeeRoleMember,
	}).Error)

	rec = f.do(t, router, http.MethodDelete, "/v1/guilds/"+withMembers.ID.String(), f.token(t, f.admin), "")
	assertEnvelopeCode(t, rec, http.StatusConflict, "RELATED_RECORDS_EXIST")

	rec = f.do(t, router, http.MethodDelete, "/v1/guilds/"+withMembers.ID.String(), f.token(t, f.leader), "")
	assertEnvelopeCode(t, rec, http.StatusForbidden, "FORBIDDEN")

	var stillThere domain.Guild
	require.NoError(t, f.db.First(&stillThere, "id = ?", withMembers.ID).Error,
		"a rejected delete leaves the guild in place")
}

// TestAdminHasNoProductBypass pins the other half of the §1.3 clarification:
// the admin gate is confined to the raw management routes. An admin who is not
// a member still fails a product route's membership check, and the product
// profile DTO still refuses to change a role.
func TestAdminHasNoProductBypass(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	router := f.routerFor(t, gin.ReleaseMode)
	authorization := f.token(t, f.admin)

	// The fixture admin leads no guild and is not a member of it, so the
	// interim invite-link product check (§3.3.3, #54/#81) rejects it even
	// though the caller is an admin.
	rec := f.do(t, router, http.MethodGet, "/v1/guilds/"+f.guild.ID.String()+"/invite-link", authorization, "")
	assertEnvelopeCode(t, rec, http.StatusForbidden, "FORBIDDEN")

	// PUT /v1/users/me is a product route with an allowlisted body: a role in
	// the body is ignored, admin caller or not.
	rec = f.do(t, router, http.MethodPut, "/v1/users/me", authorization, `{"name":"renamed","role":"user"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var stored domain.User
	require.NoError(t, f.db.First(&stored, "id = ?", f.admin.ID).Error)
	assert.Equal(t, domain.UserRoleAdmin, stored.Role, "the product DTO cannot change a role")
	assert.Equal(t, "renamed", stored.Name)
}

// TestPracticeCreateRequiresActivityAndKind covers Appendix E row 40 and the
// matching #78 acceptance criterion: the raw Event create must supply
// activity_id and kind, and a body that omits either one answers 400
// INVALID_REQUEST (§1.5) instead of a database NOT NULL violation.
//
// Each case also runs as a non-admin. The 400 proves an admin passes the gate
// and reaches the handler's binding and validation; the 403 for the same body
// proves the gate still decides before the handler runs, so nobody else ever
// reaches that validation. No rejected request writes an Event.
func TestPracticeCreateRequiresActivityAndKind(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	f := newAdminGateFixture(t)
	router := f.routerFor(t, gin.TestMode)
	admin := f.token(t, f.admin)

	var before int64
	require.NoError(t, f.db.Model(&domain.Event{}).Count(&before).Error)

	startTime := time.Now().UTC().Format(time.RFC3339)
	cases := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "missing activity_id",
			body:        fmt.Sprintf(`{"title":"no activity","start_time":%q,"kind":"practice","mode":"report"}`, startTime),
			wantMessage: "activity_id is required",
		},
		{
			name:        "zero activity_id",
			body:        fmt.Sprintf(`{"title":"zero activity","start_time":%q,"activity_id":%q,"kind":"practice","mode":"report"}`, startTime, uuid.Nil.String()),
			wantMessage: "activity_id is required",
		},
		{
			name:        "missing kind",
			body:        fmt.Sprintf(`{"title":"no kind","start_time":%q,"activity_id":%q,"mode":"report"}`, startTime, f.activity.ID.String()),
			wantMessage: "kind is required",
		},
		{
			name:        "empty kind",
			body:        fmt.Sprintf(`{"title":"empty kind","start_time":%q,"activity_id":%q,"kind":"","mode":"report"}`, startTime, f.activity.ID.String()),
			wantMessage: "kind is required",
		},
		{
			// Binding rejects this one, so the handler's own check never runs.
			name: "malformed activity_id",
			body: fmt.Sprintf(`{"title":"bad activity","start_time":%q,"activity_id":"not-a-uuid","kind":"practice","mode":"report"}`, startTime),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.do(t, router, http.MethodPost, "/v1/practices", admin, tc.body)
			assertEnvelopeCode(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
			if tc.wantMessage != "" {
				assert.Equal(t, tc.wantMessage, decodeEnvelope(t, rec).Error.Message,
					"the envelope names the field that is missing")
			}

			rec = f.do(t, router, http.MethodPost, "/v1/practices", f.token(t, f.owner), tc.body)
			assertEnvelopeCode(t, rec, http.StatusForbidden, "FORBIDDEN")
		})
	}

	var after int64
	require.NoError(t, f.db.Model(&domain.Event{}).Count(&after).Error)
	assert.Equal(t, before, after, "a rejected raw create writes no Event")
}
