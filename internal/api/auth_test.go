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
		wantReached   bool
	}{
		{
			name:          "not configured, no header",
			configuredKey: "",
			setHeader:     false,
			wantStatus:    http.StatusUnauthorized,
			wantReached:   false,
		},
		{
			name:          "not configured, header present",
			configuredKey: "",
			setHeader:     true,
			headerValue:   "anything",
			wantStatus:    http.StatusUnauthorized,
			wantReached:   false,
		},
		{
			name:          "configured, missing header",
			configuredKey: "secret-key",
			setHeader:     false,
			wantStatus:    http.StatusUnauthorized,
			wantReached:   false,
		},
		{
			name:          "configured, wrong key",
			configuredKey: "secret-key",
			setHeader:     true,
			headerValue:   "wrong-key",
			wantStatus:    http.StatusUnauthorized,
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
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if reached != tt.wantReached {
				t.Errorf("downstream handler reached = %v, want %v", reached, tt.wantReached)
			}
		})
	}
}
