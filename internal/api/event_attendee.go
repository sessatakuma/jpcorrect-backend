package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (a *API) EventAttendeeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendee, err := a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "EventAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendee)
}

func (a *API) EventAttendeeCreateHandler(c *gin.Context) {
	var attendee domain.EventAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.eventAttendeeRepo.Create(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "EventAttendee already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, attendee)
}

func (a *API) EventAttendeeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	_, err = a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "EventAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var attendee domain.EventAttendee
	if err := c.ShouldBindJSON(&attendee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	attendee.ID = id
	if err := a.eventAttendeeRepo.Update(c.Request.Context(), &attendee); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "EventAttendee already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, err := a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (a *API) EventAttendeeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	_, err = a.eventAttendeeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "EventAttendee not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.eventAttendeeRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete event attendee: has related records"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (a *API) EventAttendeeGetByEventHandler(c *gin.Context) {
	eventIDStr := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendees, err := a.eventAttendeeRepo.GetByEventID(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendees)
}

func (a *API) EventAttendeeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	attendees, err := a.eventAttendeeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, attendees)
}
