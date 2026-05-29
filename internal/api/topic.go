package api

import (
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TopicGetHandler returns a single topic by ID
// @Summary Get a topic by ID
// @Description Get a single topic with hint_vocab and hint_grammar
// @Tags topics
// @Produce json
// @Param id path string true "Topic ID"
// @Success 200 {object} domain.Topic
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/topics/{id} [get]
// @Security BearerAuth
func (a *API) TopicGetHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid UUID format"})
		return
	}

	topic, err := a.topicRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "topic not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topic)
}

// TopicAnnounceSearchHandler searches announce topics by keyword
// @Summary Search announce topics
// @Description Search announce topics by keyword
// @Tags topics
// @Produce json
// @Param q query string true "Search keyword"
// @Success 200 {array} domain.Topic
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/topics/announce/search [get]
// @Security BearerAuth
func (a *API) TopicAnnounceSearchHandler(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	topics, err := a.topicRepo.SearchAnnounce(c.Request.Context(), keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topics)
}

// TopicRandomHandler returns a random topic
// @Summary Get a random topic
// @Description Get a random topic
// @Tags topics
// @Produce json
// @Success 200 {object} domain.Topic
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/topics/random [get]
// @Security BearerAuth
func (a *API) TopicRandomHandler(c *gin.Context) {
	topic, err := a.topicRepo.GetRandom(c.Request.Context())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no topics available"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topic)
}

// ReportThemeSuggestionsHandler lists all report theme suggestions
// @Summary List report theme suggestions
// @Description Get all report theme suggestions
// @Tags topics
// @Produce json
// @Success 200 {array} domain.ReportThemeSuggestion
// @Failure 500 {object} map[string]string
// @Router /v1/topics/report-suggestions [get]
// @Security BearerAuth
func (a *API) ReportThemeSuggestionsHandler(c *gin.Context) {
	suggestions, err := a.reportThemeSuggestionRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, suggestions)
}
