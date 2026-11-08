package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testInternalAPIKey = "test-internal-api-key"
	testClientAPIKey   = "test-client-api-key"
)

// newInternalProbeAPI builds the API and a router whose /internal group carries
// one test-only probe route. The probe is mounted through internalRoutes, the
// same entry point Register uses, so the test exercises the real group and
// middleware wiring rather than a hand-rolled imitation.
func newInternalProbeAPI(t *testing.T, internalKey string) (*gin.Engine, *bool) {
	t.Helper()

	api := NewAPI("", testClientAPIKey, internalKey, &http.Transport{}, nil, "", nil)
	t.Cleanup(api.Close)

	reached := false
	router := gin.New()
	api.internalRoutes(router).GET("/probe", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})
	return router, &reached
}

// TestInternalRoutesRequireAPIKeyInEveryMode pins the /internal key contract in
// both gin modes. There is no debug bypass, CLIENT_API_KEY is not accepted, and
// an empty INTERNAL_API_KEY fails closed.
func TestInternalRoutesRequireAPIKeyInEveryMode(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	cases := []struct {
		name            string
		configuredKey   string
		headerKey       string
		debugCaller     string
		wantStatus      int
		wantMessage     string
		wantHandlerRuns bool
	}{
		{
			name:          "missing key",
			configuredKey: testInternalAPIKey,
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "missing X-API-Key header",
		},
		{
			name:          "wrong key",
			configuredKey: testInternalAPIKey,
			headerKey:     "wrong-key",
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "invalid API key",
		},
		{
			name:          "client API key is not an internal key",
			configuredKey: testInternalAPIKey,
			headerKey:     testClientAPIKey,
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "invalid API key",
		},
		{
			name:            "correct internal key reaches the handler",
			configuredKey:   testInternalAPIKey,
			headerKey:       testInternalAPIKey,
			wantStatus:      http.StatusOK,
			wantHandlerRuns: true,
		},
		{
			name:          "debug caller header does not bypass the key",
			configuredKey: testInternalAPIKey,
			debugCaller:   uuid.NewString(),
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "missing X-API-Key header",
		},
		{
			name:          "empty configured key rejects a plausible key",
			configuredKey: "",
			headerKey:     testInternalAPIKey,
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "internal API key authentication is not configured",
		},
		{
			name:          "empty configured key rejects a missing key",
			configuredKey: "",
			wantStatus:    http.StatusUnauthorized,
			wantMessage:   "internal API key authentication is not configured",
		},
	}

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					router, reached := newInternalProbeAPI(t, tc.configuredKey)

					req := httptest.NewRequest(http.MethodGet, "/internal/probe", nil)
					if tc.headerKey != "" {
						req.Header.Set("X-API-Key", tc.headerKey)
					}
					if tc.debugCaller != "" {
						req.Header.Set("X-Debug-User-ID", tc.debugCaller)
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
					require.Equal(t, tc.wantHandlerRuns, *reached)
					if tc.wantHandlerRuns {
						return
					}
					env := decodeEnvelope(t, rec)
					assert.Equal(t, "UNAUTHORIZED", env.Error.Code)
					assert.Equal(t, tc.wantMessage, env.Error.Message)
				})
			}
		})
	}
}

// TestInternalAPIKeyIsNeverLogged drives an accepted and a rejected request
// while capturing the standard logger; neither the configured key nor the
// supplied key may appear in the output.
func TestInternalAPIKeyIsNeverLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	router, _ := newInternalProbeAPI(t, testInternalAPIKey)

	for _, key := range []string{"", "wrong-key", testInternalAPIKey} {
		req := httptest.NewRequest(http.MethodGet, "/internal/probe", nil)
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	assert.NotContains(t, logs.String(), testInternalAPIKey)
	assert.NotContains(t, logs.String(), "wrong-key")
}

func TestNewAPIWiresInternalAPIKey(t *testing.T) {
	api := NewAPI("", testClientAPIKey, testInternalAPIKey, &http.Transport{}, nil, "", nil)
	t.Cleanup(api.Close)

	assert.Equal(t, testInternalAPIKey, api.internalAPIKey)
	assert.Equal(t, testClientAPIKey, api.clientAPIKey)
}
