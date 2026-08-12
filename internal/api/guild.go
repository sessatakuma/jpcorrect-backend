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
		c.JSON(http.StatusConflict, gin.H{"error": "user has already created a guild"})
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

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id type"})
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id format"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "leader transferred successfully"})
}

// @Summary Get active guild invite link
// @Description Get the currently active invite link for a guild. Returns 200 with null if expired or not found.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Success 200 {object} GuildInviteResponse "Returns the active invite link, or null if expired or not found"
// @Failure 400 {object} map[string]string "Invalid Guild ID format"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/invite-link [get]
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

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id type"})
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id format"})
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
	if attendee == nil {
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
		Code:      invite.Code,
		ExpiresAt: invite.ExpiresAt,
		CreatedAt: invite.CreatedAt,
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Create a new guild invite link
// @Description Creates a new active invite link for a guild and expires any existing active links within a single transaction.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Param request body domain.GuildInvite false "Invite Link Options (e.g. {"ttl_seconds": 86400})"
// @Success 200 {object} domain.GuildInvite
// @Failure 400 {object} map[string]string "Invalid Guild ID or Request Body"
// @Failure 500 {object} map[string]string "Internal Server Error"
// @Router /v1/guilds/{id}/invite-link [post]
func (a *API) GuildInviteLinkCreateHandler(c *gin.Context) {
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerIDStr, ok := callerIDVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user id type"})
		return
	}
	callerID, err := uuid.Parse(callerIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id format"})
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
	if attendee == nil || attendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the guild master can manage invite links"})
		return
	}

	var input domain.GuildInvite
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	now := time.Now()
	const maxTTLSeconds int64 = 31536000 // 1 year
	var expiresAt *time.Time
	if input.TTLSeconds != nil {
		ttl := *input.TTLSeconds
		if ttl <= 0 || ttl > maxTTLSeconds {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ttl_seconds must be between 1 and 31536000"})
			return
		}
		t := now.Add(time.Duration(ttl) * time.Second)
		expiresAt = &t
	}

	// Generate a new invitation link
	newInvite := &domain.GuildInvite{
		ID:        uuid.New(),
		GuildID:   guildID,
		Code:      strings.ReplaceAll(uuid.New().String(), "-", ""),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}

	// In the Transaction: expire old links + create new link
	if err := a.guildRepo.CreateInviteLinkWithTx(c.Request.Context(), guildID, newInvite, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite link"})
		return
	}

	c.JSON(http.StatusOK, newInvite)
}

type GuildApplicationResponse struct {
	ID        uuid.UUID                     `json:"id"`
	GuildID   uuid.UUID                     `json:"guild_id"`
	UserID    uuid.UUID                     `json:"user_id"`
	Status    domain.GuildApplicationStatus `json:"status"`
	CreatedAt time.Time                     `json:"created_at"`
	UpdatedAt time.Time                     `json:"updated_at"`
}

// @Summary Submit guild application
// @Description Submit an application to join a guild. Fails if caller is already a member or has a pending application.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Success 201 {object} GuildApplicationResponse "Application created successfully"
// @Failure 400 {object} map[string]string "Invalid Guild ID, already a member, or pending application exists"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/applications [post]
func (a *API) GuildApplicationCreateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}

	// Check if the guild exists
	_, err = a.guildRepo.GetByID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		return
	}

	// Check if the caller already belongs to the guild.
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check attendee status"})
		return
	}
	if attendee != nil && attendee.LeftAt == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you are already a member of this guild"})
		return
	}

	// Check if already application with a "Pending" status exists.
	pendingApp, err := a.applicationRepo.GetPendingByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check existing applications"})
		return
	}
	if pendingApp != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you already have a pending application for this guild"})
		return
	}

	now := time.Now()
	newApp := &domain.GuildApplication{
		ID:        uuid.New(),
		GuildID:   guildID,
		UserID:    callerID,
		Status:    domain.GuildApplicationStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := a.applicationRepo.Create(ctx, newApp); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "you already have a pending application for this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create application"})
		return
	}

	resp := GuildApplicationResponse{
		ID:        newApp.ID,
		GuildID:   newApp.GuildID,
		UserID:    newApp.UserID,
		Status:    newApp.Status,
		CreatedAt: newApp.CreatedAt,
		UpdatedAt: newApp.UpdatedAt,
	}

	c.JSON(http.StatusCreated, resp)
}

// @Summary List pending guild applications
// @Description Get all pending join applications for a specific guild. Caller must be the guild master or a member.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Success 200 {object} ListGuildApplicationsResponse "List of pending applications"
// @Failure 400 {object} map[string]string "Invalid Guild ID format"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - Caller is not a member or master of the guild"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/applications [get]
type ListGuildApplicationsResponse struct {
	Applications []*domain.GuildApplication `json:"applications"`
}

func (a *API) GuildApplicationsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	guildIDParam := c.Param("id")
	guildID, err := uuid.Parse(guildIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}

	// Check if the guild exists
	_, err = a.guildRepo.GetByID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		return
	}

	// Check if the caller is a member of the guild
	attendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you are not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check member permission"})
		return
	}
	if attendee.LeftAt != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you have left this guild"})
		return
	}

	// Query all pending applications.
	apps, err := a.applicationRepo.ListPendingByGuildID(ctx, guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch applications"})
		return
	}
	if apps == nil {
		apps = []*domain.GuildApplication{}
	}

	c.JSON(http.StatusOK, ListGuildApplicationsResponse{
		Applications: apps,
	})
}

// @Summary Approve guild application
// @Description Approve a pending application to join the guild. Adds the user as a member and cancels their pending applications for other guilds in a single transaction. Only guild master can perform this.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Param app_id path string true "Application ID" format(uuid)
// @Success 200 {object} map[string]string "Application approved successfully"
// @Failure 400 {object} map[string]string "Invalid ID format or application not in pending status"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - Only guild master can approve"
// @Failure 404 {object} map[string]string "Guild or Application not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/applications/{app_id}/approve [post]
func (a *API) GuildApplicationApproveHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	appID, err := uuid.Parse(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}

	// Check if the caller is the master of the guild
	callerAttendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you are not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify caller permission"})
		return
	}

	if callerAttendee.LeftAt != nil || callerAttendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only guild master can approve applications"})
		return
	}

	// Query the application and verify its status
	app, err := a.applicationRepo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query application"})
		return
	}

	if app.GuildID != guildID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application does not belong to this guild"})
		return
	}
	if app.Status != domain.GuildApplicationStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application is not in pending status"})
		return
	}

	newAttendee := &domain.GuildAttendee{
		GuildID: guildID,
		UserID:  app.UserID,
		Role:    domain.GuildAttendeeRoleMember,
	}

	if err := a.applicationRepo.ApproveWithTx(ctx, app, newAttendee); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "application approved successfully",
	})
}

// @Summary Reject guild application
// @Description Reject a pending application to join the guild. Only guild master can perform this.
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID" format(uuid)
// @Param app_id path string true "Application ID" format(uuid)
// @Success 200 {object} map[string]string "Application rejected successfully"
// @Failure 400 {object} map[string]string "Invalid ID format or application not in pending status"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden - Only guild master can reject"
// @Failure 404 {object} map[string]string "Guild or Application not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/applications/{app_id}/reject [post]
func (a *API) GuildApplicationRejectHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}
	appID, err := uuid.Parse(c.Param("app_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid application_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}

	// Check if the caller is the master of the guild
	callerAttendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you are not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify caller permission"})
		return
	}

	if callerAttendee.LeftAt != nil || callerAttendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only guild master can reject applications"})
		return
	}

	// Query the application and verify its status
	app, err := a.applicationRepo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query application"})
		return
	}

	if app.GuildID != guildID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application does not belong to this guild"})
		return
	}
	if app.Status != domain.GuildApplicationStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "application is not in pending status"})
		return
	}

	// Update status to "rejected"
	app.Status = domain.GuildApplicationStatusRejected
	if err := a.applicationRepo.Update(ctx, app); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject application"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "application rejected successfully",
	})
}

// GuildDefaultSlotGetHandler retrieves the default time slot (may be null)
// @Summary Get guild default slot
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID (UUID)" format(uuid)
// @Success 200 {object} map[string]interface{} "Success (data field contains GuildDefaultSlot or null)"
// @Failure 400 {object} map[string]string "Invalid guild_id format"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/default-slot [get]
func (a *API) GuildDefaultSlotGetHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	_, err = a.guildRepo.GetByID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		return
	}

	// Query the Guild's preset time slots.
	slot, err := a.defaultSlotRepo.GetByGuildID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusOK, gin.H{
				"data": nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query default slot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": slot,
	})
}

// GuildDefaultSlotUpsertHandler Set or update preset time slots
// @Summary Upsert guild default slot
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID (UUID)" format(uuid)
// @Param request body domain.GuildDefaultSlotUpsertReq true "Upsert Request Body"
// @Success 200 {object} map[string]interface{} "Success"
// @Failure 400 {object} map[string]string "Invalid request body or guild_id"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden: only master can manage default slot"
// @Failure 404 {object} map[string]string "Guild not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/default-slot [put]
type GuildDefaultSlotUpsertReq struct {
	DayOfWeek int    `json:"day_of_week" binding:"gte=0,lte=6"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

func (a *API) GuildDefaultSlotUpsertHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	var req GuildDefaultSlotUpsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Check the caller's identity
	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}

	// Check if the caller is the member of the guild
	callerAttendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you are not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify caller permission"})
		return
	}
	// Check if the caller is the master of the guild
	if callerAttendee.LeftAt != nil || callerAttendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only guild master can manage default slot"})
		return
	}

	// Check if the guild exists
	_, err = a.guildRepo.GetByID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		return
	}

	slot := &domain.GuildDefaultSlot{
		ID:        uuid.New(),
		GuildID:   guildID,
		DayOfWeek: req.DayOfWeek,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}

	if err := a.defaultSlotRepo.Upsert(ctx, slot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert default slot"})
		return
	}

	// Requery the latest slot records and return
	latestSlot, err := a.defaultSlotRepo.GetByGuildID(ctx, guildID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated default slot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": latestSlot,
	})
}

// GuildDefaultSlotDeleteHandler deletes the default time slot
// @Summary Delete guild default slot
// @Tags guilds
// @Accept json
// @Produce json
// @Param id path string true "Guild ID (UUID)" format(uuid)
// @Success 200 {object} map[string]string "Success"
// @Failure 400 {object} map[string]string "Invalid guild_id"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Forbidden: only master can delete default slot"
// @Failure 404 {object} map[string]string "Guild or default slot not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /v1/guilds/{id}/default-slot [delete]
func (a *API) GuildDefaultSlotDeleteHandler(c *gin.Context) {
	ctx := c.Request.Context()
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild_id"})
		return
	}

	callerIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	callerID, ok := callerIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user ID type in context"})
		return
	}
	// Check if the caller is the member of the guild
	callerAttendee, err := a.guildAttendeeRepo.GetByGuildAndUser(ctx, guildID, callerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: you are not a member of this guild"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify caller permission"})
		return
	}
	// Check if the caller is the master of the guild
	if callerAttendee.LeftAt != nil || callerAttendee.Role != domain.GuildAttendeeRoleMaster {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: only guild master can delete default slot"})
		return
	}

	// Check if the guild exists
	_, err = a.guildRepo.GetByID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guild not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query guild"})
		return
	}

	// Perform deletion
	err = a.defaultSlotRepo.DeleteByGuildID(ctx, guildID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "default slot not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete default slot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "default slot deleted successfully",
	})
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
