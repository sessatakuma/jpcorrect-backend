package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get an event attendee by ID
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param id path string true "EventAttendee ID"
// @Success 200 {object} domain.EventAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees/{id} [get]
func (a *API) EventAttendeeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendee, err := a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "EventAttendee not found"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, attendee)
}

// @Summary Create an event attendee
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param attendee body domain.EventAttendee true "EventAttendee data"
// @Success 201 {object} domain.EventAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees [post]
func (a *API) EventAttendeeCreateHandler(c *gin.Context) {
	var attendee domain.EventAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	if err := a.eventAttendeeRepo.Create(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "EventAttendee already exists"))
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, attendee)
}

// @Summary Update an event attendee
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param id path string true "EventAttendee ID"
// @Param attendee body domain.EventAttendee true "EventAttendee data"
// @Success 200 {object} domain.EventAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees/{id} [put]
func (a *API) EventAttendeeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "EventAttendee not found"))
			return
		}
		writeError(c, err)
		return
	}

	var attendee domain.EventAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, err.Error()))
		return
	}

	attendee.ID = id
	if err := a.eventAttendeeRepo.Update(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			writeError(c, errorWithMessage(domain.ErrDuplicateEntry, "EventAttendee already exists"))
			return
		}
		writeError(c, err)
		return
	}

	updated, err := a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete an event attendee
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param id path string true "EventAttendee ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees/{id} [delete]
func (a *API) EventAttendeeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	_, err = a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(c, errorWithMessage(domain.ErrNotFound, "EventAttendee not found"))
			return
		}
		writeError(c, err)
		return
	}

	if err := a.eventAttendeeRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			writeError(c, errorWithMessage(domain.ErrHasRelatedRecords, "cannot delete event attendee: has related records"))
			return
		}
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get event attendees by event ID
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param event_id path string true "Event ID"
// @Success 200 {array} domain.EventAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees/event/{event_id} [get]
func (a *API) EventAttendeeGetByEventHandler(c *gin.Context) {
	eventIDStr := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendees, err := a.eventAttendeeRepo.GetByEventID(c.Request.Context(), eventID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, attendees)
}

// @Summary Get event attendees by user ID
// @Tags event-attendees
// @Accept json
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {array} domain.EventAttendee
// @Failure 400 {object} api.ErrorResponse
// @Failure 401 {object} api.ErrorResponse
// @Failure 403 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Security BearerAuth
// @Router /v1/event-attendees/user/{user_id} [get]
func (a *API) EventAttendeeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(c, errorWithMessage(domain.ErrInvalidRequest, "invalid UUID format"))
		return
	}

	attendees, err := a.eventAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, attendees)
}
