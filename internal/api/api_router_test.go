package api

import (
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
		wantStatus                              int // Zero uses the route's success status.
	}{
		{"debug caller", gin.DebugMode, callerID.String(), "", 0},
		{"debug missing caller", gin.DebugMode, "", "", http.StatusUnauthorized},
		{"debug authorization alone", gin.DebugMode, "", "Bearer unused-token", http.StatusUnauthorized},
		{"debug invalid caller", gin.DebugMode, "not-a-uuid", "", http.StatusBadRequest},
		{"debug nil caller", gin.DebugMode, uuid.Nil.String(), "", http.StatusBadRequest},
		{"release ignores debug caller", gin.ReleaseMode, callerID.String(), "", http.StatusUnauthorized},
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
