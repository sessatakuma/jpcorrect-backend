package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
)

// invokeInviteHandler runs an invite handler with the token as the :token route
// param, which is how the handlers read it.
func invokeInviteHandler(t *testing.T, api *API, handler gin.HandlerFunc, token string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/invites/"+token, nil)
	c.Params = gin.Params{{Key: "token", Value: token}}
	handler(c)
	return w
}

func TestInviteInfoHandlerLooksUpByToken(t *testing.T) {
	guild := &domain.Guild{Name: "invite guild", Description: "description"}
	invite := &domain.InviteLink{
		ID:        uuid.New(),
		GuildID:   uuid.New(),
		Token:     "token-abc",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	gRepo := &mockGuildRepo{inviteByToken: invite, guildByID: guild}
	api := &API{guildRepo: gRepo}

	w := invokeInviteHandler(t, api, api.InviteInfoHandler, invite.Token)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{invite.Token}, gRepo.inviteByTokenCalls, "the handler must look the link up by token")

	var body struct {
		Guild     map[string]string `json:"guild"`
		IsExpired bool              `json:"is_expired"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "invite guild", body.Guild["name"])
	assert.False(t, body.IsExpired)
}

func TestInviteInfoHandlerExpiredLink(t *testing.T) {
	invite := &domain.InviteLink{
		ID:        uuid.New(),
		GuildID:   uuid.New(),
		Token:     "token-expired",
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	gRepo := &mockGuildRepo{inviteByToken: invite, guildByID: &domain.Guild{Name: "invite guild"}}
	api := &API{guildRepo: gRepo}

	w := invokeInviteHandler(t, api, api.InviteInfoHandler, invite.Token)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		IsExpired bool `json:"is_expired"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.IsExpired, "expires_at alone decides whether a link is valid")
}

func TestInviteInfoHandlerUnknownToken(t *testing.T) {
	api := &API{guildRepo: &mockGuildRepo{}}

	w := invokeInviteHandler(t, api, api.InviteInfoHandler, "token-missing")

	assert.Equal(t, http.StatusNotFound, w.Code)
}
