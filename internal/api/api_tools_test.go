package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// recordedRequest snapshots the headers the upstream actually received.
type recordedRequest struct {
	headers http.Header
	method  string
	body    string
}

// startUpstream launches a test HTTP server that records the next inbound
// request and returns it via the returned channel.
func startUpstream(t *testing.T) (*httptest.Server, <-chan recordedRequest) {
	t.Helper()
	ch := make(chan recordedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		ch <- recordedRequest{headers: r.Header.Clone(), method: r.Method, body: string(body)}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return srv, ch
}

// newProxyAPI builds the minimum API fields exercised by proxyTo, avoiding the
// full NewAPI dependency on a live DB / JWKS.
func newProxyAPI(target string) *API {
	return &API{apiToolsURL: target, proxyTransport: http.DefaultTransport.(*http.Transport).Clone()}
}

// newProxyServer wires a single POST route through proxyTo and serves it over
// a real HTTP socket so the ReverseProxy gets a CloseNotifier-capable writer.
func newProxyServer(api *API, path, upstreamPath string) *httptest.Server {
	r := gin.New()
	r.POST(path, func(c *gin.Context) { api.proxyTo(c, upstreamPath, 0) })
	return httptest.NewServer(r)
}

func closeResponseBody(t *testing.T, body io.Closer) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}

func TestProxyTo_StripsInboundAuthHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream, rec := startUpstream(t)
	t.Cleanup(upstream.Close)

	api := newProxyAPI(upstream.URL)
	proxy := newProxyServer(api, "/v1/mark-accent", api.apiToolsURL+"/api/MarkAccent/")
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/mark-accent", strings.NewReader("{}"))
	req.Header.Set("X-API-Key", "shared-client-secret-from-CLIENT_API_KEY")
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "session=abc123; csrftoken=xyz")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy call: %v", err)
	}
	defer closeResponseBody(t, resp.Body)

	got, ok := <-rec
	if !ok {
		t.Fatal("upstream never received the proxied request")
	}
	if h := got.headers.Get("X-API-Key"); h != "" {
		t.Errorf("X-API-Key leaked upstream: %q", h)
	}
	if h := got.headers.Get("Authorization"); h != "" {
		t.Errorf("Authorization leaked upstream: %q", h)
	}
	if h := got.headers.Get("Cookie"); h != "" {
		t.Errorf("Cookie leaked upstream: %q", h)
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		t.Errorf("proxy status = %d, want %d", got, want)
	}
}

func TestProxyTo_PreservesNonAuthHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream, rec := startUpstream(t)
	t.Cleanup(upstream.Close)

	api := newProxyAPI(upstream.URL)
	proxy := newProxyServer(api, "/v1/dict-query", api.apiToolsURL+"/api/DictQuery/")
	t.Cleanup(proxy.Close)

	req, _ := http.NewRequest(http.MethodPost, proxy.URL+"/v1/dict-query", strings.NewReader("{}"))
	req.Header.Set("X-API-Key", "secret")
	req.Header.Set("Authorization", "Bearer jwt")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy call: %v", err)
	}
	defer closeResponseBody(t, resp.Body)

	got := <-rec
	if ct := got.headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type dropped: %q", ct)
	}
	if acc := got.headers.Get("Accept"); acc != "application/json" {
		t.Errorf("Accept dropped: %q", acc)
	}
}

// TestProxyTo_InvalidTarget exercises the url.Parse failure branch. The proxy
// must respond 500 with a JSON error rather than panicking or forwarding.
func TestProxyTo_InvalidTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// A space inside the scheme makes net/url reject the string outright.
	invalid := "ht tp://bad url"

	api := newProxyAPI(invalid)
	proxy := newProxyServer(api, "/", invalid)
	t.Cleanup(proxy.Close)

	resp, err := http.Post(proxy.URL+"/", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("proxy call: %v", err)
	}
	defer closeResponseBody(t, resp.Body)

	if got, want := resp.StatusCode, http.StatusInternalServerError; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Invalid target URL") {
		t.Errorf("expected JSON error message, got %q", string(body))
	}
}
