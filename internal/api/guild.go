package api

import (
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
// @Security BearerAuth
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
// @Param X-Debug-User-ID header string false "Debug only: caller user UUID; ignored in release" format(uuid)
// @Param guild body domain.Guild true "Guild data"
// @Success 201 {object} domain.Guild
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/guilds [post]
// @Security BearerAuth
func (a *API) GuildCreateHandler(c *gin.Context) {
	userID, ok := requireCallerID(c)
	if !ok {
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
		if errors.Is(err, domain.ErrGuildLimitReached) {
			c.JSON(http.StatusConflict, gin.H{"error": "user has already created a guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
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
// @Security BearerAuth
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
// @Failure 400 {object} map[string]string "Invalid UUID format or user is not a member"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 409 {object} map[string]string "New leader already leads another guild"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/transfer-leader [post]
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

	callerID, ok := requireCallerID(c)
	if !ok {
		return
	}

	// 呼叫 Repo 執行 Transaction 更新
	err = a.guildRepo.TransferLeader(c.Request.Context(), guildID, callerID, req.NewLeaderUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Guild not found"})
			return
		}
		if errors.Is(err, domain.ErrNotGuildMaster) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only the guild master can transfer leadership"})
			return
		}
		if errors.Is(err, domain.ErrNotGuildMember) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "New leader must be a member of the guild"})
			return
		}
		if errors.Is(err, domain.ErrGuildLimitReached) {
			c.JSON(http.StatusConflict, gin.H{"error": "new leader already leads another guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
// @Failure 400 {object} map[string]string "Invalid Guild ID format"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/invite-link [get]
func (a *API) GuildInviteLinkGetHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerID, ok := requireCallerID(c)
	if !ok {
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "user is not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify membership"})
		return
	}
	if attendee == nil || attendee.LeftAt != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user is not a member of this guild"})
		return
	}

	invite, err := a.guildRepo.GetActiveInviteByGuildID(c.Request.Context(), guildID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invite link"})
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
// @Failure 400 {object} map[string]string "Invalid Guild ID or Request Body"
// @Failure 500 {object} map[string]string "Internal Server Error"
// @Security BearerAuth
// @Router /v1/guilds/{id}/invite-link [post]
func (a *API) GuildInviteLinkCreateHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerID, ok := requireCallerID(c)
	if !ok {
		return
	}

	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "user is not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify membership"})
		return
	}
	if attendee == nil || attendee.LeftAt != nil || attendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the guild master can manage invite links"})
		return
	}

	var input domain.InviteLink
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	now := time.Now()
	const maxTTLSeconds int64 = 31536000 // 1 year
	ttl := defaultInviteTTL
	if input.TTLSeconds != nil {
		ttlSeconds := *input.TTLSeconds
		if ttlSeconds <= 0 || ttlSeconds > maxTTLSeconds {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ttl_seconds must be between 1 and 31536000"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite link"})
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
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
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
// @Security BearerAuth
// @Router /v1/guild-attendees [post]
func (a *API) GuildAttendeeCreateHandler(c *gin.Context) {
	var attendee domain.GuildAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.guildAttendeeRepo.Create(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrGuildLimitReached) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already leads another guild"})
			return
		}
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
// @Security BearerAuth
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
		if errors.Is(err, domain.ErrGuildLimitReached) {
			c.JSON(http.StatusConflict, gin.H{"error": "user already leads another guild"})
			return
		}
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
// @Security BearerAuth
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
// @Security BearerAuth
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
// @Security BearerAuth
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

	userID, ok := requireCallerID(c)
	if !ok {
		return
	}
	if userID == targetUserID {
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

	userID, ok := requireCallerID(c)
	if !ok {
		return
	}
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID)
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

	userID, ok := requireCallerID(c)
	if !ok {
		return
	}

	_, err = a.guildAttendeeRepo.GetByGuildAndUser(c.Request.Context(), guildID, userID)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "already a member"})
		return
	}

	// Check no pending application
	existing, _ := a.joinRequestRepo.GetByUserID(c.Request.Context(), userID)
	for _, req := range existing {
		if req.GuildID == guildID && req.Status == domain.JoinRequestStatusPending {
			c.JSON(http.StatusConflict, gin.H{"error": "already have a pending application"})
			return
		}
	}

	req := &domain.JoinRequest{
		GuildID: guildID,
		UserID:  userID,
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

	var req *domain.JoinRequest
	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Only a still-pending request that belongs to this guild may be approved.
		pending, err := a.joinRequestRepo.GetPendingForUpdate(c.Request.Context(), tx, guildID, appID)
		if err != nil {
			return err
		}
		req = pending

		// Create guild attendee on the same transaction.
		now := time.Now()
		attendee := &domain.GuildAttendee{
			GuildID:  guildID,
			UserID:   req.UserID,
			Role:     domain.GuildAttendeeRoleMember,
			JoinedAt: &now,
		}
		if err := a.guildAttendeeRepo.CreateWithTx(c.Request.Context(), tx, attendee); err != nil {
			return err
		}

		if err := a.joinRequestRepo.UpdateStatusWithTx(c.Request.Context(), tx, req.ID, domain.JoinRequestStatusApproved); err != nil {
			return err
		}

		// Cancel user's other pending requests on the same transaction.
		return a.joinRequestRepo.CancelPendingByUserIDWithTx(c.Request.Context(), tx, req.UserID)
	})

	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
		case errors.Is(err, domain.ErrDuplicateEntry):
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member of this guild"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve application"})
		}
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

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := a.joinRequestRepo.GetPendingForUpdate(c.Request.Context(), tx, guildID, appID); err != nil {
			return err
		}
		return a.joinRequestRepo.UpdateStatusWithTx(c.Request.Context(), tx, appID, domain.JoinRequestStatusRejected)
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject application"})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "application rejected"})

	// TODO: trigger notification
	c.JSON(http.StatusOK, gin.H{"message": "application rejected"})
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
		MemberCount    int64      `json:"member_count"`
		LastActivityAt *time.Time `json:"last_activity_at"`
	}

	var results []guildDiscoverRow
	err := a.db.WithContext(ctx).
		Table("guild").
		Select("guild.*, COUNT(DISTINCT guild_attendee.id) AS member_count, MAX(activity.created_at) AS last_activity_at").
		Joins("LEFT JOIN guild_attendee ON guild_attendee.guild_id = guild.id").
		Joins("LEFT JOIN activity ON activity.guild_id = guild.id").
		Where("guild.deleted_at IS NULL").
		Group("guild.id").
		Order("last_activity_at DESC NULLS LAST").
		Limit(perPage).
		Offset(offset).
		Find(&results).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch guilds"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"guilds":   results,
		"page":     page,
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
		UserID               uuid.UUID  `json:"user_id"`
		Name                 string     `json:"name"`
		LastReportActivityID *string    `json:"last_report_activity_id"`
		LastReportAt         *time.Time `json:"last_report_at"`
	}

	var memberInfos []memberReportInfo

	for _, m := range members {
		var lastActivity struct {
			ActivityID string    `gorm:"column:activity_id"`
			CreatedAt  time.Time `gorm:"column:created_at"`
		}
		a.db.WithContext(ctx).
			Table("activity").
			Select("activity.id AS activity_id, activity.created_at").
			Joins("JOIN event ON event.activity_id = activity.id").
			Joins("JOIN event_attendee ON event_attendee.event_id = event.id").
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
		idx     int
		sortKey int64
	}
	sorted := make([]sortable, len(memberInfos))
	for i, info := range memberInfos {
		sortKey := int64(0)
		if info.LastReportAt != nil {
			sortKey = info.LastReportAt.Unix()
		}
		sorted[i] = sortable{idx: i, sortKey: sortKey}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].sortKey < sorted[j].sortKey
	})

	suggestCount := min(2, len(sorted))

	suggestedMembers := make([]memberReportInfo, 0, suggestCount)
	for i := 0; i < suggestCount; i++ {
		suggestedMembers = append(suggestedMembers, memberInfos[sorted[i].idx])
	}

	c.JSON(http.StatusOK, gin.H{
		"members":   memberInfos,
		"suggested": suggestedMembers,
	})
}
