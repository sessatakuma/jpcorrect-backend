package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guilds/{id} [get]
func (a *API) GuildGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	guild, err := a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Guild not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, guild)
}

// @Summary Create a guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param X-Debug-User-ID header string false "Debug only: caller user UUID; ignored in release" format(uuid)
// @Param guild body domain.Guild true "Guild data"
// @Success 201 {object} domain.Guild
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guilds [post]
func (a *API) GuildCreateHandler(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		writeError(c, domain.ErrUnauthorized)
		return
	}
	userIDStr, ok := userIDVal.(string)
	if !ok {
		writeError(c, domain.ErrInternal)
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid user id format"))
		return
	}

	var guild domain.Guild
	if err := c.ShouldBindJSON(&guild); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	attendee := domain.GuildAttendee{
		UserID: userID,
		Role:   domain.GuildAttendeeRoleMaster,
	}

	// Write Guild and GuildAttendee within the same transaction.
	if err := a.guildRepo.CreateWithMaster(c.Request.Context(), &guild, &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Guild already exists"))
			return
		}
		if errors.Is(err, domain.ErrGuildLimitReached) {
			writeError(c, errorWithMessage(domain.ErrGuildLimitReached, "user has already created a guild"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, guild)
}

type UpdateGuildRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
}

// @Summary Update a guild
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param guild body UpdateGuildRequest true "Guild data"
// @Success 200 {object} domain.Guild
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guilds/{id} [put]
func (a *API) GuildUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	guild, err := a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Guild not found"))
			return
		}
		writeError(c, err)
		return
	}

	var req UpdateGuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
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
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Guild already exists"))
			return
		}
		writeError(c, err)
		return
	}

	updated, err := a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guilds/{id} [delete]
func (a *API) GuildDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.guildRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Guild not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.guildRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete guild: has related records"))
			return
		}
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

type TransferLeaderRequest struct {
	NewLeaderUserID uuid.UUID `json:"new_leader_user_id" binding:"required"`
}

// @Summary Transfer guild leadership
// @Description Transfer the master/leader role of a guild to another member
// @Tags guilds
// @Accept json
// @Produce json
// @Param X-Debug-User-ID header string false "Debug only: caller user UUID; ignored in release" format(uuid)
// @Param id path string true "Guild ID"
// @Param request body TransferLeaderRequest true "New leader user ID"
// @Success 200 {object} map[string]string "leader transferred successfully"
// @Failure 400 {object} api.ErrorResponse "Invalid UUID format or request body"
// @Failure 403 {object} api.ErrorResponse "Only the guild master can transfer leadership"
// @Failure 404 {object} api.ErrorResponse "Guild not found, or the new leader is not an active member"
// @Failure 409 {object} api.ErrorResponse "New leader already leads another guild"
// @Failure 500 {object} api.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/transfer-leader [post]
func (a *API) GuildTransferLeaderHandler(c *gin.Context) {
	idStr := c.Param("id")
	guildID, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	var req TransferLeaderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		writeError(c, domain.ErrUnauthorized)
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		writeError(c, domain.ErrInternal)
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid user id format"))
		return
	}

	// 呼叫 Repo 執行 Transaction 更新
	err = a.guildRepo.TransferLeader(c.Request.Context(), guildID, callerID, req.NewLeaderUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Guild not found"))
			return
		}
		if errors.Is(err, domain.ErrNotGuildMaster) {
			writeError(c, errorWithMessage(domain.ErrNotGuildMaster, "only the guild master can transfer leadership"))
			return
		}
		// Spec §3.3: a target that is not an active member answers 404 ErrNotFound.
		if errors.Is(err, domain.ErrNotGuildMember) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "new leader must be an active member of the guild"))
			return
		}
		if errors.Is(err, domain.ErrGuildLimitReached) {
			writeError(c, errorWithMessage(domain.ErrGuildLimitReached, "new leader already leads another guild"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "leader transferred successfully"})
}

// defaultInviteTTL is the lifetime of a new invite link when the caller does
// not ask for another one: the spec's "7 days after creation" (§2.2).
const defaultInviteTTL = 7 * 24 * time.Hour

// GuildInviteResponse is the invite link as returned by the GET endpoint. The
// JSON key `token` (formerly `code`) is our own choice; see domain.InviteLink.
type GuildInviteResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// @Summary Get active guild invite link
// @Description Get the currently active invite link for a guild. Returns 200 with null if expired or not found.
// @Tags guilds
// @Accept json
// @Produce json
// @Param X-Debug-User-ID header string false "Debug only: caller user UUID; ignored in release" format(uuid)
// @Param id path string true "Guild ID" format(uuid)
// @Success 200 {object} GuildInviteResponse "Returns the active invite link, or null if expired or not found"
// @Failure 400 {object} api.ErrorResponse "Invalid Guild ID format"
// @Failure 401 {object} api.ErrorResponse "Missing or invalid credentials"
// @Failure 403 {object} api.ErrorResponse "Caller is not a member of this guild"
// @Failure 500 {object} api.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/invite-link [get]
func (a *API) GuildInviteLinkGetHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid guild_id"))
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		writeError(c, domain.ErrUnauthorized)
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		writeError(c, domain.ErrInternal)
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid user id format"))
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotGuildMember, "user is not a member of this guild"))
			return
		}
		writeError(c, err)
		return
	}
	if attendee == nil || attendee.LeftAt != nil {
		writeError(c, errorWithMessage(domain.ErrNotGuildMember, "user is not a member of this guild"))
		return
	}

	invite, err := a.guildRepo.GetActiveInviteByGuildID(c.Request.Context(), guildID, time.Now())
	if err != nil {
		writeError(c, err)
		return
	}

	if invite == nil {
		c.JSON(http.StatusOK, nil)
		return
	}

	// Convert to Response format or return the invite directly.
	resp := GuildInviteResponse{
		Token:     invite.Token,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Create a new guild invite link
// @Description Creates a new active invite link for a guild and expires any existing active links within a single transaction. The link expires 7 days after creation unless `ttl_seconds` asks for another lifetime.
// @Tags guilds
// @Accept json
// @Produce json
// @Param X-Debug-User-ID header string false "Debug only: caller user UUID; ignored in release" format(uuid)
// @Param id path string true "Guild ID" format(uuid)
// @Param request body domain.InviteLink false "Invite Link Options (e.g. {"ttl_seconds": 86400})"
// @Success 200 {object} domain.InviteLink
// @Failure 400 {object} api.ErrorResponse "Invalid Guild ID or Request Body"
// @Failure 401 {object} api.ErrorResponse "Missing or invalid credentials"
// @Failure 403 {object} api.ErrorResponse "Only the guild master can manage invite links"
// @Failure 500 {object} api.ErrorResponse "Internal Server Error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/invite-link [post]
func (a *API) GuildInviteLinkCreateHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid guild_id"))
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		writeError(c, domain.ErrUnauthorized)
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		writeError(c, domain.ErrInternal)
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid user id format"))
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotGuildMember, "user is not a member of this guild"))
			return
		}
		writeError(c, err)
		return
	}
	if attendee == nil || attendee.LeftAt != nil || attendee.Role != domain.GuildAttendeeRoleMaster {
		writeError(c, errorWithMessage(domain.ErrNotGuildMaster, "only the guild master can manage invite links"))
		return
	}

	var input domain.InviteLink
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid request body"))
		return
	}

	now := time.Now()
	const maxTTLSeconds int64 = 31536000 // 1 year
	ttl := defaultInviteTTL
	if input.TTLSeconds != nil {
		ttlSeconds := *input.TTLSeconds
		if ttlSeconds <= 0 || ttlSeconds > maxTTLSeconds {
			writeError(c, errorWithMessage(domain.ErrInvalidRequest, "ttl_seconds must be between 1 and 31536000"))
			return
		}
		ttl = time.Duration(ttlSeconds) * time.Second
	}

	// Generate a new invitation link
	newInvite := &domain.InviteLink{
		ID:              uuid.New(),
		GuildID:         guildID,
		Token:           strings.ReplaceAll(uuid.New().String(), "-", ""),
		ExpiresAt:       now.Add(ttl),
		CreatedByUserID: callerID,
		CreatedAt:       now,
	}

	// In the Transaction: expire old links + create new link
	if err := a.guildRepo.CreateInviteLinkWithTx(c.Request.Context(), guildID, newInvite, now); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, newInvite)
}

// @Summary Get a guild attendee by ID
// @Tags guild-attendees
// @Accept json
// @Produce json
// @Param id path string true "GuildAttendee ID"
// @Success 200 {object} domain.GuildAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees/{id} [get]
func (a *API) GuildAttendeeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "GuildAttendee not found"))
			return
		}
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees [post]
func (a *API) GuildAttendeeCreateHandler(c *gin.Context) {
	var attendee domain.GuildAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	if err := a.guildAttendeeRepo.Create(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrGuildLimitReached) {
			writeError(c, errorWithMessage(domain.ErrGuildLimitReached, "user already leads another guild"))
			return
		}
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "GuildAttendee already exists"))
			return
		}
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees/{id} [put]
func (a *API) GuildAttendeeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "GuildAttendee not found"))
			return
		}
		writeError(c, err)
		return
	}

	var attendee domain.GuildAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	attendee.ID = id
	if err := a.guildAttendeeRepo.Update(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrGuildLimitReached) {
			writeError(c, errorWithMessage(domain.ErrGuildLimitReached, "user already leads another guild"))
			return
		}
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "GuildAttendee already exists"))
			return
		}
		writeError(c, err)
		return
	}

	updated, err := a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees/{id} [delete]
func (a *API) GuildAttendeeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.guildAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "GuildAttendee not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.guildAttendeeRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete guild attendee: has related records"))
			return
		}
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees/guild/{guild_id} [get]
func (a *API) GuildAttendeeGetByGuildHandler(c *gin.Context) {
	guildIDStr := c.Param("guild_id")
	guildID, err := uuid.Parse(guildIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendees, err := a.guildAttendeeRepo.GetByGuildID(c.Request.Context(), guildID)
	if err != nil {
		writeError(c, err)
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
// @Failure 400 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/guild-attendees/user/{user_id} [get]
func (a *API) GuildAttendeeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendees, err := a.guildAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, attendees)
}
