package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWebSocketAPI builds an API for the /ws tests. It goes through NewAPI on
// purpose: the upgrader's error callback is configured there, so a test that
// built &API{} by hand would assert against a different handshake path than the
// server runs.
func newWebSocketAPI(t *testing.T, allowedOrigins []string) *API {
	t.Helper()

	gin.SetMode(gin.TestMode)
	api := NewAPI("", "", "", http.DefaultTransport.(*http.Transport).Clone(), nil, "", allowedOrigins)
	t.Cleanup(api.Close)

	return api
}

// validHandshakeHeaders are the headers a browser sends for a well-formed
// upgrade request. Tests mutate one of them to reach a specific rejection.
func validHandshakeHeaders() map[string]string {
	return map[string]string{
		"Connection":            "Upgrade",
		"Upgrade":               "websocket",
		"Sec-Websocket-Version": "13",
		"Sec-Websocket-Key":     "dGhlIHNhbXBsZSBub25jZQ==",
	}
}

// TestWebSocketHandshakeRejectionUsesEnvelope pins the error envelope on every
// handshake failure gorilla/websocket answers by itself. Those rejections used
// to be gorilla's plain-text body, written before ServeWebSocket could see the
// error.
func TestWebSocketHandshakeRejectionUsesEnvelope(t *testing.T) {
	api := newWebSocketAPI(t, []string{"*"})

	r := gin.New()
	r.GET("/ws", api.ServeWebSocket)

	withHeader := func(overrides map[string]string) map[string]string {
		headers := validHandshakeHeaders()
		for name, value := range overrides {
			headers[name] = value
		}
		return headers
	}

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name:    "plain HTTP request, no upgrade headers",
			headers: nil,
		},
		{
			name:    "connection token is not upgrade",
			headers: withHeader(map[string]string{"Connection": "keep-alive"}),
		},
		{
			name:    "unsupported websocket version",
			headers: withHeader(map[string]string{"Sec-Websocket-Version": "8"}),
		},
		{
			name:    "malformed websocket key",
			headers: withHeader(map[string]string{"Sec-Websocket-Key": "not-base64"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
			// gorilla's default rejection advertises the supported version;
			// the envelope keeps it so a version-mismatch client can retry.
			assert.Equal(t, "13", rec.Header().Get("Sec-Websocket-Version"))

			env := decodeEnvelope(t, rec)
			assert.Equal(t, "INVALID_REQUEST", env.Error.Code)
			assert.Equal(t, domain.ErrInvalidRequest.Error(), env.Error.Message)
			assert.NotContains(t, rec.Body.String(), "websocket:",
				"the gorilla handshake reason must not reach the client")
		})
	}
}

// TestWebSocketOriginRejectionUsesEnvelope covers the 403 branch: CheckOrigin
// refuses an Origin that ALLOWED_ORIGINS does not list. gorilla would answer it
// with the plain-text body "Forbidden".
func TestWebSocketOriginRejectionUsesEnvelope(t *testing.T) {
	api := newWebSocketAPI(t, []string{"https://talkuma.example"})

	r := gin.New()
	r.GET("/ws", api.ServeWebSocket)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	for name, value := range validHandshakeHeaders() {
		req.Header.Set(name, value)
	}
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	env := decodeEnvelope(t, rec)
	assert.Equal(t, "FORBIDDEN", env.Error.Code)
	assert.Equal(t, domain.ErrForbidden.Error(), env.Error.Message)
	assert.NotContains(t, rec.Body.String(), "CheckOrigin",
		"the gorilla handshake reason must not reach the client")
}

// TestWebSocketMethodRejectionUsesEnvelope pins the fallback for a handshake
// status §1.5 does not name. Register mounts /ws as GET only, so gin answers a
// non-GET request with 404 before the handler runs; this route reaches the
// upgrader's 405 branch and must still emit the envelope rather than text.
func TestWebSocketMethodRejectionUsesEnvelope(t *testing.T) {
	api := newWebSocketAPI(t, []string{"*"})

	r := gin.New()
	r.Any("/ws", api.ServeWebSocket)

	req := httptest.NewRequest(http.MethodPost, "/ws", nil)
	for name, value := range validHandshakeHeaders() {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	env := decodeEnvelope(t, rec)
	assert.Equal(t, "INVALID_REQUEST", env.Error.Code)
}

// TestWebSocketUpgradeStillSucceeds guards the success path: the error callback
// must not interfere with a real upgrade, and the server still greets the
// client with its assigned id.
func TestWebSocketUpgradeStillSucceeds(t *testing.T) {
	const origin = "https://talkuma.example"
	api := newWebSocketAPI(t, []string{origin})

	r := gin.New()
	r.GET("/ws", api.ServeWebSocket)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	header := http.Header{"Origin": []string{origin}}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	var msg Message
	require.NoError(t, conn.ReadJSON(&msg))
	require.Equal(t, "connected", msg.Type)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	assert.NotEmpty(t, payload["id"], "the server assigns the client an id")

	_, ok := api.webrtcHub.GetClient(payload["id"])
	assert.True(t, ok, "the upgraded client is registered in the hub")
}
