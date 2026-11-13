package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJWTTestAPI builds an API with only the dependencies AuthMiddleware needs
// and points it at a JWKS the returned signer controls.
func newJWTTestAPI(t *testing.T, repo *mockUserRepo) (*API, *testSigner) {
	t.Helper()

	signer := newTestSigner(t)
	api := &API{userRepo: repo}
	signer.attachTo(t, api)
	return api, signer
}

// TestCurrentUser pins the helper every authenticated handler starts with.
func TestCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := storedUser(t, uuid.New(), "me@example.com")

	tests := []struct {
		name     string
		setup    func(*gin.Context)
		wantUser *domain.User
	}{
		{
			name:     "no value in context",
			setup:    func(*gin.Context) {},
			wantUser: nil,
		},
		{
			name:     "the retired userID string key is not an identity",
			setup:    func(c *gin.Context) { c.Set("userID", user.ID.String()) },
			wantUser: nil,
		},
		{
			name:     "value of the wrong type",
			setup:    func(c *gin.Context) { c.Set(userContextKey, user.ID.String()) },
			wantUser: nil,
		},
		{
			name:     "resolved user",
			setup:    func(c *gin.Context) { c.Set(userContextKey, user) },
			wantUser: user,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			tt.setup(c)

			got := currentUser(c)
			if tt.wantUser == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tt.wantUser.ID, got.ID)
		})
	}
}

// TestAuthMiddlewareResolvesUser is the §1.2 contract: a verified JWT is
// exchanged for the internal User row, an uninitialized caller is told to call
// users/init, and nothing but the JWT establishes identity.
func TestAuthMiddlewareResolvesUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const email = "caller@example.com"
	sub := uuid.MustParse("6b1c1f0e-8b81-4a4d-9f0c-3f1d1c2b3a40")

	t.Run("valid token with a stored row sets currentUser", func(t *testing.T) {
		stored := storedUser(t, sub, email)
		// A custom role claim must not override the stored role.
		stored.Role = domain.UserRoleStaff
		repo := &mockUserRepo{getBySupabaseUserIDResult: stored}
		api, signer := newJWTTestAPI(t, repo)

		var reached bool
		var seen *domain.User
		var legacyAny any
		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) {
			reached = true
			seen = currentUser(c)
			legacyAny, _ = c.Get("userID")
			c.Status(http.StatusOK)
		})

		token := signer.token(t, supabaseClaims{
			Email: email,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: sub.String(),
				// A role claim is not part of the contract; it must be ignored.
				Audience: jwt.ClaimStrings{"role:admin"},
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.True(t, reached)
		require.NotNil(t, seen)
		assert.Equal(t, stored.ID, seen.ID)
		assert.Equal(t, domain.UserRoleStaff, seen.Role, "authorization uses the stored row, not the token")
		assert.Equal(t, []uuid.UUID{sub}, repo.getBySupabaseUserIDCalls)
		assert.Nil(t, legacyAny, `the retired "userID" key is never set`)
	})

	t.Run("valid token without a stored row → 401 USER_NOT_INITIALIZED", func(t *testing.T) {
		repo := &mockUserRepo{getBySupabaseUserIDErr: domain.ErrNotFound}
		api, signer := newJWTTestAPI(t, repo)

		var reached bool
		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) {
			reached = true
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+signer.userToken(t, sub, email))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.False(t, reached, "the handler must not run for an uninitialized caller")

		env := decodeEnvelope(t, rec)
		assert.Equal(t, "USER_NOT_INITIALIZED", env.Error.Code)
		assert.Equal(t, []uuid.UUID{sub}, repo.getBySupabaseUserIDCalls)
	})

	t.Run("expired token → 401", func(t *testing.T) {
		api, signer := newJWTTestAPI(t, &mockUserRepo{})

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		token := signer.token(t, supabaseClaims{
			Email: email,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   sub.String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		env := decodeEnvelope(t, rec)
		assert.Equal(t, "UNAUTHORIZED", env.Error.Code)
		assert.Equal(t, "invalid token", env.Error.Message)
	})

	t.Run("token signed by a different key → 401", func(t *testing.T) {
		api, _ := newJWTTestAPI(t, &mockUserRepo{})
		other := newTestSigner(t)

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+other.userToken(t, sub, email))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("token without a subject → 401", func(t *testing.T) {
		api, signer := newJWTTestAPI(t, &mockUserRepo{})

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+signer.userToken(t, uuid.Nil, email))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("subject that is not a UUID → 401 without a lookup", func(t *testing.T) {
		repo := &mockUserRepo{}
		api, signer := newJWTTestAPI(t, repo)

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		token := signer.token(t, supabaseClaims{
			Email:            email,
			RegisteredClaims: jwt.RegisteredClaims{Subject: "not-a-uuid"},
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.getBySupabaseUserIDCalls, "a malformed subject must not reach the repository")
	})

	t.Run("nil UUID subject → 401 without a lookup", func(t *testing.T) {
		repo := &mockUserRepo{}
		api, signer := newJWTTestAPI(t, repo)

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		token := signer.token(t, supabaseClaims{
			Email:            email,
			RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.Nil.String()},
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.getBySupabaseUserIDCalls, "the nil UUID must not reach the repository")
	})

	t.Run("identity header without a token → 401", func(t *testing.T) {
		api, _ := newJWTTestAPI(t, &mockUserRepo{})

		r := gin.New()
		r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("X-Debug-User-ID", uuid.NewString())
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
	})
}

// TestAuthMiddlewareUsersInitExemption pins §1.2 item 4: users/init skips the
// user lookup and nothing else does.
func TestAuthMiddlewareUsersInitExemption(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sub := uuid.MustParse("8f2d5c14-3a6b-4f7e-9b21-5c8e4a1d7f60")

	newServer := func(t *testing.T, repo *mockUserRepo, wantSubject string) (*gin.Engine, *testSigner) {
		t.Helper()
		api, signer := newJWTTestAPI(t, repo)
		r := gin.New()
		r.POST("/v1/users/init", api.AuthMiddleware(), func(c *gin.Context) {
			auth := currentSupabaseAuth(c)
			require.NotNil(t, auth)
			assert.Equal(t, wantSubject, auth.Subject, "users/init receives the verified subject claim unparsed")
			assert.Equal(t, "first@example.com", auth.Email)
			assert.Nil(t, currentUser(c), "the exemption resolves no user")
			c.Status(http.StatusOK)
		})
		return r, signer
	}

	t.Run("lookup is skipped", func(t *testing.T) {
		repo := &mockUserRepo{}
		r, signer := newServer(t, repo, sub.String())

		req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
		req.Header.Set("Authorization", "Bearer "+signer.userToken(t, sub, "first@example.com"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Empty(t, repo.getBySupabaseUserIDCalls, "users/init must not query the user table")
	})

	// The middleware must not reject the claim for this route: the handler owns
	// the 400 INVALID_REQUEST answer (spec §2.1), and either way no repo call
	// happens for an unusable subject.
	t.Run("an unusable subject reaches the handler", func(t *testing.T) {
		for _, tc := range []struct{ name, subject string }{
			{"missing subject", ""},
			{"subject that is not a UUID", "not-a-uuid"},
			{"nil UUID subject", uuid.Nil.String()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repo := &mockUserRepo{}
				r, signer := newServer(t, repo, tc.subject)

				token := signer.token(t, supabaseClaims{
					Email:            "first@example.com",
					RegisteredClaims: jwt.RegisteredClaims{Subject: tc.subject},
				})
				req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				assert.Empty(t, repo.getBySupabaseUserIDCalls, "the exemption never queries the user table")
				assert.Empty(t, repo.createIfAbsentCalls, "the middleware never writes a row")
			})
		}
	})

	t.Run("the exemption still requires a valid token", func(t *testing.T) {
		repo := &mockUserRepo{}
		r, signer := newServer(t, repo, sub.String())

		tests := []struct {
			name       string
			authorizat string
		}{
			{name: "missing token"},
			{name: "garbage token", authorizat: "Bearer not-a-token"},
			{name: "expired token", authorizat: "Bearer " + signer.token(t, supabaseClaims{
				Email: "first@example.com",
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   sub.String(),
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
				},
			})},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
				if tt.authorizat != "" {
					req.Header.Set("Authorization", tt.authorizat)
				}
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
				assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
			})
		}
	})
}
