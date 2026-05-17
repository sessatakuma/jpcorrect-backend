package api

import (
	crypto_rand "crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// @Summary Get a guild by ID
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id} [get]
// @Security BearerAuth
func (a *API) GuildGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	if err := a.RequireGuildMember(c, id); err != nil {
		return
	}

	guild, err := a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, guild)
}

// @Summary Create a guild
// @Description Create a new guild with the caller as master
// @Tags guilds
// @Accept json
// @Produce json
// @Param guild body domain.Guild true "Guild data"
// @Success 201 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds [post]
// @Security BearerAuth
func (a *API) GuildCreateHandler(c *gin.Context) {
	userIDRaw, _ := c.Get("userID")
	userID := userIDRaw.(uuid.UUID)

	attendees, err := a.guildAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(attendees) >= 1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "already in a guild"})
		return
	}

	var guild domain.Guild
	if err := c.ShouldBindJSON(&guild); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&guild).Error; err != nil {
			return err
		}
		attendee := &domain.GuildAttendee{
			GuildID:  guild.ID,
			UserID:   userID,
			Role:     domain.GuildAttendeeRoleMaster,
			JoinedAt: &now,
		}
		return tx.Create(attendee).Error
	})
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "Guild already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, guild)
}

// @Summary Update a guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param guild body domain.Guild true "Guild data"
// @Success 200 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id} [put]
// @Security BearerAuth
func (a *API) GuildUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	if err := a.RequireGuildLeader(c, id); err != nil {
		return
	}

	_, err = a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var guild domain.Guild
	if err := c.ShouldBindJSON(&guild); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	guild.ID = id
	if err := a.guildRepo.Update(c.Request.Context(), &guild); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "Guild already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id} [delete]
func (a *API) GuildDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	if err := a.RequireGuildLeader(c, id); err != nil {
		return
	}

	_, err = a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.guildRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete guild: has related records"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get a guild attendee by ID
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param id path string true "GuildAttendee ID"
// @Success 200 {object} domain.GuildAttendee
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees/{id} [get]
func (a *API) GuildAttendeeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "GuildAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendee)
}

// @Summary Create a guild attendee
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param attendee body domain.GuildAttendee true "GuildAttendee data"
// @Success 201 {object} domain.GuildAttendee
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees [post]
func (a *API) GuildAttendeeCreateHandler(c *gin.Context) {
	var attendee domain.GuildAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.guildAttendeeRepo.Create(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "GuildAttendee already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, attendee)
}

// @Summary Update a guild attendee
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param id path string true "GuildAttendee ID"
// @Param attendee body domain.GuildAttendee true "GuildAttendee data"
// @Success 200 {object} domain.GuildAttendee
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees/{id} [put]
func (a *API) GuildAttendeeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	_, err = a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "GuildAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var attendee domain.GuildAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attendee.ID = id
	if err := a.guildAttendeeRepo.Update(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "GuildAttendee already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a guild attendee
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param id path string true "GuildAttendee ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees/{id} [delete]
func (a *API) GuildAttendeeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	_, err = a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "GuildAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.guildAttendeeRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete guild attendee: has related records"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get guild attendees by guild ID
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param guild_id path string true "Guild ID"
// @Success 200 {array} domain.GuildAttendee
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees/guild/{guild_id} [get]
func (a *API) GuildAttendeeGetByGuildHandler(c *gin.Context) {
	guildIDStr := c.Param("guild_id")
	guildID, err := uuid.Parse(guildIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendees, err := a.guildAttendeeRepo.GetByGuildID(c.Request.Context(), guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendees)
}

// @Summary Get guild attendees by user ID
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {array} domain.GuildAttendee
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guild-attendees/user/{user_id} [get]
func (a *API) GuildAttendeeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendees, err := a.guildAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendees)
}

// GuildMembersHandler returns the list of guild members
// @Summary List guild members
// @Description Get all members of a guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {array} domain.GuildAttendee
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/members [get]
// @Security BearerAuth
func (a *API) GuildMembersHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildMember(c, guildID); err != nil {
		return
	}

	members, err := a.guildAttendeeRepo.GetByGuildID(c.Request.Context(), guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get members"})
		return
	}
	c.JSON(http.StatusOK, members)
}

// GuildMemberRemoveHandler removes a member from the guild
// @Summary Remove guild member
// @Description Remove a member from the guild (leader only, cannot remove self)
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Param user_id path string true "User ID to remove"
// @Success 200 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/members/{user_id} [delete]
// @Security BearerAuth
func (a *API) GuildMemberRemoveHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	userID, _ := c.Get("userID")
	if userID.(uuid.UUID) == targetUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove yourself, use leave instead"})
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, targetUserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	if err := a.guildAttendeeRepo.Delete(c.Request.Context(), attendee.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

// GuildLeaveHandler allows a member to leave the guild
// @Summary Leave guild
// @Description Leave the guild. Leader must transfer leadership first.
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/leave [post]
// @Security BearerAuth
func (a *API) GuildLeaveHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildMember(c, guildID); err != nil {
		return
	}

	userID, _ := c.Get("userID")
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "membership not found"})
		return
	}

	if attendee.Role == domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusBadRequest, gin.H{"error": "leader must transfer leadership before leaving"})
		return
	}

	if err := a.guildAttendeeRepo.Delete(c.Request.Context(), attendee.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to leave guild"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "left guild"})
}

// @Summary Submit guild application
// @Description Submit a join request to a guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 201 {object} domain.JoinRequest
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /v1/guilds/{id}/applications [post]
// @Security BearerAuth
func (a *API) GuildApplicationCreateHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}

	userID, _ := c.Get("userID")

	_, err = a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID.(uuid.UUID))
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "already a member"})
		return
	}

	// Check no pending application
	existing, _ := a.joinRequestRepo.GetByUserID(c.Request.Context(), userID.(uuid.UUID))
	for _, req := range existing {
		if req.GuildID == guildID && req.Status == domain.JoinRequestStatusPending {
			c.JSON(http.StatusConflict, gin.H{"error": "already have a pending application"})
			return
		}
	}

	req := &domain.JoinRequest{
		GuildID: guildID,
		UserID:  userID.(uuid.UUID),
		Status:  domain.JoinRequestStatusPending,
	}

	if err := a.joinRequestRepo.Create(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create application"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusCreated, req)
}

// @Summary List guild applications
// @Description List pending join requests for a guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {array} domain.JoinRequest
// @Failure 403 {object} map[string]string
// @Router /v1/guilds/{id}/applications [get]
// @Security BearerAuth
func (a *API) GuildApplicationsHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	pending := domain.JoinRequestStatusPending
	applications, err := a.joinRequestRepo.GetByGuildID(c.Request.Context(), guildID, &pending)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get applications"})
		return
	}

	c.JSON(http.StatusOK, applications)
}

// @Summary Approve guild application
// @Description Approve a join request and add member to guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Param app_id path string true "Application ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /v1/guilds/{id}/applications/{app_id}/approve [post]
// @Security BearerAuth
func (a *API) GuildApplicationApproveHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	appID, err := uuid.Parse(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application id"})
		return
	}

	req, err := a.joinRequestRepo.GetByID(c.Request.Context(), appID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
		return
	}

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Approve the request
		req.Status = domain.JoinRequestStatusApproved
		if err := a.joinRequestRepo.Update(c.Request.Context(), req); err != nil {
			return err
		}

		// Create guild attendee
		now := time.Now()
		attendee := &domain.GuildAttendee{
			GuildID:  guildID,
			UserID:   req.UserID,
			Role:     domain.GuildAttendeeRoleMember,
			JoinedAt: &now,
		}
		if err := a.guildAttendeeRepo.Create(c.Request.Context(), attendee); err != nil {
			return err
		}

		// Cancel user's other pending requests
		return a.joinRequestRepo.CancelPendingByUserID(c.Request.Context(), req.UserID)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve application"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "application approved"})
}

// @Summary Reject guild application
// @Description Reject a join request
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Param app_id path string true "Application ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /v1/guilds/{id}/applications/{app_id}/reject [post]
// @Security BearerAuth
func (a *API) GuildApplicationRejectHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	appID, err := uuid.Parse(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application id"})
		return
	}

	req, err := a.joinRequestRepo.GetByID(c.Request.Context(), appID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
		return
	}

	req.Status = domain.JoinRequestStatusRejected
	if err := a.joinRequestRepo.Update(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject application"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "application rejected"})
}

// @Summary Get guild invite link
// @Description Get the current active invite link for the guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} domain.InviteLink
// @Failure 403 {object} map[string]string
// @Router /v1/guilds/{id}/invite-link [get]
// @Security BearerAuth
func (a *API) GuildInviteLinkGetHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	links, err := a.inviteLinkRepo.GetByGuildID(c.Request.Context(), guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get invite link"})
		return
	}

	now := time.Now()
	for _, link := range links {
		if link.ExpiresAt.After(now) {
			c.JSON(http.StatusOK, link)
			return
		}
	}

	c.JSON(http.StatusOK, nil)
}

// @Summary Create guild invite link
// @Description Generate a new invite link for the guild (expires old ones)
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 201 {object} domain.InviteLink
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/invite-link [post]
// @Security BearerAuth
func (a *API) GuildInviteLinkCreateHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	userIDRaw, _ := c.Get("userID")
	userID := userIDRaw.(uuid.UUID)

	tokenBytes := make([]byte, 16)
	if _, err := crypto_rand.Read(tokenBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	token := hex.EncodeToString(tokenBytes)

	var newLink *domain.InviteLink
	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		oldLinks, err := a.inviteLinkRepo.GetByGuildID(c.Request.Context(), guildID)
		if err == nil {
			for _, link := range oldLinks {
				if link.ExpiresAt.After(now) {
					link.ExpiresAt = now
					if err := a.inviteLinkRepo.Update(c.Request.Context(), link); err != nil {
						// Log but don't fail — expiring old links is best-effort
					}
				}
			}
		}

		newLink = &domain.InviteLink{
			GuildID:         guildID,
			Token:           token,
			ExpiresAt:       now.Add(7 * 24 * time.Hour),
			CreatedByUserID: userID,
		}
		return a.inviteLinkRepo.Create(c.Request.Context(), newLink)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite link"})
		return
	}

	c.JSON(http.StatusCreated, newLink)
}

// @Summary Get guild default slot
// @Description Get the default scheduling slot for a guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} domain.GuildDefaultSlot
// @Failure 403 {object} map[string]string
// @Router /v1/guilds/{id}/default-slot [get]
// @Security BearerAuth
func (a *API) GuildDefaultSlotGetHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildMember(c, guildID); err != nil {
		return
	}

	slot, err := a.guildDefaultSlotRepo.GetByGuildID(c.Request.Context(), guildID)
	if err != nil {
		c.JSON(http.StatusOK, nil)
		return
	}
	c.JSON(http.StatusOK, slot)
}

// @Summary Upsert guild default slot
// @Description Set or update the default scheduling slot for a guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} domain.GuildDefaultSlot
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /v1/guilds/{id}/default-slot [put]
// @Security BearerAuth
func (a *API) GuildDefaultSlotUpsertHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	var input struct {
		DayOfWeek int    `json:"day_of_week"`
		TimeOfDay string `json:"time_of_day"`
		Timezone  string `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	slot := &domain.GuildDefaultSlot{
		GuildID:   guildID,
		DayOfWeek: input.DayOfWeek,
		TimeOfDay: input.TimeOfDay,
	}
	if input.Timezone != "" {
		slot.Timezone = input.Timezone
	}

	if err := a.guildDefaultSlotRepo.Upsert(c.Request.Context(), slot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert default slot"})
		return
	}

	c.JSON(http.StatusOK, slot)
}

// @Summary Delete guild default slot
// @Description Remove the default scheduling slot for a guild
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Router /v1/guilds/{id}/default-slot [delete]
// @Security BearerAuth
func (a *API) GuildDefaultSlotDeleteHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	if err := a.guildDefaultSlotRepo.Delete(c.Request.Context(), guildID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete default slot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "default slot deleted"})
}

// @Summary Transfer guild leadership
// @Description Transfer guild leader role to another member
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param body body object true "New leader" example({"new_leader_user_id": "uuid"})
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/transfer-leader [post]
// @Security BearerAuth
func (a *API) GuildTransferLeaderHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	var body struct {
		NewLeaderUserID string `json:"new_leader_user_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	newLeaderID, err := uuid.Parse(body.NewLeaderUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	userIDRaw, _ := c.Get("userID")
	userID := userIDRaw.(uuid.UUID)

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.GuildAttendee{}).
			Where("guild_id = ? AND user_id = ?", guildID, userID).
			Update("role", domain.GuildAttendeeRoleMember).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.GuildAttendee{}).
			Where("guild_id = ? AND user_id = ?", guildID, newLeaderID).
			Update("role", domain.GuildAttendeeRoleMaster).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to transfer leadership"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "leadership transferred"})
}

// GuildDiscoverHandler lists public guilds ordered by most recent activity
// @Summary Discover public guilds
// @Description List public guilds ordered by most recent activity time with pagination
// @Tags guilds
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(20)
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/discover [get]
// @Security BearerAuth
func (a *API) GuildDiscoverHandler(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	ctx := c.Request.Context()

	type guildDiscoverRow struct {
		domain.Guild
		MemberCount     int64         `json:"member_count"`
		LastActivityAt  *time.Time    `json:"last_activity_at"`
	}

	var results []guildDiscoverRow
	err := a.db.WithContext(ctx).
		Table("guild").
		Select("guild.*, COUNT(DISTINCT guild_attendee.guild_attendee_id) AS member_count, MAX(activity.created_at) AS last_activity_at").
		Joins("LEFT JOIN guild_attendee ON guild_attendee.guild_id = guild.guild_id").
		Joins("LEFT JOIN activity ON activity.guild_id = guild.guild_id").
		Where("guild.deleted_at IS NULL").
		Group("guild.guild_id").
		Order("last_activity_at DESC NULLS LAST").
		Limit(perPage).
		Offset(offset).
		Find(&results).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch guilds"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"guilds": results,
		"page":   page,
		"per_page": perPage,
	})
}

// GuildReportRotationHandler suggests which members should do the next report
// @Summary Get report rotation suggestion
// @Description Suggest guild members for the next report based on who hasn't reported in a while
// @Tags guilds
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/report-rotation-suggestion [get]
// @Security BearerAuth
func (a *API) GuildReportRotationHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}
	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	ctx := c.Request.Context()

	members, err := a.guildAttendeeRepo.GetByGuildID(ctx, guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get members"})
		return
	}

	type memberReportInfo struct {
		UserID              uuid.UUID  `json:"user_id"`
		Name                string     `json:"name"`
		LastReportActivityID *string   `json:"last_report_activity_id"`
		LastReportAt        *time.Time `json:"last_report_at"`
	}

	var memberInfos []memberReportInfo

	for _, m := range members {
		var lastActivity struct {
			ActivityID string    `gorm:"column:activity_id"`
			CreatedAt  time.Time `gorm:"column:created_at"`
		}
		a.db.WithContext(ctx).
			Table("activity").
			Select("activity.activity_id, activity.created_at").
			Joins("JOIN event ON event.activity_id = activity.activity_id").
			Joins("JOIN event_attendee ON event_attendee.event_id = event.event_id").
			Where("activity.guild_id = ? AND activity.mode = ? AND event_attendee.user_id = ?", guildID, domain.ActivityModeReport, m.UserID).
			Where("activity.deleted_at IS NULL").
			Order("activity.created_at DESC").
			Limit(1).
			Find(&lastActivity)

		var activityID *string
		var reportAt *time.Time
		if lastActivity.ActivityID != "" {
			activityID = &lastActivity.ActivityID
			reportAt = &lastActivity.CreatedAt
		}

		user, _ := a.userRepo.GetByID(ctx, m.UserID)
		name := ""
		if user != nil {
			name = user.Name
		}

		memberInfos = append(memberInfos, memberReportInfo{
			UserID:               m.UserID,
			Name:                 name,
			LastReportActivityID: activityID,
			LastReportAt:         reportAt,
		})
	}

	// Sort by last_report_at ascending (nil = never reported, should be first)
	// Suggest top 2 members who haven't reported in the longest time (or never)
	type sortable struct {
		idx      int
		sortKey  int64
	}
	sorted := make([]sortable, len(memberInfos))
	for i, info := range memberInfos {
		sortKey := int64(0)
		if info.LastReportAt != nil {
			sortKey = info.LastReportAt.Unix()
		}
		sorted[i] = sortable{idx: i, sortKey: sortKey}
	}
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].sortKey < sorted[j-1].sortKey; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	suggestCount := 2
	if len(sorted) < suggestCount {
		suggestCount = len(sorted)
	}
	suggestions := make([]int, 0, suggestCount)
	for i := 0; i < suggestCount; i++ {
		suggestions = append(suggestions, sorted[i].idx)
	}

	suggestedMembers := make([]memberReportInfo, 0, suggestCount)
	for _, idx := range suggestions {
		suggestedMembers = append(suggestedMembers, memberInfos[idx])
	}

	c.JSON(http.StatusOK, gin.H{
		"members":    memberInfos,
		"suggested":  suggestedMembers,
	})
}
