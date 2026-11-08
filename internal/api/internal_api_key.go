package api

import (
	"crypto/subtle"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

// InternalAPIKeyMiddleware requires the X-API-Key header to match
// INTERNAL_API_KEY. It guards the /internal group in every GIN_MODE, including
// debug; unlike the /v1 middlewares it has no debug bypass and never accepts a
// user JWT (§1.2 item 6, §3.9.1).
//
// It fails closed: an empty configured key rejects every request, so the
// process cannot expose an open internal group. The key value is never logged;
// rejection messages only describe the header.
func (a *API) InternalAPIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.internalAPIKey == "" {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "internal API key authentication is not configured"))
			c.Abort()
			return
		}

		providedKey := c.GetHeader("X-API-Key")
		if providedKey == "" {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "missing X-API-Key header"))
			c.Abort()
			return
		}

		if subtle.ConstantTimeCompare([]byte(providedKey), []byte(a.internalAPIKey)) != 1 {
			writeError(c, errorWithMessage(domain.ErrUnauthorized, "invalid API key"))
			c.Abort()
			return
		}

		c.Next()
	}
}
