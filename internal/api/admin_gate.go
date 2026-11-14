package api

import (
	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

// This file holds the system-administration gate of the retained legacy raw user
// write routes: POST /v1/users, PUT /v1/users/{id} and DELETE /v1/users/{id}. The
// policy is the one issue #78 approved for every retained raw CRUD management
// route (spec §1.3 "System administration and legacy raw CRUD", §3.15).
//
// The product routes (POST /v1/users/init, GET/PUT /v1/users/me, GET
// /v1/users/{id}, GET /v1/users/name/{name}) are untouched: AuthMiddleware alone
// opens them, in every GIN_MODE.

// IsSystemAdmin reports whether user is a system administrator: a caller whose
// trusted, server-resolved stored role is admin (spec §1.3).
//
// Guild leadership (GuildAttendee.Role == master) is not system administration,
// staff does not imply it, and a role carried by a request body or by a custom
// JWT claim is never consulted: user is always the row AuthMiddleware resolved
// from the token's subject.
func IsSystemAdmin(user *domain.User) bool {
	return user != nil && user.Role == domain.UserRoleAdmin
}

// RequireSystemAdmin is the route-level gate of the retained legacy raw user
// write routes. Mount it after AuthMiddleware on the route; it allows only
// IsSystemAdmin(caller), and answers 401 for a request without a resolved caller
// and 403 for every other role, in every GIN_MODE, through the §1.5 envelope.
//
// The gate runs before the request body is read because it is middleware, so a
// rejected caller cannot even have a candidate row parsed, let alone bound. It
// is a route gate, not a product permission, and no product route consults it:
// being an admin grants no bypass anywhere else.
func (a *API) RequireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		caller := currentUser(c)
		if caller == nil {
			writeError(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}

		if !IsSystemAdmin(caller) {
			writeError(c, errorWithMessage(domain.ErrForbidden, "system administrator role required"))
			c.Abort()
			return
		}

		c.Next()
	}
}
