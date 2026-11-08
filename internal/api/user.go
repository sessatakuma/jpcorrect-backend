package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a user by ID
// @Description Public profile of another user. Only the allowlisted
// @Description PublicUserProfile fields are returned: email, status, role,
// @Description is_email_verified, supabase_user_id, password_hash, points and
// @Description deleted_at never leave the server.
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} api.PublicUserProfile
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/{id} [get]
func (a *API) UserGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	user, err := a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "User not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, newPublicUserProfile(user))
}

// PublicUserProfile is the allowlisted view of another user (spec §3.1.2). The
// set is intentionally minimal: identity, display name, avatar and level are
// what a guild or activity page needs, and everything else on domain.User is
// either private or internal bookkeeping.
type PublicUserProfile struct {
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	AvatarURL *string   `json:"avatar_url"`
	Level     int       `json:"level"`
	CreatedAt time.Time `json:"created_at"`
}

// newPublicUserProfile projects the allowlisted fields out of a stored user.
// Add a field here only after checking that it is safe for other members to
// read.
func newPublicUserProfile(user *domain.User) PublicUserProfile {
	return PublicUserProfile{
		UserID:    user.ID,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		Level:     user.Level,
		CreatedAt: user.CreatedAt,
	}
}

// @Summary Create a user
// @Tags users
// @Accept json
// @Produce json
// @Param user body domain.User true "User data"
// @Success 201 {object} domain.User
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users [post]
func (a *API) UserCreateHandler(c *gin.Context) {
	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	if err := a.userRepo.Create(c.Request.Context(), &user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "User already exists"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, user)
}

// @Summary Update a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param user body domain.User true "User data"
// @Success 200 {object} domain.User
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/{id} [put]
func (a *API) UserUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "User not found"))
			return
		}
		writeError(c, err)
		return
	}

	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	user.ID = id
	if err := a.userRepo.Update(c.Request.Context(), &user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "User already exists"))
			return
		}
		writeError(c, err)
		return
	}

	updated, err := a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/{id} [delete]
func (a *API) UserDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.userRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "User not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.userRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete user: has related records"))
			return
		}
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get users by name
// @Description Public profiles of every user with this display name.
// @Tags users
// @Accept json
// @Produce json
// @Param name path string true "User name"
// @Success 200 {array} api.PublicUserProfile
// @Failure 401 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/name/{name} [get]
func (a *API) UserGetByNameHandler(c *gin.Context) {
	name := c.Param("name")

	users, err := a.userRepo.GetByName(c.Request.Context(), name)
	if err != nil {
		writeError(c, err)
		return
	}

	profiles := make([]PublicUserProfile, 0, len(users))
	for _, user := range users {
		profiles = append(profiles, newPublicUserProfile(user))
	}

	c.JSON(http.StatusOK, profiles)
}

// @Summary Initialize the caller's account
// @Description First-login endpoint (spec §3.1.1, §6.3). The caller authenticates
// @Description with a Supabase JWT; the middleware skips the user lookup for this
// @Description path only and the handler creates the row from the token's sub and
// @Description email. Idempotent: a retried or concurrent call returns the same
// @Description profile. If another account already owns the email, the request
// @Description answers 409 and no row is created or linked.
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} domain.User "The caller's profile, created or already existing"
// @Failure 400 {object} api.ErrorResponse "The token carries no email address"
// @Failure 401 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse "The email belongs to another account"
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/init [post]
func (a *API) UserInitHandler(c *gin.Context) {
	auth := currentSupabaseAuth(c)
	if auth == nil || auth.Subject == "" {
		writeError(c, domain.ErrUnauthorized)
		return
	}

	email := strings.TrimSpace(auth.Email)
	if email == "" {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "the access token carries no email address"))
		return
	}

	ctx := c.Request.Context()
	sub := auth.Subject
	candidate := domain.User{
		ID:             uuid.New(),
		Email:          email,
		SupabaseUserID: &sub,
	}

	if err := a.userRepo.CreateIfAbsentBySupabaseUserID(ctx, &candidate); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			// A different row already holds this email. Accounts are never
			// linked or merged automatically (issue #73 engineering default).
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "email is already in use by another account"))
			return
		}
		writeError(c, err)
		return
	}

	user, err := a.userRepo.GetBySupabaseUserID(ctx, sub)
	if err != nil {
		// The idempotent insert left no visible row: a soft-deleted account with
		// this sub blocks the insert and is not usable.
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrUserNotInitialized, "this account is not available"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}

// @Summary Get the caller's own profile
// @Description Returns the authenticated caller's full profile, resolved from the
// @Description JWT.
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} domain.User
// @Failure 401 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/me [get]
func (a *API) UserMeHandler(c *gin.Context) {
	user := currentUser(c)
	if user == nil {
		writeError(c, domain.ErrUnauthorized)
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateMeRequest is the allowlisted body of PUT /v1/users/me: the caller may
// change the display name and the avatar URL and nothing else. Email, role,
// status, points and supabase_user_id are server-owned, so a client that sends
// them anyway is simply ignored (issue #73 clarification).
type UpdateMeRequest struct {
	Name      *string `json:"name" binding:"omitempty,min=1,max=100"`
	AvatarURL *string `json:"avatar_url" binding:"omitempty,max=2048"`
}

// @Summary Update the caller's own profile
// @Description Updates the allowlisted fields of the caller's own profile: name
// @Description and avatar_url. Any other field in the body, including role,
// @Description status and points, is ignored.
// @Tags users
// @Accept json
// @Produce json
// @Param user body UpdateMeRequest true "Fields to update"
// @Success 200 {object} domain.User
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/me [put]
func (a *API) UserMeUpdateHandler(c *gin.Context) {
	caller := currentUser(c)
	if caller == nil {
		writeError(c, domain.ErrUnauthorized)
		return
	}

	var req UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	ctx := c.Request.Context()
	// Read the row again instead of trusting the middleware snapshot, so the
	// write does not carry values resolved earlier in the request.
	user, err := a.userRepo.GetByID(ctx, caller.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "User not found"))
			return
		}
		writeError(c, err)
		return
	}

	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}

	if err := a.userRepo.Update(ctx, user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "User already exists"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}
