package api

import (
	"errors"
	"net/http"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// @Summary Get invite link info
// @Description Get guild info for an invite link by token. Public endpoint — no authentication required.
// @Tags invites
// @Accept json
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/invites/{token} [get]
func (a *API) InviteInfoHandler(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	inviteLink, err := a.inviteLinkRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	isExpired := time.Now().After(inviteLink.ExpiresAt)

	guild, err := a.guildRepo.GetByID(c.Request.Context(), inviteLink.GuildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"guild": gin.H{
			"name":        guild.Name,
			"description": guild.Description,
		},
		"is_expired": isExpired,
	})
}

// @Summary Accept an invite link
// @Description Accept an invite link to join a guild. Requires authentication.
// @Tags invites
// @Accept json
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/invites/{token}/accept [post]
// @Security BearerAuth
func (a *API) InviteAcceptHandler(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	userIDRaw, _ := c.Get("userID")
	userID := userIDRaw.(uuid.UUID)

	inviteLink, err := a.inviteLinkRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "invite not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if time.Now().After(inviteLink.ExpiresAt) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invite expired"})
		return
	}

	// Check user not already in this guild
	existing, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), inviteLink.GuildID, userID)
	if err == nil && existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "already a member of this guild"})
		return
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check user has fewer than 3 guilds
	userGuilds, err := a.guildAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(userGuilds) >= 3 {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot join more than 3 guilds"})
		return
	}

	// Transaction: create GuildAttendee + cancel pending JoinRequests
	now := time.Now()
	attendee := &domain.GuildAttendee{
		ID:       uuid.New(),
		GuildID:  inviteLink.GuildID,
		UserID:   userID,
		Role:     domain.GuildAttendeeRoleMember,
		JoinedAt: &now,
	}

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(attendee).Error; err != nil {
			return err
		}
		return a.joinRequestRepo.CancelPendingByUserID(c.Request.Context(), userID)
	})
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "already a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "joined guild successfully"})
}
