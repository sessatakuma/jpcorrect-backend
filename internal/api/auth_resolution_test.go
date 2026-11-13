package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// identityRouter mounts AuthMiddleware in front of the routes that depend on
// the caller identity, so every test here goes through real JWT verification
// and the real user resolution step.
type identityRouter struct {
	engine      *gin.Engine
	initReached bool
	probeUserID any
	probeSub    any
	probeUser   *domain.User
}

func newIdentityRouter(t *testing.T) (*identityRouter, *fakeUserRepo, *mockGuildRepo, *testSigner) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	api, users, guilds := newAccountAPI()
	signer := newTestSigner(t)
	signer.attachTo(t, api)

	router := &identityRouter{engine: gin.New()}
	router.engine.Use(api.AuthMiddleware())
	router.engine.GET("/v1/users/me", api.UserMeHandler)
	router.engine.POST("/v1/users/init", func(c *gin.Context) {
		router.initReached = true
		c.Status(http.StatusOK)
	})
	router.engine.POST("/v1/guilds", api.GuildCreateHandler)
	router.engine.GET("/v1/probe", func(c *gin.Context) {
		router.probeUserID, _ = c.Get(ContextKeyUserID)
		router.probeSub, _ = c.Get(ContextKeySupabaseID)
		router.probeUser = currentUser(c)
		c.Status(http.StatusOK)
	})

	return router, users, guilds, signer
}

func (r *identityRouter) do(method, path, token string, body []byte) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	r.engine.ServeHTTP(rec, req)
	return rec
}

// storedAccount returns the row an initialized caller owns.
func storedAccount(t *testing.T, sub uuid.UUID) *domain.User {
	t.Helper()
	return &domain.User{
		ID:             uuid.New(),
		Email:          "caller@example.com",
		Name:           "caller",
		SupabaseUserID: &sub,
		Role:           domain.UserRoleUser,
		Status:         domain.UserStatusActive,
	}
}

func TestAuthMiddlewareResolvesInternalUser(t *testing.T) {
	sub := uuid.New()
	account := storedAccount(t, sub)

	router, users, _, signer := newIdentityRouter(t)
	users.seed(*account)

	rec := router.do(http.MethodGet, "/v1/probe", signer.userToken(t, sub.String(), account.Email), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, sub, router.probeSub, "the verified subject stays available")
	require.NotNil(t, router.probeUser)
	assert.Equal(t, account.ID, router.probeUser.ID, "the resolved row is the stored account")
	assert.Equal(t, account.ID.String(), router.probeUserID,
		"handlers that read userID must receive the internal user ID, not the subject")
	assert.NotEqual(t, sub.String(), router.probeUserID,
		"the Supabase subject must never be published as the internal user ID")
}

func TestAuthMiddlewareMeReturnsResolvedAccount(t *testing.T) {
	sub := uuid.New()
	account := storedAccount(t, sub)

	router, users, _, signer := newIdentityRouter(t)
	users.seed(*account)

	rec := router.do(http.MethodGet, "/v1/users/me", signer.userToken(t, sub.String(), account.Email), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var body domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, account.ID, body.ID)
	assert.Equal(t, account.Email, body.Email)
}

func TestAuthMiddlewareRejectsUninitializedCaller(t *testing.T) {
	sub := uuid.New()

	router, users, _, signer := newIdentityRouter(t)

	rec := router.do(http.MethodGet, "/v1/users/me", signer.userToken(t, sub.String(), "new@example.com"), nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "user not initialized", body["error"])
	assert.Equal(t, int(1), users.lookupCalls, "the middleware performs the lookup")
	assert.Equal(t, 0, users.createCalls, "a rejected request creates nothing")
}

func TestAuthMiddlewareExemptsUsersInitFromTheLookup(t *testing.T) {
	sub := uuid.New()

	router, users, _, signer := newIdentityRouter(t)

	rec := router.do(http.MethodPost, "/v1/users/init", signer.userToken(t, sub.String(), "new@example.com"), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, router.initReached, "the init handler runs without an account")
	assert.Equal(t, 0, users.lookupCalls, "init is exempt from the user lookup")
}

func TestAuthMiddlewarePropagatesInternalIDToGuildCreate(t *testing.T) {
	sub := uuid.New()
	account := storedAccount(t, sub)

	router, users, guilds, signer := newIdentityRouter(t)
	users.seed(*account)

	body, err := json.Marshal(map[string]string{"name": "identity guild"})
	require.NoError(t, err)

	rec := router.do(http.MethodPost, "/v1/guilds", signer.userToken(t, sub.String(), account.Email), body)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, guilds.createdMasters, 1)

	assert.Equal(t, account.ID, guilds.createdMasters[0].UserID,
		"guild ownership is keyed by the internal user ID")
	assert.NotEqual(t, sub, guilds.createdMasters[0].UserID,
		"guild ownership must not be keyed by the Supabase subject")
}

func TestAuthMiddlewareRejectsUninitializedGuildCaller(t *testing.T) {
	sub := uuid.New()

	router, _, guilds, signer := newIdentityRouter(t)

	body, err := json.Marshal(map[string]string{"name": "identity guild"})
	require.NoError(t, err)

	rec := router.do(http.MethodPost, "/v1/guilds", signer.userToken(t, sub.String(), "new@example.com"), body)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, guilds.createdMasters, "no guild is written without an internal account")
}

func TestAuthMiddlewareRejectsInvalidSubject(t *testing.T) {
	for _, subject := range []string{"not-a-uuid", uuid.Nil.String()} {
		t.Run(subject, func(t *testing.T) {
			for _, route := range []struct{ method, path string }{
				{http.MethodGet, "/v1/users/me"},
				{http.MethodPost, "/v1/users/init"},
			} {
				router, users, _, signer := newIdentityRouter(t)
				rec := router.do(route.method, route.path, signer.userToken(t, subject, "x@example.com"), nil)
				require.Equal(t, http.StatusUnauthorized, rec.Code)
				assert.Equal(t, 0, users.lookupCalls)
				assert.Equal(t, 0, users.createCalls)
				assert.False(t, router.initReached)
			}
		})
	}
}

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	router, users, _, _ := newIdentityRouter(t)

	rec := router.do(http.MethodGet, "/v1/users/me", "", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 0, users.lookupCalls)
}

func TestAuthMiddlewareReportsLookupFailure(t *testing.T) {
	sub := uuid.New()

	router, users, _, signer := newIdentityRouter(t)
	users.lookupErr = assert.AnError

	rec := router.do(http.MethodGet, "/v1/users/me", signer.userToken(t, sub.String(), "caller@example.com"), nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "failed to resolve user", body["error"])
}
