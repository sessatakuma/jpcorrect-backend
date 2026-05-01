package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// @Summary Get a transcript by ID
// @Tags transcripts
// @Accept json
// @Produce json
// @Param id path string true "Transcript ID"
// @Success 200 {object} domain.Transcript
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts/{id} [get]
func (a *API) TranscriptGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	transcript, err := a.transcriptRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transcript not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transcript)
}

// @Summary Create a transcript
// @Tags transcripts
// @Accept json
// @Produce json
// @Param transcript body domain.Transcript true "Transcript data"
// @Success 201 {object} domain.Transcript
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts [post]
func (a *API) TranscriptCreateHandler(c *gin.Context) {
	var transcript domain.Transcript
	if err := c.ShouldBindJSON(&transcript); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.transcriptRepo.Create(c.Request.Context(), &transcript); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "Transcript already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, transcript)
}

// @Summary Update a transcript
// @Tags transcripts
// @Accept json
// @Produce json
// @Param id path string true "Transcript ID"
// @Param transcript body domain.Transcript true "Transcript data"
// @Success 200 {object} domain.Transcript
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts/{id} [put]
func (a *API) TranscriptUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	// Check if record exists first
	_, err = a.transcriptRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transcript not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var transcript domain.Transcript
	if err := c.ShouldBindJSON(&transcript); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	transcript.ID = id
	if err := a.transcriptRepo.Update(c.Request.Context(), &transcript); err != nil {
		if errors.Is(err, domain.ErrDuplicateEntry) {
			c.JSON(http.StatusConflict, gin.H{"error": "Transcript already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated object
	updated, err := a.transcriptRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// @Summary Delete a transcript
// @Tags transcripts
// @Accept json
// @Produce json
// @Param id path string true "Transcript ID"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts/{id} [delete]
func (a *API) TranscriptDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	// Check if record exists first
	_, err = a.transcriptRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Transcript not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.transcriptRepo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrHasRelatedRecords) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot delete transcript: has related records"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get transcripts by event ID
// @Tags transcripts
// @Accept json
// @Produce json
// @Param event_id path string true "Event ID"
// @Success 200 {array} domain.Transcript
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts/event/{event_id} [get]
func (a *API) TranscriptGetByEventHandler(c *gin.Context) {
	eventIDStr := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	transcripts, err := a.transcriptRepo.GetByEventID(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transcripts)
}

// @Summary Get transcripts by user ID
// @Tags transcripts
// @Accept json
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {array} domain.Transcript
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/transcripts/user/{user_id} [get]
func (a *API) TranscriptGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	transcripts, err := a.transcriptRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transcripts)
}
