package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"jpcorrect-backend/internal/domain"
)

func (a *API) RequireGuildMember(c *gin.Context, guildID uuid.UUID) error {
	userID, _ := c.Get("userID")
	_, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a guild member"})
		return err
	}
	return nil
}

func (a *API) RequireGuildLeader(c *gin.Context, guildID uuid.UUID) error {
	userID, _ := c.Get("userID")
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID.(uuid.UUID))
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
	userID, _ := c.Get("userID")
	if userID.(uuid.UUID) == targetUserID {
		return nil
	}
	return a.RequireGuildLeader(c, guildID)
}

func (a *API) RequireMistakeOwner(c *gin.Context, mistakeID uuid.UUID) error {
	userID, _ := c.Get("userID")
	mistake, err := a.mistakeRepo.GetByID(c.Request.Context(), mistakeID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mistake not found"})
		return err
	}
	if mistake.UserID != userID.(uuid.UUID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "not the mistake owner"})
		return domain.NewAuthError(http.StatusForbidden, "not the mistake owner", "")
	}
	return nil
}

func (a *API) RequireActivityVisible(c *gin.Context, activityID uuid.UUID) error {
	return a.RequireActivityMember(c, activityID)
}
