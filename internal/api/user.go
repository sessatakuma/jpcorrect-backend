package api

import (
	"errors"
	"net/http"
	"strings"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UpdateMeRequest struct {
	Name      string  `json:"nickname" binding:"required,max=50"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,url"`
}

// @Summary Get a user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/{id} [get]
func (a *API) UserGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	user, err := a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UserMeHandler returns the caller's own profile. AuthMiddleware resolved the
// internal user from the JWT `sub`, so no second lookup or ID parsing happens
// here.
// @Summary Get the current user's profile
// @Tags users
// @Produce json
// @Success 200 {object} domain.User
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/me [get]
func (a *API) UserMeHandler(c *gin.Context) {
	user := currentUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UserInitHandler creates the caller's account on first login. It is idempotent:
// the insert conflicts on supabase_user_id and does nothing, then the handler
// reads the winning row back. The internal user ID is never taken from the
// token; the database generates it and it stays stable for the account.
// @Summary Initialize the authenticated user's account
// @Description Finds or creates the local user record keyed by the Supabase identity from the JWT.
// @Description Idempotent: a retried or concurrent call returns the same account.
// @Description If another account already owns the token's email address, the request
// @Description answers 409 and no account is created or linked.
// @Tags users
// @Produce json
// @Success 200 {object} domain.User
// @Failure 400 {object} map[string]string "The token carries no email address"
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string "The email belongs to another account"
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/init [post]
func (a *API) UserInitHandler(c *gin.Context) {
	val, exists := c.Get(ContextKeySupabaseID)
	supabaseID, ok := val.(uuid.UUID)
	if !exists || !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authentication info"})
		return
	}

	email, _ := c.Get(ContextKeyEmail)
	emailStr, _ := email.(string)
	emailStr = strings.TrimSpace(emailStr)
	if emailStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the access token carries no email address"})
		return
	}

	ctx := c.Request.Context()
	candidate := domain.User{
		ID:             uuid.New(),
		Email:          emailStr,
		SupabaseUserID: &supabaseID,
	}

	if err := a.userRepo.CreateIfAbsentBySupabaseUserID(ctx, &candidate); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			// A different row already owns this email. Accounts are never linked
			// or merged from a matching email address.
			c.JSON(http.StatusConflict, gin.H{"error": "email is already in use by another account"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize user"})
		return
	}

	user, err := a.userRepo.GetBySupabaseUserID(ctx, supabaseID)
	if err != nil {
		// The insert kept no visible row: a soft-deleted account still owns this
		// subject through the unique index, so the account is not available.
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "this account is not available"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// @Summary Update a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param user body domain.User true "User data"
// @Success 200 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/{id} [put]
func (a *API) UserUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	stored, err := a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user.ID = id
	keepServerOwnedUserFields(&user, stored)

	if err := a.userRepo.Update(c.Request.Context(), &user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// keepServerOwnedUserFields copies every field a client may not set from the
// stored row into the incoming one. The raw update route binds the whole model,
// so without this a caller could link itself to any Supabase subject, promote
// itself, or change another user's email. Linking an account is a policy
// decision, and no request may make it from client input.
func keepServerOwnedUserFields(incoming, stored *domain.User) {
	incoming.Email = stored.Email
	incoming.SupabaseUserID = stored.SupabaseUserID
	incoming.PasswordHash = stored.PasswordHash
	incoming.IsEmailVerified = stored.IsEmailVerified
	incoming.Role = stored.Role
	incoming.Status = stored.Status
	incoming.LateStreak = stored.LateStreak
	incoming.Points = stored.Points
	incoming.Level = stored.Level
	incoming.CreatedAt = stored.CreatedAt
}

// UserMeUpdateHandler updates the caller's own display name and avatar.
// The allowlisted DTO ignores every other field, so a client cannot change its
// email, role, status, points or Supabase mapping through this route.
// @Summary Update the current user's profile
// @Tags users
// @Accept json
// @Produce json
// @Param body body UpdateMeRequest true "Fields to update"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/me [put]
func (a *API) UserMeUpdateHandler(c *gin.Context) {
	caller := currentUser(c)
	if caller == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var inputData UpdateMeRequest
	if err := c.ShouldBindJSON(&inputData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	// Read the row again instead of writing the middleware snapshot back, so the
	// update starts from the stored state.
	user, err := a.userRepo.GetByID(ctx, caller.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user.Name = inputData.Name
	if inputData.AvatarURL != nil {
		user.AvatarURL = inputData.AvatarURL
	}

	if err := a.userRepo.Update(ctx, user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "User nickname or data conflict"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile updated successfully"})
}

// @Summary Delete a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/{id} [delete]
func (a *API) UserDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	_, err = a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.userRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete user: has related records"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get users by name
// @Tags users
// @Accept json
// @Produce json
// @Param name path string true "User name"
// @Success 200 {array} domain.User
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/name/{name} [get]
func (a *API) UserGetByNameHandler(c *gin.Context) {
	name := c.Param("name")

	users, err := a.userRepo.GetByName(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, users)
}

// @Summary Get a user by email
// @Tags users
// @Accept json
// @Produce json
// @Param email path string true "User email"
// @Success 200 {object} domain.User
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /v1/users/email/{email} [get]
func (a *API) UserGetByEmailHandler(c *gin.Context) {
	email := c.Param("email")

	user, err := a.userRepo.GetByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}
