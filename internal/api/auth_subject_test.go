package api

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthMiddlewareRejectsMalformedSubject pins the boundary check: a signed
// token whose sub is empty, blank, or the nil UUID is rejected before any
// lookup or write can run with that identity.
func TestAuthMiddlewareRejectsMalformedSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	const kid = "subject-key"
	jwksURL := newJWKSServer(t, &key.PublicKey, kid)

	api := NewAPI("", "", &http.Transport{}, nil, jwksURL, nil)
	require.NoError(t, api.InitializeJWKS(t.Context()))
	t.Cleanup(api.ShutdownJWKS)

	serve := func(token string) (status int, reached bool, body string) {
		router := gin.New()
		router.GET("/v1/users/me", api.AuthMiddleware(), func(c *gin.Context) {
			reached = true
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code, reached, w.Body.String()
	}

	rejected := []struct {
		name    string
		subject string
	}{
		{name: "empty subject", subject: ""},
		{name: "blank subject", subject: "   "},
		{name: "nil uuid subject", subject: uuid.Nil.String()},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			token := signSupabaseToken(t, key, kid, tc.subject, "member@example.com", true)
			status, reached, body := serve(token)
			assert.Equal(t, http.StatusUnauthorized, status, body)
			assert.False(t, reached, "a rejected token must never reach the handler")
		})
	}

	t.Run("still accepts a real subject", func(t *testing.T) {
		token := signSupabaseToken(t, key, kid, "supabase-"+uuid.NewString(), "member@example.com", true)
		status, reached, body := serve(token)
		assert.Equal(t, http.StatusOK, status, body)
		assert.True(t, reached)
	})
}
