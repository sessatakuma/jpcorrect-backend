package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type createActivityRequest struct {
	PracticeAt      time.Time     `json:"practice_at" binding:"required"`
	Mode            string        `json:"mode"`
	Theme           *string       `json:"theme"`
	AnnounceTopicID *uuid.UUID    `json:"announce_topic_id"`
	ReviewAt        *time.Time    `json:"review_at"`
	ReporterUserIDs []uuid.UUID   `json:"reporter_user_ids"`
}

type updateActivityRequest struct {
	PracticeAt      *time.Time    `json:"practice_at"`
	ReviewAt        *time.Time    `json:"review_at"`
	Mode            *string       `json:"mode"`
	Theme           *string       `json:"theme"`
	ReporterUserIDs []uuid.UUID   `json:"reporter_user_ids"`
}

type abortActivityRequest struct {
	Reason *string `json:"reason"`
}

type activityDetailResponse struct {
	Activity *domain.Activity `json:"activity"`
	Events   []domain.Event   `json:"events"`
}

// @Summary Create an activity
// @Description Create a new activity with practice and review events for a guild
// @Tags activities
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param body body createActivityRequest true "Activity data"
// @Success 201 {object} activityDetailResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/activities [post]
// @Security BearerAuth
func (a *API) ActivityCreateHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}

	if err := a.RequireGuildLeader(c, guildID); err != nil {
		return
	}

	var req createActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mode := domain.ActivityModeReport
	if req.Mode != "" {
		mode = domain.ActivityMode(req.Mode)
	}

	reviewAt := req.PracticeAt.Add(7 * 24 * time.Hour)
	if req.ReviewAt != nil {
		reviewAt = *req.ReviewAt
	}

	var activity domain.Activity
	var events []domain.Event

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var maxSeq int
		if err := tx.Model(&domain.Activity{}).
			Where("guild_id = ? AND deleted_at IS NULL", guildID).
			Select("COALESCE(MAX(sequence_number), 0)").
			Scan(&maxSeq).Error; err != nil {
			return err
		}

		activity = domain.Activity{
			GuildID:         guildID,
			SequenceNumber:  maxSeq + 1,
			Status:          domain.ActivityStatusPendingPractice,
			Mode:            mode,
			Theme:           req.Theme,
			AnnounceTopicID: req.AnnounceTopicID,
		}

		if err := tx.Create(&activity).Error; err != nil {
			return err
		}

		eventMode := domain.EventModeReport
		if mode == domain.ActivityModeConversation {
			eventMode = domain.EventModeConversation
		}

		practiceEvent := domain.Event{
			Title:      "Practice",
			StartTime:  req.PracticeAt,
			Mode:       eventMode,
			ActivityID: &activity.ID,
		}
		if err := tx.Create(&practiceEvent).Error; err != nil {
			return err
		}

		reviewEvent := domain.Event{
			Title:      "Review",
			StartTime:  reviewAt,
			Mode:       domain.EventModeReview,
			ActivityID: &activity.ID,
		}
		if err := tx.Create(&reviewEvent).Error; err != nil {
			return err
		}

		events = []domain.Event{practiceEvent, reviewEvent}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusCreated, activityDetailResponse{
		Activity: &activity,
		Events:   events,
	})
}

// @Summary Get an activity
// @Description Get an activity by ID with its events
// @Tags activities
// @Accept json
// @Produce json
// @Param id path string true "Activity ID"
// @Success 200 {object} activityDetailResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/activities/{id} [get]
// @Security BearerAuth
func (a *API) ActivityGetHandler(c *gin.Context) {
	activityID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid activity id"})
		return
	}

	if err := a.RequireActivityMember(c, activityID); err != nil {
		return
	}

	activity, err := a.activityRepo.GetByID(c.Request.Context(), activityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "activity not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var events []domain.Event
	if err := a.db.WithContext(c.Request.Context()).
		Where("activity_id = ?", activityID).
		Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, activityDetailResponse{
		Activity: activity,
		Events:   events,
	})
}

// @Summary Update an activity
// @Description Update an activity's time, mode, theme, and corresponding events
// @Tags activities
// @Accept json
// @Produce json
// @Param id path string true "Activity ID"
// @Param body body updateActivityRequest true "Activity update data"
// @Success 200 {object} activityDetailResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/activities/{id} [put]
// @Security BearerAuth
func (a *API) ActivityUpdateHandler(c *gin.Context) {
	activityID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid activity id"})
		return
	}

	activity, err := a.activityRepo.GetByID(c.Request.Context(), activityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "activity not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.RequireGuildLeader(c, activity.GuildID); err != nil {
		return
	}

	var req updateActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Mode != nil {
		activity.Mode = domain.ActivityMode(*req.Mode)
	}
	if req.Theme != nil {
		activity.Theme = req.Theme
	}

	var events []domain.Event
	if err := a.db.WithContext(c.Request.Context()).
		Where("activity_id = ?", activityID).
		Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = a.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(activity).Error; err != nil {
			return err
		}

		for i := range events {
			if events[i].Mode == domain.EventModeReview {
				if req.ReviewAt != nil {
					events[i].StartTime = *req.ReviewAt
				} else if req.PracticeAt != nil {
					events[i].StartTime = req.PracticeAt.Add(7 * 24 * time.Hour)
				}
			} else {
				if req.PracticeAt != nil {
					events[i].StartTime = *req.PracticeAt
				}
				if req.Mode != nil {
					if *req.Mode == string(domain.ActivityModeConversation) {
						events[i].Mode = domain.EventModeConversation
					} else {
						events[i].Mode = domain.EventModeReport
					}
				}
			}
			if err := tx.Save(&events[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, activityDetailResponse{
		Activity: activity,
		Events:   events,
	})
}

// @Summary Abort an activity
// @Description Abort an activity with an optional reason
// @Tags activities
// @Accept json
// @Produce json
// @Param id path string true "Activity ID"
// @Param body body abortActivityRequest true "Abort data"
// @Success 200 {object} domain.Activity
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/activities/{id}/abort [post]
// @Security BearerAuth
func (a *API) ActivityAbortHandler(c *gin.Context) {
	activityID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid activity id"})
		return
	}

	activity, err := a.activityRepo.GetByID(c.Request.Context(), activityID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "activity not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.RequireGuildLeader(c, activity.GuildID); err != nil {
		return
	}

	var req abortActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = err
	}

	userIDRaw, _ := c.Get("userID")
	userID := userIDRaw.(uuid.UUID)
	now := time.Now()

	activity.Status = domain.ActivityStatusAborted
	activity.AbortedByUserID = &userID
	activity.AbortedAt = &now
	activity.AbortedReason = req.Reason

	if err := a.activityRepo.Update(c.Request.Context(), activity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// TODO: trigger notification
	c.JSON(http.StatusOK, activity)
}

// @Summary List guild activities
// @Description List activities for a guild with pagination and optional status filter
// @Tags activities
// @Accept json
// @Produce json
// @Param id path string true "Guild ID"
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(20)
// @Param status query string false "Filter by status"
// @Success 200 {array} domain.Activity
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/guilds/{id}/activities [get]
// @Security BearerAuth
func (a *API) GuildActivitiesHandler(c *gin.Context) {
	guildID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid guild id"})
		return
	}

	if err := a.RequireGuildMember(c, guildID); err != nil {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	statusFilter := c.Query("status")

	if statusFilter != "" {
		var activities []*domain.Activity
		offset := (page - 1) * perPage
		if err := a.db.WithContext(c.Request.Context()).
			Where("guild_id = ?", guildID).
			Where("status = ?", domain.ActivityStatus(statusFilter)).
			Order("created_at DESC").
			Offset(offset).
			Limit(perPage).
			Find(&activities).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, activities)
		return
	}

	activities, err := a.activityRepo.GetByGuildID(c.Request.Context(), guildID, page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, activities)
}
