package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebSocketRateLimitUsesEnvelope pins the /ws throttle response: it used to
// be a bare {"error": "too many connections"}.
func TestWebSocketRateLimitUsesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rl := NewRateLimiter(time.Minute, 1)
	t.Cleanup(rl.Close)

	api := &API{rateLimiter: rl}
	r := gin.New()
	r.GET("/ws", api.ServeWebSocket)

	serve := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	serve() // consumes the single allowed attempt; the upgrade itself fails
	rec := serve()

	require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	env := decodeEnvelope(t, rec)
	assert.Equal(t, "RATE_LIMITED", env.Error.Code)
}

// TestNoLegacyErrorShape is the grep-based guard against the retired
// {"error": "...", "details": "..."} response: no handler builds an error body
// by hand, and the AuthError format is gone.
func TestNoLegacyErrorShape(t *testing.T) {
	handBuiltErrorBody := regexp.MustCompile(`(?s)gin\.H\s*\{\s*"error"`)
	retired := []string{
		`"details":`,
		"respondAuthError",
		"domain.AuthError",
		"domain.NewAuthError",
	}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		src, err := os.ReadFile(name)
		require.NoError(t, err)
		checked++

		assert.NotRegexp(t, handBuiltErrorBody, string(src),
			"%s builds an error body by hand; every locally produced error goes through writeError", name)
		for _, needle := range retired {
			assert.NotContains(t, string(src), needle,
				"%s still references the retired error format (%s)", name, needle)
		}
	}

	require.Positive(t, checked, "the guard must actually scan the package sources")
}
