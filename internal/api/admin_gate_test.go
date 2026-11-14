package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file pins the access policy of the three legacy raw user write routes
// (POST /v1/users, PUT /v1/users/{id}, DELETE /v1/users/{id}) at the router
// level, in both gin modes, with the repositories mocked: the gate decides on
// the stored User.Role that AuthMiddleware resolved, never on a JWT claim or a
// request body, and it decides before the handler reads the body. The product
// routes keep their open access. The real-PostgreSQL half, including a real JWT
// takeover sequence, lives in user_takeover_integration_test.go.

// malformedRawUserWrite is deliberately broken JSON: a caller that gets 403
// never parsed it, so a 403 also proves the gate ran first, while an admin
// getting 400 shows the gate let the request through to the raw handler.
const malformedRawUserWrite = `{"name":`

// rawUserWriteRoute is one gated route and the outcome an admin gets on it. The
// non-admin outcome is 403 FORBIDDEN on all three.
type rawUserWriteRoute struct {
	method, path, body string
	adminStatus        int
	adminCode          string
}

// rawUserWriteRoutes are the three gated routes, targeting targetID.
func rawUserWriteRoutes(targetID string) []rawUserWriteRoute {
	return []rawUserWriteRoute{
		{http.MethodPost, "/v1/users", `{"email":"raw@example.com","name":"raw"}`, http.StatusCreated, ""},
		{http.MethodPut, "/v1/users/" + targetID, malformedRawUserWrite, http.StatusBadRequest, "INVALID_REQUEST"},
		{http.MethodDelete, "/v1/users/" + targetID, "", http.StatusNoContent, ""},
	}
}

// TestRawUserWriteRoutesRequireSystemAdmin walks the gate over the three routes
// for every role a caller can hold, in debug and in release mode.
func TestRawUserWriteRoutesRequireSystemAdmin(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	cases := []struct {
		name string
		// role is the stored User.Role AuthMiddleware resolves.
		role domain.UserRole
		// claim is an extra "role" claim on the token.
		claim string
		admin bool
	}{
		{"plain user", domain.UserRoleUser, "", false},
		{"plain user with an admin claim", domain.UserRoleUser, "admin", false},
		{"staff", domain.UserRoleStaff, "", false},
		{"staff with an admin claim", domain.UserRoleStaff, "admin", false},
		{"admin", domain.UserRoleAdmin, "", true},
	}

	for _, mode := range []string{gin.DebugMode, gin.ReleaseMode} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(mode)

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					caller := storedUser(t, uuid.New(), "caller@example.com")
					caller.Role = tc.role
					target := storedUser(t, uuid.New(), "target@example.com")

					for _, route := range rawUserWriteRoutes(target.ID.String()) {
						t.Run(route.method, func(t *testing.T) {
							f := newUserRouterFixture(t, caller)
							// The raw handlers read the row they write.
							f.users.getByIDResult = target

							token := f.token
							if tc.claim != "" {
								token = f.signer.rawToken(t, jwt.MapClaims{
									"sub":   f.subject.String(),
									"email": caller.Email,
									"role":  tc.claim,
								})
							}

							rec := f.do(route.method, route.path, route.body, "Bearer "+token, "")

							if tc.admin {
								require.Equal(t, route.adminStatus, rec.Code, rec.Body.String())
								if route.adminCode != "" {
									assert.Equal(t, route.adminCode, decodeEnvelope(t, rec).Error.Code)
								}
								return
							}

							// Every other caller is refused, on every route, in the
							// §1.5 envelope, before the body is parsed or the row
							// is touched.
							require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
							assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
							assert.Empty(t, f.users.createCalls, "the raw create must not run")
							assert.Empty(t, f.users.updateCalls, "the raw update must not run")
							assert.Empty(t, f.users.deleteCalls, "the raw delete must not run")
						})
					}
				})
			}
		})
	}
}

// TestRawUserWriteRoutesWithoutCredentials proves the gate never turns a missing
// credential into a 403: AuthMiddleware answers 401 first.
func TestRawUserWriteRoutesWithoutCredentials(t *testing.T) {
	target := storedUser(t, uuid.New(), "target@example.com")

	for _, route := range rawUserWriteRoutes(target.ID.String()) {
		t.Run(route.method, func(t *testing.T) {
			f := newUserRouterFixture(t, storedUser(t, uuid.New(), "caller@example.com"))
			rec := f.do(route.method, route.path, route.body, "", "")
			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
			assert.Empty(t, f.users.createCalls)
			assert.Empty(t, f.users.deleteCalls)
		})
	}
}

// TestSystemAdminGateWithoutResolvedCaller covers RequireSystemAdmin on its own,
// mounted where nothing resolved a caller: it must answer 401, not 403, because
// the caller is unknown rather than unauthorized.
func TestSystemAdminGateWithoutResolvedCaller(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	gin.SetMode(gin.TestMode)

	api := &API{}
	router := gin.New()
	router.PUT("/guarded", api.RequireSystemAdmin(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/guarded", strings.NewReader(`{}`)))

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
}

// TestSystemAdminGateAllowsOnlyAdmin pins the classification helper: admin is the
// only system administrator. A missing row, staff, a plain user and an empty role
// are not.
func TestSystemAdminGateAllowsOnlyAdmin(t *testing.T) {
	assert.True(t, IsSystemAdmin(&domain.User{Role: domain.UserRoleAdmin}))
	assert.False(t, IsSystemAdmin(nil))
	assert.False(t, IsSystemAdmin(&domain.User{Role: domain.UserRoleUser}))
	assert.False(t, IsSystemAdmin(&domain.User{Role: domain.UserRoleStaff}))
	assert.False(t, IsSystemAdmin(&domain.User{Role: ""}))
}

// TestProductUserRoutesStayOpenToEveryRole is the "nothing else changed" half: a
// plain user keeps full access to the product routes, and an admin gains no
// bypass on them.
func TestProductUserRoutesStayOpenToEveryRole(t *testing.T) {
	for _, role := range []domain.UserRole{domain.UserRoleUser, domain.UserRoleStaff, domain.UserRoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			caller := storedUser(t, uuid.New(), "caller@example.com")
			caller.Role = role
			target := storedUser(t, uuid.New(), "target@example.com")

			f := newUserRouterFixture(t, caller)
			f.users.getByIDResult = caller

			// Own profile, read and write: the product write path is open.
			rec := f.do(http.MethodGet, "/v1/users/me", "", "Bearer "+f.token, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			rec = f.do(http.MethodPut, "/v1/users/me", `{"name":"renamed"}`, "Bearer "+f.token, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Len(t, f.users.profileUpdates, 1, "the product write path is not gated")
			assert.Empty(t, f.users.updateCalls, "and it still does not save the whole row")

			// Another user's public profile.
			f.users.getByIDResult = target
			rec = f.do(http.MethodGet, "/v1/users/"+target.ID.String(), "", "Bearer "+f.token, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			// First login stays open too.
			rec = f.do(http.MethodPost, "/v1/users/init", `{}`, "Bearer "+f.token, "")
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		})
	}
}
