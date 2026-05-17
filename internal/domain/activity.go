package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ActivityStatus represents the lifecycle status of an activity.
type ActivityStatus string

const (
	ActivityStatusPendingPractice ActivityStatus = "pending_practice"
	ActivityStatusInPractice      ActivityStatus = "in_practice"
	ActivityStatusAnalyzing       ActivityStatus = "analyzing"
	ActivityStatusInFeedback      ActivityStatus = "in_feedback"
	ActivityStatusInReview        ActivityStatus = "in_review"
	ActivityStatusDone            ActivityStatus = "done"
	ActivityStatusAborted         ActivityStatus = "aborted"
)

// ActivityMode represents the mode of an activity.
type ActivityMode string

const (
	ActivityModeReport       ActivityMode = "report"
	ActivityModeConversation ActivityMode = "conversation"
)

// AIStatus represents the status of AI processing for an activity.
type AIStatus string

const (
	AIStatusPending    AIStatus = "pending"
	AIStatusProcessing AIStatus = "processing"
	AIStatusCompleted  AIStatus = "completed"
	AIStatusFailed     AIStatus = "failed"
)

// YoutubeStatus represents the status of YouTube upload for an activity.
type YoutubeStatus string

const (
	YoutubeStatusPending   YoutubeStatus = "pending"
	YoutubeStatusUploading YoutubeStatus = "uploading"
	YoutubeStatusCompleted YoutubeStatus = "completed"
	YoutubeStatusFailed    YoutubeStatus = "failed"
)

// Activity represents an activity in the jpcorrect system.
// Maps to jpcorrect.activity table.
type Activity struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"activity_id"`
	GuildID         uuid.UUID      `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"guild_id"`
	SequenceNumber  int            `json:"sequence_number"`
	Status          ActivityStatus `gorm:"default:pending_practice" json:"status"`
	Mode            ActivityMode   `gorm:"default:report" json:"mode"`
	Theme           *string        `json:"theme"`
	AnnounceTopicID *uuid.UUID     `gorm:"type:uuid;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"announce_topic_id"`
	YoutubeURL      *string        `json:"youtube_url"`
	AIStatus        AIStatus       `gorm:"default:pending" json:"ai_status"`
	YoutubeStatus   YoutubeStatus  `gorm:"default:pending" json:"youtube_status"`
	AbortedByUserID *uuid.UUID     `gorm:"type:uuid;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"aborted_by_user_id"`
	AbortedAt       *time.Time     `json:"aborted_at"`
	AbortedReason   *string        `gorm:"type:text" json:"aborted_reason"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type ActivityRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Activity, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID, page, perPage int) ([]*Activity, error)
	Create(ctx context.Context, activity *Activity) error
	Update(ctx context.Context, activity *Activity) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status ActivityStatus) error
	Delete(ctx context.Context, id uuid.UUID) error
}
