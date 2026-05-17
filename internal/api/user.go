package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
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

// @Summary Create a user
// @Tags users
// @Accept json
// @Produce json
// @Param user body domain.User true "User data"
// @Success 201 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/users [post]
func (a *API) UserCreateHandler(c *gin.Context) {
	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.userRepo.Create(c.Request.Context(), &user); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/users/{id} [put]
func (a *API) UserUpdateHandler(c *gin.Context) {
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

	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user.ID = id
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

// UserInitHandler returns the current authenticated user.
// Since the auth middleware already resolves the user from JWT claims,
// this endpoint is idempotent — it simply returns the user from context.
//
// @Summary Initialize current user
// @Description Return the authenticated user based on JWT supabase_user_id (idempotent)
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} domain.User
// @Failure 401 {object} map[string]string
// @Router /v1/users/init [post]
// @Security BearerAuth
func (a *API) UserInitHandler(c *gin.Context) {
	user := c.MustGet("user").(*domain.User)
	c.JSON(http.StatusOK, user)
}

// UserMeHandler returns the current authenticated user's profile.
//
// @Summary Get current user profile
// @Description Get the authenticated user's profile
// @Tags users
// @Produce json
// @Success 200 {object} domain.User
// @Failure 401 {object} map[string]string
// @Router /v1/users/me [get]
// @Security BearerAuth
func (a *API) UserMeHandler(c *gin.Context) {
	user := c.MustGet("user").(*domain.User)
	c.JSON(http.StatusOK, user)
}

// UserMeUpdateHandler updates the current authenticated user's profile.
// Only Name, AvatarURL, and Timezone are updatable.
//
// @Summary Update current user profile
// @Description Update the authenticated user's profile (name, avatar_url, timezone)
// @Tags users
// @Accept json
// @Produce json
// @Param body body object true "Fields to update" example({"name":"John","avatar_url":"https://example.com/avatar.png","timezone":"Asia/Tokyo"})
// @Success 200 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/users/me [put]
// @Security BearerAuth
func (a *API) UserMeUpdateHandler(c *gin.Context) {
	user := c.MustGet("user").(*domain.User)

	var input struct {
		Name      *string `json:"name"`
		AvatarURL *string `json:"avatar_url"`
		Timezone  *string `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	if input.Name != nil {
		user.Name = *input.Name
	}
	if input.AvatarURL != nil {
		user.AvatarURL = input.AvatarURL
	}
	if input.Timezone != nil {
		user.Timezone = *input.Timezone
	}

	if err := a.userRepo.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// @Summary Get a user by email
// @Tags users
// @Accept json
// @Produce json
// @Param email path string true "User email"
// @Success 200 {object} domain.User
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
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
