package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"jpcorrect-backend/internal/domain"
)

// callerID reads the authenticated caller's local user ID from the gin
// context. AuthMiddleware and RequireLocalUser store a uuid.UUID; the debug
// caller middleware and handler tests may store the string form, so both are
// accepted.
func callerID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get("userID")
	if !exists {
		return uuid.Nil, false
	}
	switch v := value.(type) {
	case uuid.UUID:
		return v, v != uuid.Nil
	case string:
		id, err := uuid.Parse(v)
		if err != nil || id == uuid.Nil {
			return uuid.Nil, false
		}
		return id, true
	default:
		return uuid.Nil, false
	}
}

// requireCallerID returns the caller ID or writes 401 and aborts the chain.
func requireCallerID(c *gin.Context) (uuid.UUID, bool) {
	id, ok := callerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.Nil, false
	}
	return id, true
}

// RequireSystemAdmin allows only the trusted, server-resolved caller whose
// stored User.Role is admin (epic #61, issue #78). It reads the caller that
// RequireLocalUser resolved, or resolves it from the caller ID, and runs before
// any handler reads a request body: a role in the body or a custom JWT claim
// never grants access. It is deliberately separate from the product helpers
// below and does not give any product route an admin bypass.
func (a *API) RequireSystemAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := a.currentUser(c)
		if !ok {
			// currentUser already wrote the 401/500 response.
			c.Abort()
			return
		}
		if user.Role != domain.UserRoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "system admin role required"})
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

func (a *API) RequireGuildMember(c *gin.Context, guildID uuid.UUID) error {
	userID, ok := requireCallerID(c)
	if !ok {
		return domain.NewAuthError(http.StatusUnauthorized, "unauthorized", "")
	}
	_, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a guild member"})
		return err
	}
	return nil
}

func (a *API) RequireGuildLeader(c *gin.Context, guildID uuid.UUID) error {
	userID, ok := requireCallerID(c)
	if !ok {
		return domain.NewAuthError(http.StatusUnauthorized, "unauthorized", "")
	}
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a guild member"})
		return err
	}
	if attendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a guild leader"})
		return domain.NewAuthError(http.StatusForbidden, "not a guild leader", "")
	}
	return nil
}

func (a *API) RequireActivityMember(c *gin.Context, activityID uuid.UUID) error {
	activity, err := a.activityRepo.GetByID(c.Request.Context(), activityID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "activity not found"})
		return err
	}
	return a.RequireGuildMember(c, activity.GuildID)
}

func (a *API) RequireSelfOrGuildLeader(c *gin.Context, targetUserID, guildID uuid.UUID) error {
	userID, ok := requireCallerID(c)
	if !ok {
		return domain.NewAuthError(http.StatusUnauthorized, "unauthorized", "")
	}
	if userID == targetUserID {
		return nil
	}
	return a.RequireGuildLeader(c, guildID)
}

func (a *API) RequireMistakeOwner(c *gin.Context, mistakeID uuid.UUID) error {
	userID, ok := requireCallerID(c)
	if !ok {
		return domain.NewAuthError(http.StatusUnauthorized, "unauthorized", "")
	}
	mistake, err := a.mistakeRepo.GetByID(c.Request.Context(), mistakeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mistake not found"})
		return err
	}
	if mistake.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not the mistake owner"})
		return domain.NewAuthError(http.StatusForbidden, "not the mistake owner", "")
	}
	return nil
}
