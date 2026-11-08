package api

import (
	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

// currentUser returns the caller the authentication middleware attached to the
// request, or nil when it attached none.
//
// TEMPORARY: this is a stand-in for the helper issue #73 adds to
// internal/api/auth.go. It keeps #73's exact contract on purpose — the caller
// lives under the gin context key "user" as a *domain.User — so the permission
// helpers of issue #74 are already written against the final interface. Delete
// this file (and nothing else) once #73 has landed.
func currentUser(c *gin.Context) *domain.User {
	value, exists := c.Get("user")
	if !exists {
		return nil
	}

	caller, ok := value.(*domain.User)
	if !ok {
		return nil
	}

	return caller
}
