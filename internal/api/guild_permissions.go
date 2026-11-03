package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"jpcorrect-backend/internal/domain"
)

func callerID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return uuid.Nil, false
	}
	id, ok := value.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return uuid.Nil, false
	}
	return id, true
}

// requireActiveMaster writes an error response on failure. Callers must return
// immediately when it returns false, before reading or mutating protected data.
func (a *API) requireActiveMaster(c *gin.Context, guildID uuid.UUID) bool {
	userID, ok := callerID(c)
	if !ok {
		return false
	}
	ctx := c.Request.Context()
	if _, err := a.guildRepo.GetByID(ctx, guildID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		}
		return false
	}
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, userID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify caller permission"})
		return false
	}
	if err != nil || attendee == nil || attendee.LeftAt != nil || attendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only an active guild master can perform this operation"})
		return false
	}
	return true
}
