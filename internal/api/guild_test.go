package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// mockGuildRepo implements domain.GuildRepository by embedding the interface
// and overriding only the methods exercised by the handlers under test.
// Any non-overridden method that is accidentally called will panic on the nil
// embedded interface — acceptable for these focused handler tests.
type mockGuildRepo struct {
	domain.GuildRepository

	createdMasters []domain.GuildAttendee

	transferLeaderCalls []transferLeaderCall
	transferLeaderErr   error

	activeInviteCalls int
	activeInvite      *domain.InviteLink
	activeInviteErr   error

	createInviteLinkCalls []createInviteLinkCall
	createInviteLinkErr   error
}

type transferLeaderCall struct {
	guildID, callerID, newLeaderID uuid.UUID
}

type createInviteLinkCall struct {
	guildID uuid.UUID
	invite  *domain.InviteLink
}

func (m *mockGuildRepo) CreateWithMaster(_ context.Context, _ *domain.Guild, attendee *domain.GuildAttendee) error {
	m.createdMasters = append(m.createdMasters, *attendee)
	return nil
}

func (m *mockGuildRepo) TransferLeader(_ context.Context, guildID, callerID, newLeaderID uuid.UUID) error {
	m.transferLeaderCalls = append(m.transferLeaderCalls, transferLeaderCall{guildID, callerID, newLeaderID})
	return m.transferLeaderErr
}

func (m *mockGuildRepo) GetActiveInviteByGuildID(_ context.Context, _ uuid.UUID, _ time.Time) (*domain.InviteLink, error) {
	m.activeInviteCalls++
	return m.activeInvite, m.activeInviteErr
}

func (m *mockGuildRepo) CreateInviteLinkWithTx(_ context.Context, guildID uuid.UUID, newInvite *domain.InviteLink, _ time.Time) error {
	m.createInviteLinkCalls = append(m.createInviteLinkCalls, createInviteLinkCall{guildID: guildID, invite: newInvite})
	return m.createInviteLinkErr
}

// mockAttendeeRepo implements domain.GuildAttendeeRepository by embedding the
// interface and overriding only GetByGuildAndUser, the single method the
// handlers under test call on it.
type mockAttendeeRepo struct {
	domain.GuildAttendeeRepository

	getByGuildAndUserResult *domain.GuildAttendee
	getByGuildAndUserErr    error
	getByGuildAndUserCalls  []getByGuildAndUserCall
}

type getByGuildAndUserCall struct {
	guildID, userID uuid.UUID
}

func (m *mockAttendeeRepo) GetByGuildAndUser(_ context.Context, guildID, userID uuid.UUID) (*domain.GuildAttendee, error) {
	m.getByGuildAndUserCalls = append(m.getByGuildAndUserCalls, getByGuildAndUserCall{guildID, userID})
	return m.getByGuildAndUserResult, m.getByGuildAndUserErr
}

func newTestAPI() (*API, *mockGuildRepo, *mockAttendeeRepo) {
	gRepo := &mockGuildRepo{}
	aRepo := &mockAttendeeRepo{}
	api := &API{guildRepo: gRepo, guildAttendeeRepo: aRepo}
	return api, gRepo, aRepo
}

// invokeHandler mounts handler on a fresh gin engine. The UUID segment of path
// is converted to the :id route param (the handlers read c.Param("id")). When
// userID is non-nil it is injected into the gin context as a string, exactly
// like AuthMiddleware does (c.Set("userID", claims.Subject)); when nil, no
// value is set so c.Get("userID") returns (nil, false) → 401.
//
// The HTTP method is inferred: POST when a body is provided (even an empty
// one), GET otherwise — matching the handlers under test.
func invokeHandler(t *testing.T, handler gin.HandlerFunc, path string, userID *uuid.UUID, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	parts := strings.Split(path, "/")
	for i, p := range parts {
		if _, err := uuid.Parse(p); err == nil {
			parts[i] = ":id"
		}
	}
	pattern := strings.Join(parts, "/")

	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}

	r := gin.New()
	r.Handle(method, pattern, func(c *gin.Context) {
		if userID != nil {
			c.Set("userID", userID.String())
		}
		handler(c)
	})

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

type transferLeaderBody struct {
	NewLeaderUserID uuid.UUID `json:"new_leader_user_id"`
}

type inviteLinkBody struct {
	TTLSeconds int64 `json:"ttl_seconds"`
}

func TestGuildTransferLeaderHandler(t *testing.T) {
	callerID := uuid.New()
	newLeaderID := uuid.New()
	transferPath := func(guildID uuid.UUID) string {
		return "/v1/guilds/" + guildID.String() + "/transfer-leader"
	}

	t.Run("no userID in context → 401", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(uuid.New()), nil, body)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, gRepo.transferLeaderCalls)
	})

	t.Run("caller is not a member → 403", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		guildID := uuid.New()
		// The caller-membership/master check runs inside TransferLeader (repo
		// layer): a caller without a membership row is rejected with
		// ErrNotGuildMaster, which the handler maps to 403.
		gRepo.transferLeaderErr = domain.ErrNotGuildMaster
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(guildID), &callerID, body)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "only the guild master can transfer leadership")
		assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
		// the handler delegated the authz check to the repo — the call was made
		if assert.Len(t, gRepo.transferLeaderCalls, 1) {
			assert.Equal(t, callerID, gRepo.transferLeaderCalls[0].callerID)
		}
	})

	t.Run("caller is member but not master → 403", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		guildID := uuid.New()
		// Same handler branch: TransferLeader rejects a member (non-master)
		// caller with ErrNotGuildMaster → 403.
		gRepo.transferLeaderErr = domain.ErrNotGuildMaster
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(guildID), &callerID, body)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "only the guild master can transfer leadership")
		assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
		if assert.Len(t, gRepo.transferLeaderCalls, 1) {
			assert.Equal(t, callerID, gRepo.transferLeaderCalls[0].callerID)
		}
	})

	t.Run("caller is master, new leader not an active member → 404", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		guildID := uuid.New()
		gRepo.transferLeaderErr = domain.ErrNotGuildMember
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(guildID), &callerID, body)

		// Spec §3.3: "Target not an active member → 404 ErrNotFound".
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "new leader must be an active member of the guild")
		assert.Equal(t, "NOT_FOUND", decodeEnvelope(t, rec).Error.Code)
		if assert.Len(t, gRepo.transferLeaderCalls, 1) {
			assert.Equal(t, newLeaderID, gRepo.transferLeaderCalls[0].newLeaderID)
		}
	})

	t.Run("caller is master, guild not found → 404", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		gRepo.transferLeaderErr = domain.ErrNotFound
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(uuid.New()), &callerID, body)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "Guild not found")
		assert.Equal(t, "NOT_FOUND", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("new leader already leads another guild → 409", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		guildID := uuid.New()
		gRepo.transferLeaderErr = domain.ErrGuildLimitReached
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(guildID), &callerID, body)

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Contains(t, rec.Body.String(), "new leader already leads another guild")
		assert.Equal(t, "GUILD_LIMIT_REACHED", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("caller is master, success → 200", func(t *testing.T) {
		api, gRepo, _ := newTestAPI()
		guildID := uuid.New()
		body, err := json.Marshal(transferLeaderBody{NewLeaderUserID: newLeaderID})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildTransferLeaderHandler, transferPath(guildID), &callerID, body)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "leader transferred successfully")
		if assert.Len(t, gRepo.transferLeaderCalls, 1) {
			assert.Equal(t, guildID, gRepo.transferLeaderCalls[0].guildID)
			assert.Equal(t, callerID, gRepo.transferLeaderCalls[0].callerID)
			assert.Equal(t, newLeaderID, gRepo.transferLeaderCalls[0].newLeaderID)
		}
	})
}

func TestGuildInviteLinkGetHandler(t *testing.T) {
	callerID := uuid.New()
	getPath := func(guildID uuid.UUID) string {
		return "/v1/guilds/" + guildID.String() + "/invite-link"
	}

	t.Run("no userID → 401", func(t *testing.T) {
		api, _, aRepo := newTestAPI()

		rec := invokeHandler(t, api.GuildInviteLinkGetHandler, getPath(uuid.New()), nil, nil)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, aRepo.getByGuildAndUserCalls)
	})

	t.Run("non-member → 403", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		guildID := uuid.New()
		aRepo.getByGuildAndUserErr = domain.ErrNotFound

		rec := invokeHandler(t, api.GuildInviteLinkGetHandler, getPath(guildID), &callerID, nil)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "user is not a member of this guild")
		assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
		if assert.Len(t, aRepo.getByGuildAndUserCalls, 1) {
			assert.Equal(t, guildID, aRepo.getByGuildAndUserCalls[0].guildID)
			assert.Equal(t, callerID, aRepo.getByGuildAndUserCalls[0].userID)
		}
		// membership check failed → the invite must never be queried
		assert.Zero(t, gRepo.activeInviteCalls)
	})

	for _, role := range []domain.GuildAttendeeRole{domain.GuildAttendeeRoleMember, domain.GuildAttendeeRoleMaster} {
		t.Run("left member with role "+string(role)+" → 403", func(t *testing.T) {
			api, gRepo, aRepo := newTestAPI()
			leftAt := time.Now()
			aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: role, LeftAt: &leftAt}

			rec := invokeHandler(t, api.GuildInviteLinkGetHandler, getPath(uuid.New()), &callerID, nil)

			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Zero(t, gRepo.activeInviteCalls)
		})
	}

	t.Run("member success → 200 with body", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		guildID := uuid.New()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
		expiresAt := time.Now().Add(time.Hour)
		gRepo.activeInvite = &domain.InviteLink{
			Token:     "abc123",
			ExpiresAt: expiresAt,
			CreatedAt: time.Now(),
		}

		rec := invokeHandler(t, api.GuildInviteLinkGetHandler, getPath(guildID), &callerID, nil)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"token":"abc123"`)
		assert.NotContains(t, rec.Body.String(), `"code"`, "the response key is `token`, not the legacy `code`")
		if assert.Len(t, aRepo.getByGuildAndUserCalls, 1) {
			assert.Equal(t, guildID, aRepo.getByGuildAndUserCalls[0].guildID)
			assert.Equal(t, callerID, aRepo.getByGuildAndUserCalls[0].userID)
		}
		assert.Equal(t, 1, gRepo.activeInviteCalls)
	})

	t.Run("member but no active invite → 200 null", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}

		rec := invokeHandler(t, api.GuildInviteLinkGetHandler, getPath(uuid.New()), &callerID, nil)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "null", strings.TrimSpace(rec.Body.String()))
		assert.Equal(t, 1, gRepo.activeInviteCalls)
	})
}

func TestGuildInviteLinkCreateHandler(t *testing.T) {
	callerID := uuid.New()
	createPath := func(guildID uuid.UUID) string {
		return "/v1/guilds/" + guildID.String() + "/invite-link"
	}
	codePattern := `^[0-9a-f]{32}$`

	t.Run("non-member → 403", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserErr = domain.ErrNotFound

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, []byte(`{"ttl_seconds":3600}`))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "user is not a member of this guild")
		assert.Empty(t, gRepo.createInviteLinkCalls)
	})

	t.Run("member not master → 403", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMember}

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, []byte(`{"ttl_seconds":3600}`))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "only the guild master can manage invite links")
		assert.Empty(t, gRepo.createInviteLinkCalls)
	})

	t.Run("left master → 403", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		leftAt := time.Now()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster, LeftAt: &leftAt}

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, []byte(`{"ttl_seconds":3600}`))

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Empty(t, gRepo.createInviteLinkCalls)
	})

	t.Run("master, TTL <= 0 → 400", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
		body, err := json.Marshal(inviteLinkBody{TTLSeconds: -5})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, body)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ttl_seconds must be between 1 and 31536000")
		assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, gRepo.createInviteLinkCalls)
	})

	t.Run("master, TTL too large → 400", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
		body, err := json.Marshal(inviteLinkBody{TTLSeconds: 999999999})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, body)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ttl_seconds must be between 1 and 31536000")
		assert.Empty(t, gRepo.createInviteLinkCalls)
	})

	t.Run("master, valid TTL → 200", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		guildID := uuid.New()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
		body, err := json.Marshal(inviteLinkBody{TTLSeconds: 3600})
		assert.NoError(t, err)

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(guildID), &callerID, body)

		assert.Equal(t, http.StatusOK, rec.Code)
		before := time.Now()
		var resp struct {
			Token           string    `json:"token"`
			ExpiresAt       time.Time `json:"expires_at"`
			CreatedByUserID uuid.UUID `json:"created_by_user_id"`
		}
		assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Regexp(t, codePattern, resp.Token)
		assert.Equal(t, callerID, resp.CreatedByUserID)
		assert.WithinDuration(t, before.Add(time.Hour), resp.ExpiresAt, 5*time.Second)
		assert.NotContains(t, rec.Body.String(), `"code"`, "the response key is `token`, not the legacy `code`")
		if assert.Len(t, gRepo.createInviteLinkCalls, 1) {
			assert.Equal(t, guildID, gRepo.createInviteLinkCalls[0].guildID)
			assert.Len(t, gRepo.createInviteLinkCalls[0].invite.Token, 32)
			assert.Equal(t, callerID, gRepo.createInviteLinkCalls[0].invite.CreatedByUserID)
		}
	})

	t.Run("master, empty body → 200 with the default 7-day expiry", func(t *testing.T) {
		api, gRepo, aRepo := newTestAPI()
		aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}

		rec := invokeHandler(t, api.GuildInviteLinkCreateHandler, createPath(uuid.New()), &callerID, []byte{})

		assert.Equal(t, http.StatusOK, rec.Code)
		before := time.Now()
		var resp struct {
			Token     string    `json:"token"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Regexp(t, codePattern, resp.Token)
		// expires_at is NOT NULL: a link always carries an expiry, 7 days by default.
		assert.WithinDuration(t, before.Add(7*24*time.Hour), resp.ExpiresAt, 5*time.Second)
		if assert.Len(t, gRepo.createInviteLinkCalls, 1) {
			assert.False(t, gRepo.createInviteLinkCalls[0].invite.ExpiresAt.IsZero())
		}
	})
}
