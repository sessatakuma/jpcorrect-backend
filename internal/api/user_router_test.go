package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userRouterFixture is one fully wired Register() for a single test case.
type userRouterFixture struct {
	router  *gin.Engine
	users   *mockUserRepo
	signer  *testSigner
	token   string
	subject string
}

// newUserRouterFixture builds the real router with mock repositories and a test
// JWKS. caller == nil models a valid token for a user who never called
// users/init: the lookup answers ErrNotFound.
func newUserRouterFixture(t *testing.T, caller *domain.User) userRouterFixture {
	t.Helper()

	signer := newTestSigner(t)
	api, _, _ := newTestAPI()
	users := &mockUserRepo{}
	if caller != nil {
		users.getBySupabaseUserIDResult = caller
	} else {
		users.getBySupabaseUserIDErr = domain.ErrNotFound
	}
	api.userRepo = users
	signer.attachTo(t, api)

	router := gin.New()
	Register(router, api)

	subject := uuid.NewString()
	email := "caller@example.com"
	if caller != nil && caller.SupabaseUserID != nil {
		subject = *caller.SupabaseUserID
		email = caller.Email
	}

	return userRouterFixture{
		router:  router,
		users:   users,
		signer:  signer,
		token:   signer.userToken(t, subject, email),
		subject: subject,
	}
}

// do sends one request and returns the recorder.
func (f userRouterFixture) do(method, path, body, authorization, identityHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if identityHeader != "" {
		req.Header.Set("X-Debug-User-ID", identityHeader)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// TestRegisterUserRoutesAuthentication is the router-level acceptance of #73:
// every /v1 user route demands a real JWT in gin.DebugMode and in
// gin.ReleaseMode, a JWT is the only thing that establishes identity, and the
// removed email lookup is unregistered.
func TestRegisterUserRoutesAuthentication(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)
			caller := storedUser(t, uuid.NewString(), "caller@example.com")

			// Every authenticated route answers 401 without credentials, and a
			// bare identity header is never a credential.
			noCredentialRoutes := []struct{ method, path, body string }{
				{http.MethodGet, "/v1/users/me", ""},
				{http.MethodPut, "/v1/users/me", `{"name":"renamed"}`},
				{http.MethodGet, "/v1/users/" + caller.ID.String(), ""},
				{http.MethodGet, "/v1/users/name/stored%20name", ""},
				{http.MethodPost, "/v1/users/init", `{}`},
			}
			for _, route := range noCredentialRoutes {
				t.Run("no credentials "+route.method+" "+route.path, func(t *testing.T) {
					f := newUserRouterFixture(t, caller)

					rec := f.do(route.method, route.path, route.body, "", "")
					require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
					assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)

					rec = f.do(route.method, route.path, route.body, "", caller.ID.String())
					require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
					assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code,
						"an identity header must never replace a JWT")
					assert.Empty(t, f.users.getBySupabaseUserIDCalls)
				})
			}

			// A valid JWT reaches the handler.
			t.Run("valid token reaches the handler "+http.MethodGet+" /v1/users/me", func(t *testing.T) {
				f := newUserRouterFixture(t, caller)

				rec := f.do(http.MethodGet, "/v1/users/me", "", "Bearer "+f.token, "")
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

				var body domain.User
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, caller.ID, body.ID)
				assert.Equal(t, []string{f.subject}, f.users.getBySupabaseUserIDCalls)
			})

			t.Run("valid token without a row → 401 USER_NOT_INITIALIZED", func(t *testing.T) {
				f := newUserRouterFixture(t, nil)

				// Every authenticated route, not only the account endpoints.
				for _, path := range []string{
					"/v1/users/me",
					"/v1/users/" + uuid.NewString(),
					"/v1/guilds/" + uuid.NewString(),
					"/v1/mistakes/" + uuid.NewString(),
				} {
					rec := f.do(http.MethodGet, path, "", "Bearer "+f.token, "")
					require.Equal(t, http.StatusUnauthorized, rec.Code, "%s: %s", path, rec.Body.String())
					assert.Equal(t, "USER_NOT_INITIALIZED", decodeEnvelope(t, rec).Error.Code, path)
				}
			})

			t.Run("invalid token → 401", func(t *testing.T) {
				f := newUserRouterFixture(t, caller)

				rec := f.do(http.MethodGet, "/v1/users/me", "", "Bearer not-a-token", "")
				require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
				assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
			})

			t.Run("init with a valid token reaches the handler", func(t *testing.T) {
				created := storedUser(t, uuid.NewString(), "first@example.com")
				f := newUserRouterFixture(t, created)

				rec := f.do(http.MethodPost, "/v1/users/init", `{}`, "Bearer "+f.token, "")
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

				require.Len(t, f.users.createIfAbsentCalls, 1)
				assert.Equal(t, created.Email, f.users.createIfAbsentCalls[0].Email)
			})

			t.Run("removed email lookup is unregistered", func(t *testing.T) {
				f := newUserRouterFixture(t, caller)

				rec := f.do(http.MethodGet, "/v1/users/email/caller@example.com", "", "Bearer "+f.token, "")
				require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			})
		})
	}
}
