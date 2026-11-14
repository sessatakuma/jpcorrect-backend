package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the router-level access-policy audit for #78 (spec §3.15,
// §1.3 "System administration and legacy raw CRUD").
//
// routePolicies classifies every route Register mounts. The tests below fail on
// two kinds of drift:
//
//   - a route that Register mounts but nobody classified (so adding a route
//     without deciding its access policy is a test failure), and
//   - a classified route that is not mounted any more.
//
// The behaviour itself (403 for a non-admin, 401 without a JWT or with an
// invalid one, admin access, raw-write semantics) is exercised against real
// rows in admin_gate_integration_test.go, driven by the same table through
// systemAdminRoutes.

// routePolicy is a route's access contract: what a caller needs before the
// handler runs.
type routePolicy string

const (
	// policyPublic needs no credentials at all. /ws is public today: the §4.3
	// JWT-at-upgrade check belongs to the Hub work (#62, #80), not to #78.
	policyPublic routePolicy = "public"

	// policyAPIKey accepts the api-tools service credential only. These are the
	// reverse-proxy routes tracked in #58: X-API-Key via APIKeyMiddleware, no
	// JWT, unchanged by #78.
	policyAPIKey routePolicy = "api-tools-api-key"

	// policyUser is a product route: a real JWT in every GIN_MODE, then the
	// handler's own §1.3 membership/owner/state checks. Admin is not special
	// here (no global bypass).
	policyUser routePolicy = "product-user"

	// policySystemAdmin is a retained or deprecated legacy raw CRUD management
	// route: a real JWT plus a stored domain.User.Role == admin, in every
	// GIN_MODE (#78).
	policySystemAdmin routePolicy = "system-admin"
)

// swaggerRoute is mounted in debug mode only.
const swaggerRoute = "GET /swagger/*any"

// routePolicies is the classification of every route Register mounts, grouped
// by policy. Keep it in sync with Register: the audit test names any route that
// is missing.
var routePolicies = map[string]routePolicy{
	// Public: no credentials.
	"GET /healthz": policyPublic,
	"GET /ws":      policyPublic,
	swaggerRoute:   policyPublic,

	// api-tools reverse proxy (#58): X-API-Key only, no JWT, out of scope here.
	"POST /v1/mark-accent":            policyAPIKey,
	"POST /v1/mark-accent/stream":     policyAPIKey,
	"POST /v1/usage-query/headwords":  policyAPIKey,
	"POST /v1/usage-query/url":        policyAPIKey,
	"POST /v1/usage-query/id-details": policyAPIKey,
	"POST /v1/dict-query":             policyAPIKey,
	"POST /v1/sentence-query":         policyAPIKey,

	// Product account routes (§3.1). The two invite-link routes are still the
	// interim membership/leader checks from #54; #81 owns their product
	// behavior.
	"POST /v1/users/init":                 policyUser,
	"GET /v1/users/me":                    policyUser,
	"PUT /v1/users/me":                    policyUser,
	"GET /v1/users/:id":                   policyUser,
	"GET /v1/users/name/:name":            policyUser,
	"POST /v1/guilds":                     policyUser,
	"GET /v1/guilds/:id":                  policyUser,
	"PUT /v1/guilds/:id":                  policyUser,
	"POST /v1/guilds/:id/transfer-leader": policyUser,
	"GET /v1/guilds/:id/invite-link":      policyUser,
	"POST /v1/guilds/:id/invite-link":     policyUser,

	// Legacy raw CRUD management (§3.15): admin only.
	"POST /v1/users":       policySystemAdmin,
	"PUT /v1/users/:id":    policySystemAdmin,
	"DELETE /v1/users/:id": policySystemAdmin,

	// Mistakes. GET /v1/mistakes/:id is here only as the interim gate: #100
	// replaces it with the §3.11.4 product read.
	"POST /v1/mistakes":                policySystemAdmin,
	"GET /v1/mistakes/:id":             policySystemAdmin,
	"PUT /v1/mistakes/:id":             policySystemAdmin,
	"DELETE /v1/mistakes/:id":          policySystemAdmin,
	"GET /v1/mistakes/event/:event_id": policySystemAdmin,
	"GET /v1/mistakes/user/:user_id":   policySystemAdmin,

	// Practices: deprecated, admin-only raw management (§3.4.3).
	"POST /v1/practices":              policySystemAdmin,
	"GET /v1/practices/:id":           policySystemAdmin,
	"PUT /v1/practices/:id":           policySystemAdmin,
	"DELETE /v1/practices/:id":        policySystemAdmin,
	"GET /v1/practices/user/:user_id": policySystemAdmin,

	// Guild attendees: deprecated, admin-only raw management (§3.3.2).
	"POST /v1/guild-attendees":                policySystemAdmin,
	"GET /v1/guild-attendees/:id":             policySystemAdmin,
	"PUT /v1/guild-attendees/:id":             policySystemAdmin,
	"DELETE /v1/guild-attendees/:id":          policySystemAdmin,
	"GET /v1/guild-attendees/guild/:guild_id": policySystemAdmin,
	"GET /v1/guild-attendees/user/:user_id":   policySystemAdmin,

	// Transcripts: retained raw management only.
	"POST /v1/transcripts":                policySystemAdmin,
	"GET /v1/transcripts/:id":             policySystemAdmin,
	"PUT /v1/transcripts/:id":             policySystemAdmin,
	"DELETE /v1/transcripts/:id":          policySystemAdmin,
	"GET /v1/transcripts/event/:event_id": policySystemAdmin,
	"GET /v1/transcripts/user/:user_id":   policySystemAdmin,

	// Event attendees: retained as admin-only raw management.
	"POST /v1/event-attendees":                policySystemAdmin,
	"GET /v1/event-attendees/:id":             policySystemAdmin,
	"PUT /v1/event-attendees/:id":             policySystemAdmin,
	"DELETE /v1/event-attendees/:id":          policySystemAdmin,
	"GET /v1/event-attendees/event/:event_id": policySystemAdmin,
	"GET /v1/event-attendees/user/:user_id":   policySystemAdmin,

	// The one retained guild route #78 owns (§3.3.1). Delete semantics are
	// unchanged; only access and the error envelope are gated.
	"DELETE /v1/guilds/:id": policySystemAdmin,
}

// systemAdminRoutes are the classified admin-only routes, in registration
// order-independent form. The integration tests iterate this slice, so a route
// classified here without a passing gate test cannot exist.
func systemAdminRoutes() []string {
	routes := make([]string, 0, len(routePolicies))
	for key, policy := range routePolicies {
		if policy == policySystemAdmin {
			routes = append(routes, key)
		}
	}
	return routes
}

// splitRouteKey splits a "METHOD /path" key from routePolicies.
func splitRouteKey(t *testing.T, key string) (method, path string) {
	t.Helper()

	method, path, found := strings.Cut(key, " ")
	require.True(t, found, "route key %q is not METHOD SPACE PATH", key)
	return method, path
}

var routeParamPattern = regexp.MustCompile(`:([a-zA-Z_]+)`)

// concretePath turns a gin route pattern into a request path by replacing every
// `:param` with a fresh UUID. A well-formed value is what a client would send;
// an existing row is deliberately not needed, because the gate under test
// decides before any path parameter is parsed or looked up.
func concretePath(t *testing.T, pattern string) string {
	t.Helper()

	return routeParamPattern.ReplaceAllStringFunc(pattern, func(string) string {
		return uuid.NewString()
	})
}

// newAuditAPI builds the API Register needs; no database is touched because
// these tests only register and reject.
func newAuditAPI(t *testing.T) *API {
	t.Helper()

	api := NewAPI("", "test-client-api-key", "test-internal-api-key",
		http.DefaultTransport.(*http.Transport).Clone(), nil, "", nil)
	t.Cleanup(api.Close)
	return api
}

// TestEveryRegisteredRouteHasAnAccessPolicy is the drift alarm #78 asks for:
// every route gin knows about is classified, and no classification is stale.
func TestEveryRegisteredRouteHasAnAccessPolicy(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)
			router := gin.New()
			Register(router, newAuditAPI(t))

			registered := make(map[string]bool)
			for _, route := range router.Routes() {
				key := route.Method + " " + route.Path
				registered[key] = true

				policy, classified := routePolicies[key]
				require.True(t, classified,
					"route %s has no access policy: classify it in routePolicies", key)
				assert.Contains(t,
					[]routePolicy{policyPublic, policyAPIKey, policyUser, policySystemAdmin},
					policy, "route %s has an unknown policy", key)
			}

			for key := range routePolicies {
				if registered[key] {
					continue
				}
				// Swagger is a debug-only route.
				assert.Equal(t, gin.ReleaseMode, mode,
					"route %s is classified but Register does not mount it", key)
				assert.Equal(t, swaggerRoute, key,
					"only the debug-only Swagger route may be absent in release mode")
			}

			// No /v1 route is anonymous: every one is either an api-tools
			// service route or an authenticated user/admin route.
			for key := range registered {
				_, path := splitRouteKey(t, key)
				if !strings.HasPrefix(path, "/v1/") {
					continue
				}
				assert.NotEqual(t, policyPublic, routePolicies[key],
					"/v1 route %s must not be public", key)
			}
		})
	}
}

// TestSystemAdminSetCoversTheIssueRoutes pins the size and the properties of
// the admin-only set, so #78's endpoint list cannot silently shrink.
func TestSystemAdminSetCoversTheIssueRoutes(t *testing.T) {
	routes := systemAdminRoutes()

	// 31 raw CRUD routes from the issue's Endpoints list, GET /v1/mistakes/:id
	// included as the interim gate, plus the retained DELETE /v1/guilds/:id.
	assert.Len(t, routes, 33)

	byGroup := map[string]int{}
	for _, key := range routes {
		_, path := splitRouteKey(t, key)
		parts := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
		byGroup[parts[0]]++
	}
	assert.Equal(t, map[string]int{
		"users":           3,
		"mistakes":        6,
		"practices":       5,
		"guild-attendees": 6,
		"transcripts":     6,
		"event-attendees": 6,
		"guilds":          1,
	}, byGroup)
}

// TestV1RoutesRequireAJWT proves AuthMiddleware is attached to every /v1 route
// that is not an api-tools service route: without credentials each answers 401
// through the §1.5 envelope, in both modes. The admin gate never turns a
// missing identity into a 403.
func TestV1RoutesRequireAJWT(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)
			router := gin.New()
			Register(router, newAuditAPI(t))

			for key, policy := range routePolicies {
				if policy != policyUser && policy != policySystemAdmin {
					continue
				}

				t.Run(key, func(t *testing.T) {
					method, pattern := splitRouteKey(t, key)
					req := httptest.NewRequest(method, concretePath(t, pattern), strings.NewReader(`{"broken":`))
					req.Header.Set("Content-Type", "application/json")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
					env := decodeEnvelope(t, rec)
					assert.Equal(t, "UNAUTHORIZED", env.Error.Code)
					assert.Empty(t, env.Error.Details)
				})
			}
		})
	}
}

// TestAPIToolsRoutesKeepTheAPIKeyPolicy pins that #78 left the api-tools group
// alone: in release mode all seven routes answer 401 without the service key
// and never ask for a JWT.
func TestAPIToolsRoutesKeepTheAPIKeyPolicy(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	Register(router, newAuditAPI(t))

	for key, policy := range routePolicies {
		if policy != policyAPIKey {
			continue
		}

		t.Run(key, func(t *testing.T) {
			method, pattern := splitRouteKey(t, key)
			req := httptest.NewRequest(method, pattern, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		})
	}
}
