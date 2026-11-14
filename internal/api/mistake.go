package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a mistake by ID
// @Tags mistakes
// @Accept json
// @Produce json
// @Param id path string true "Mistake ID"
// @Success 200 {object} domain.Mistake
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes/{id} [get]
func (a *API) MistakeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	mistake, err := a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Mistake not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, mistake)
}

// @Summary Create a mistake
// @Tags mistakes
// @Accept json
// @Produce json
// @Param mistake body domain.Mistake true "Mistake data"
// @Success 201 {object} domain.Mistake
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes [post]
func (a *API) MistakeCreateHandler(c *gin.Context) {
	var mistake domain.Mistake
	if err := c.ShouldBindJSON(&mistake); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	if err := a.mistakeRepo.Create(c.Request.Context(), &mistake); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Mistake already exists"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, mistake)
}

// @Summary Update a mistake
// @Tags mistakes
// @Accept json
// @Produce json
// @Param id path string true "Mistake ID"
// @Param mistake body domain.Mistake true "Mistake data"
// @Success 200 {object} domain.Mistake
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes/{id} [put]
func (a *API) MistakeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	// Check if record exists first
	_, err = a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Mistake not found"))
			return
		}
		writeError(c, err)
		return
	}

	var mistake domain.Mistake
	if err := c.ShouldBindJSON(&mistake); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	mistake.ID = id
	if err := a.mistakeRepo.Update(c.Request.Context(), &mistake); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "Mistake already exists"))
			return
		}
		writeError(c, err)
		return
	}

	// Return updated object
	updated, err := a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a mistake
// @Tags mistakes
// @Accept json
// @Produce json
// @Param id path string true "Mistake ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes/{id} [delete]
func (a *API) MistakeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	// Check if record exists first
	_, err = a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "Mistake not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.mistakeRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete mistake: has related records"))
			return
		}
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get mistakes by event ID
// @Tags mistakes
// @Accept json
// @Produce json
// @Param event_id path string true "Event ID"
// @Success 200 {array} domain.Mistake
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes/event/{event_id} [get]
func (a *API) MistakeGetByEventHandler(c *gin.Context) {
	eventIDStr := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	mistakes, err := a.mistakeRepo.GetByEventID(c.Request.Context(), eventID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, mistakes)
}

// @Summary Get mistakes by user ID
// @Tags mistakes
// @Accept json
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {array} domain.Mistake
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/mistakes/user/{user_id} [get]
func (a *API) MistakeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	mistakes, err := a.mistakeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, mistakes)
}
