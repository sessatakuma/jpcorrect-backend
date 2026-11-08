package api

import "github.com/gin-gonic/gin"

// internalRoutes mounts the /internal group and is the only place internal
// routes are registered, so no route under it can escape
// InternalAPIKeyMiddleware in any GIN_MODE. This issue ships the group and the
// key contract only; the internal endpoints themselves are added by the issues
// that introduce them (§3.9.1, §3.14).
func (api *API) internalRoutes(r gin.IRouter) *gin.RouterGroup {
	group := r.Group("/internal")
	group.Use(api.InternalAPIKeyMiddleware())
	return group
}
