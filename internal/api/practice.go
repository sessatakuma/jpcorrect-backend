package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a practice by ID
// @Tags practices
// @Accept json
// @Produce json
// @Param id path string true "Practice ID"
// @Success 200 {object} domain.Event
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/practices/{id} [get]
func (a *API) PracticeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	practice, err := a.eventRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Practice not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, practice)
}

// @Summary Create a practice
// @Tags practices
// @Accept json
// @Produce json
// @Param practice body domain.Event true "Practice data"
// @Success 201 {object} domain.Event
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/practices [post]
func (a *API) PracticeCreateHandler(c *gin.Context) {
	var practice domain.Event
	if err := c.ShouldBindJSON(&practice); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	if err := a.eventRepo.Create(c.Request.Context(), &practice); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Event already exists"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, practice)
}

// @Summary Update a practice
// @Tags practices
// @Accept json
// @Produce json
// @Param id path string true "Practice ID"
// @Param practice body domain.Event true "Practice data"
// @Success 200 {object} domain.Event
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/practices/{id} [put]
func (a *API) PracticeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	// Check if record exists first
	_, err = a.eventRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Practice not found"))
			return
		}
		writeError(c, err)
		return
	}

	var practice domain.Event
	if err := c.ShouldBindJSON(&practice); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	practice.ID = id
	if err := a.eventRepo.Update(c.Request.Context(), &practice); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Event already exists"))
			return
		}
		writeError(c, err)
		return
	}

	// Return updated object
	updated, err := a.eventRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a practice
// @Tags practices
// @Accept json
// @Produce json
// @Param id path string true "Practice ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/practices/{id} [delete]
func (a *API) PracticeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	// Check if record exists first
	_, err = a.eventRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Practice not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.eventRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete event: has related records"))
			return
		}
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get practices by user ID
// @Tags practices
// @Accept json
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {array} domain.Event
// @Failure 400 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/practices/user/{user_id} [get]
func (a *API) PracticeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	practices, err := a.eventRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, practices)
}
