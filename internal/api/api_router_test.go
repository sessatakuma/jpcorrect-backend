package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
)

// TestRegisterGuildCaller drives real routes through Register in both gin modes
// and pins the outcome (accepted or rejected, status and envelope code). The
// error shape changed with #70; which requests are accepted must not.
func TestRegisterGuildCaller(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	callerID, guildID, newLeaderID := uuid.New(), uuid.New(), uuid.New()
	guildPath := "/v1/guilds/" + guildID.String()
	routes := []struct {
		name, method, path, body string
		wantStatus               int
	}{
		{"create", http.MethodPost, "/v1/guilds", `{"name":"test guild"}`, http.StatusCreated},
		{"transfer", http.MethodPost, guildPath + "/transfer-leader", `{"new_leader_user_id":"` + newLeaderID.String() + `"}`, http.StatusOK},
		{"get invite", http.MethodGet, guildPath + "/invite-link", "", http.StatusOK},
		{"create invite", http.MethodPost, guildPath + "/invite-link", `{}`, http.StatusOK},
	}
	cases := []struct {
		name, mode, callerHeader, authorization string
		wantStatus                              int    // Zero uses the route's success status.
		wantCode                                string // Empty when the request is accepted.
	}{
		{"debug caller", gin.DebugMode, callerID.String(), "", 0, ""},
		{"debug missing caller", gin.DebugMode, "", "", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"debug authorization alone", gin.DebugMode, "", "Bearer unused-token", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"debug invalid caller", gin.DebugMode, "not-a-uuid", "", http.StatusBadRequest, "INVALID_REQUEST"},
		{"debug nil caller", gin.DebugMode, uuid.Nil.String(), "", http.StatusBadRequest, "INVALID_REQUEST"},
		{"release ignores debug caller", gin.ReleaseMode, callerID.String(), "", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"release rejects bare token without JWKS", gin.ReleaseMode, callerID.String(), "Bearer unused-token", http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(tc.mode)
			for _, route := range routes {
				t.Run(route.name, func(t *testing.T) {
					api, gRepo, aRepo := newTestAPI()
					aRepo.getByGuildAndUserResult = &domain.GuildAttendee{Role: domain.GuildAttendeeRoleMaster}
					router := gin.New()
					Register(router, api)
					req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("X-Debug-User-ID", tc.callerHeader)
					req.Header.Set("Authorization", tc.authorization)
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					if tc.wantStatus != 0 {
						assert.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
						assert.Equal(t, tc.wantCode, decodeEnvelope(t, rec).Error.Code)
						assert.Empty(t, gRepo.createdMasters)
						assert.Empty(t, gRepo.transferLeaderCalls)
						assert.Empty(t, aRepo.getByGuildAndUserCalls)
						assert.Zero(t, gRepo.activeInviteCalls)
						assert.Empty(t, gRepo.createInviteLinkCalls)
						return
					}

					require.Equal(t, route.wantStatus, rec.Code, rec.Body.String())
					switch route.name {
					case "create":
						require.Len(t, gRepo.createdMasters, 1)
						assert.Equal(t, callerID, gRepo.createdMasters[0].UserID)
					case "transfer":
						require.Len(t, gRepo.transferLeaderCalls, 1)
						assert.Equal(t, callerID, gRepo.transferLeaderCalls[0].callerID)
						assert.Equal(t, guildID, gRepo.transferLeaderCalls[0].guildID)
						assert.Equal(t, newLeaderID, gRepo.transferLeaderCalls[0].newLeaderID)
					default:
						require.Len(t, aRepo.getByGuildAndUserCalls, 1)
						assert.Equal(t, callerID, aRepo.getByGuildAndUserCalls[0].userID)
						assert.Equal(t, guildID, aRepo.getByGuildAndUserCalls[0].guildID)
						if route.method == http.MethodGet {
							assert.Equal(t, 1, gRepo.activeInviteCalls)
						} else {
							assert.Len(t, gRepo.createInviteLinkCalls, 1)
						}
					}
				})
			}
		})
	}
}

// TestAPIKeyRoutesByMode pins the mode split of the api-tools group: release
// enforces APIKeyMiddleware, debug does not. Both failure and success use the
// same envelope, and no other route needs an API key in either mode.
func TestAPIKeyRoutesByMode(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	// A dedicated upstream that never blocks: several subtests proxy to it.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	tests := []struct {
		name       string
		mode       string
		apiKey     string
		header     string
		wantStatus int
		wantCode   string
	}{
		{"release without key", gin.ReleaseMode, "secret-key", "", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"release with wrong key", gin.ReleaseMode, "secret-key", "wrong", http.StatusUnauthorized, "UNAUTHORIZED"},
		{"release with key", gin.ReleaseMode, "secret-key", "secret-key", http.StatusOK, ""},
		{"debug without key", gin.DebugMode, "secret-key", "", http.StatusOK, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(tc.mode)
			api := &API{
				apiToolsURL:    upstream.URL,
				clientAPIKey:   tc.apiKey,
				proxyTransport: http.DefaultTransport.(*http.Transport).Clone(),
			}
			router := gin.New()
			Register(router, api)

			// A real socket: httputil.ReverseProxy needs a CloseNotifier-capable
			// writer, which httptest.ResponseRecorder is not.
			srv := httptest.NewServer(router)
			t.Cleanup(srv.Close)

			req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/dict-query", strings.NewReader(`{}`))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			if tc.header != "" {
				req.Header.Set("X-API-Key", tc.header)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer closeResponseBody(t, resp.Body)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, resp.StatusCode, string(body))

			if tc.wantCode != "" {
				assert.Equal(t, tc.wantCode, decodeEnvelopeBytes(t, body).Error.Code)
			}
		})
	}
}
