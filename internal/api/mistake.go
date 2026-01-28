package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (a *API) MistakeGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	mistake, err := a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mistake not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, mistake)
}

func (a *API) MistakeCreateHandler(c *gin.Context) {
	var mistake domain.Mistake
	if err := c.ShouldBindJSON(&mistake); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.mistakeRepo.Create(c.Request.Context(), &mistake); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, mistake)
}

func (a *API) MistakeUpdateHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	// Check if record exists first
	_, err = a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mistake not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var mistake domain.Mistake
	if err := c.ShouldBindJSON(&mistake); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mistake.ID = id
	if err := a.mistakeRepo.Update(c.Request.Context(), &mistake); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return updated object
	updated, err := a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (a *API) MistakeDeleteHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	// Check if record exists first
	_, err = a.mistakeRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mistake not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.mistakeRepo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (a *API) MistakeGetByPracticeHandler(c *gin.Context) {
	// Route uses practice_id for backward compatibility, but it's actually event_id
	eventIDStr := c.Param("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	mistakes, err := a.mistakeRepo.GetByEventID(c.Request.Context(), eventID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mistakes not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, mistakes)
}

func (a *API) MistakeGetByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	mistakes, err := a.mistakeRepo.GetByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Mistakes not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, mistakes)
}
