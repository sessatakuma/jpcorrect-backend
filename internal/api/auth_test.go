package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// newAuthAPI builds the minimum API fields exercised by APIKeyMiddleware,
// avoiding the full NewAPI dependency on a live DB / JWKS.
func newAuthAPI(clientAPIKey string) *API {
	return &API{clientAPIKey: clientAPIKey}
}

// newAuthServer wires a single GET route through APIKeyMiddleware and a trivial
// downstream handler that flips *reached and returns 200 when it runs.
func newAuthServer(api *API, reached *bool) *gin.Engine {
	r := gin.New()
	r.GET("/protected", api.APIKeyMiddleware(), func(c *gin.Context) {
		*reached = true
		c.Status(http.StatusOK)
	})
	return r
}

func TestAPIKeyMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		configuredKey string
		setHeader     bool
		headerValue   string
		wantStatus    int
		wantCode      string
		wantMessage   string
		wantReached   bool
	}{
		{
			name:          "not configured, no header",
			configuredKey: "",
			setHeader:     false,
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "UNAUTHORIZED",
			wantMessage:   "API key authentication is not configured",
			wantReached:   false,
		},
		{
			name:          "not configured, header present",
			configuredKey: "",
			setHeader:     true,
			headerValue:   "anything",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "UNAUTHORIZED",
			wantMessage:   "API key authentication is not configured",
			wantReached:   false,
		},
		{
			name:          "configured, missing header",
			configuredKey: "secret-key",
			setHeader:     false,
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "UNAUTHORIZED",
			wantMessage:   "missing X-API-Key header",
			wantReached:   false,
		},
		{
			name:          "configured, wrong key",
			configuredKey: "secret-key",
			setHeader:     true,
			headerValue:   "wrong-key",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "UNAUTHORIZED",
			wantMessage:   "invalid API key",
			wantReached:   false,
		},
		{
			name:          "configured, correct key",
			configuredKey: "secret-key",
			setHeader:     true,
			headerValue:   "secret-key",
			wantStatus:    http.StatusOK,
			wantReached:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			api := newAuthAPI(tt.configuredKey)
			r := newAuthServer(api, &reached)

			req, err := http.NewRequest(http.MethodGet, "/protected", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if tt.setHeader {
				req.Header.Set("X-API-Key", tt.headerValue)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if reached != tt.wantReached {
				t.Errorf("downstream handler reached = %v, want %v", reached, tt.wantReached)
			}

			// A rejection must use the §1.5 envelope; decodeEnvelope also
			// rejects the retired {"error": "...", "details": "..."} shape.
			if tt.wantCode == "" {
				return
			}
			env := decodeEnvelope(t, w)
			if env.Error.Code != tt.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tt.wantCode)
			}
			if env.Error.Message != tt.wantMessage {
				t.Errorf("error.message = %q, want %q", env.Error.Message, tt.wantMessage)
			}
		})
	}
}

// TestAuthMiddlewareEnvelope pins the JWT middleware's rejection shape and
// status codes. JWKS is not initialized, so a token cannot be validated and the
// request fails as an internal error — the same status it had before the
// envelope migration.
func TestAuthMiddlewareEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
		wantCode      string
		wantMessage   string
	}{
		{
			name:        "missing header",
			wantStatus:  http.StatusUnauthorized,
			wantCode:    "UNAUTHORIZED",
			wantMessage: "missing authorization header",
		},
		{
			name:          "wrong scheme",
			authorization: "Basic abc",
			wantStatus:    http.StatusUnauthorized,
			wantCode:      "UNAUTHORIZED",
			wantMessage:   "invalid authorization header format",
		},
		{
			name:          "token without JWKS",
			authorization: "Bearer unused-token",
			wantStatus:    http.StatusInternalServerError,
			wantCode:      "INTERNAL_ERROR",
			wantMessage:   "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newAuthAPI("")
			reached := false
			r := gin.New()
			r.GET("/protected", api.AuthMiddleware(), func(c *gin.Context) {
				reached = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if reached {
				t.Error("the protected handler must not run without valid credentials")
			}

			env := decodeEnvelope(t, w)
			if env.Error.Code != tt.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tt.wantCode)
			}
			if env.Error.Message != tt.wantMessage {
				t.Errorf("error.message = %q, want %q", env.Error.Message, tt.wantMessage)
			}
		})
	}
}
