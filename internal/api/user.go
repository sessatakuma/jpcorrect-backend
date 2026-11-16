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
// @Failure 400 {object} api.ErrorResponse
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

	c.JSON(http.StatusOK, user)
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
// @Tags users
// @Accept json
// @Produce json
// @Param name path string true "User name"
// @Success 200 {array} domain.User
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

	c.JSON(http.StatusOK, users)
}

// @Summary Get a user by email
// @Tags users
// @Accept json
// @Produce json
// @Param email path string true "User email"
// @Success 200 {object} domain.User
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/users/email/{email} [get]
func (a *API) UserGetByEmailHandler(c *gin.Context) {
	email := c.Param("email")

	user, err := a.userRepo.GetByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "User not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, user)
}
