package api

import (
	"errors"
	"net/http"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a guild by ID
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Success 200 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id} [get]
func (a *API) GuildGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
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
// @Tags guilds
// @Accept json
// @Produce json
// @Param guild body domain.Guild true "Guild data"
// @Success 201 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds [post]
func (a *API) GuildCreateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userIDStr, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id type"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id format"})
		return
	}

	// Check the number of guilds established by the caller.
	count, err := a.guildRepo.CountMasterGuildsByUserID(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count >= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user has already created a guild"})
		return
	}

	var guild domain.Guild
	if err := c.ShouldBindJSON(&guild); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attendee := domain.GuildAttendee{
		UserID: userID,
		Role:   domain.GuildAttendeeRoleMaster,
	}

	// Write Guild and GuildAttendee within the same transaction.
	if err := a.guildRepo.CreateWithMaster(c.Request.Context(), &guild, &attendee); err != nil {
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
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id} [put]
type UpdateGuildRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

func (a *API) GuildUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
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

	var req UpdateGuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		guild.Name = *req.Name
	}
	if req.Description != nil {
		guild.Description = *req.Description
	}

	if err := a.guildRepo.Update(c.Request.Context(), guild); err != nil {
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

// @Summary Transfer guild leadership
// @Description Transfer the master/leader role of a guild to another member
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param request body TransferLeaderRequest true "New leader user ID"
// @Success 200 {object} map[string]string "leader transferred successfully"
// @Failure 400 {object} map[string]string "Invalid UUID format or user is not a member"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/transfer-leader [post]
type TransferLeaderRequest struct {
	NewLeaderUserID uuid.UUID `json:"new_leader_user_id" binding:"required"`
}

func (a *API) GuildTransferLeaderHandler(c *gin.Context) {
	idStr := c.Param("id")
	guildID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	var req TransferLeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 呼叫 Repo 執行 Transaction 更新
	err = a.guildRepo.TransferLeader(c.Request.Context(), guildID, req.NewLeaderUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Guild not found"})
			return
		}
		if errors.Is(err, domain.ErrNotGuildMember) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "New leader must be a member of the guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "leader transferred successfully"})
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

type GuildInviteResponse struct {
	Code      string     `json:"code"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

func (a *API) GuildInviteLinkGetHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	invite, err := a.guildRepo.GetActiveInviteByGuildID(c.Request.Context(), guildID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invite link"})
		return
	}

	// 若已過期或無 row，invite 為 nil，Go 的 JSON 序列化會自動輸出 200 + null
	if invite == nil {
		c.JSON(http.StatusOK, nil)
		return
	}

	// 轉成 Response 格式或直接回傳 invite
	resp := GuildInviteResponse{
		Code:      invite.Code,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
	}

	c.JSON(http.StatusOK, resp)
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
