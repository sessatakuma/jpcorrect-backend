package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EventMode represents the mode of an event.
type EventMode string

const (
	EventModeReport       EventMode = "report"
	EventModeConversation EventMode = "conversation"
	EventModeReview       EventMode = "review"
)

// Event represents an event in the jpcorrect system.
// Maps to jpcorrect.event table.
type Event struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"event_id"`
	Title              string         `json:"title"`
	Description        *string        `gorm:"type:text" json:"description"`
	StartTime          time.Time      `json:"start_time"`
	ExpectedDuration   float64        `json:"expected_duration"`
	ActualDuration     *float64       `json:"actual_duration"`
	RecordLink         *string        `json:"record_link"`
	Mode               EventMode      `gorm:"default:report;uniqueIndex:idx_event_activity_mode,priority:2" json:"mode"`
	ActivityID         *uuid.UUID     `gorm:"type:uuid;uniqueIndex:idx_event_activity_mode,priority:1;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"activity_id"`
	RecordingStartedBy *uuid.UUID     `gorm:"type:uuid;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"recording_started_by"`
	RecordingStartedAt *time.Time     `json:"recording_started_at"`
	RecordingEndedAt   *time.Time     `json:"recording_ended_at"`
	Note               *string        `gorm:"type:text" json:"note"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type EventRepository interface {
	GetByID(ctx context.Context, eventID uuid.UUID) (*Event, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Event, error)

	Create(ctx context.Context, event *Event) error
	Update(ctx context.Context, event *Event) error
	Delete(ctx context.Context, eventID uuid.UUID) error
	MarkRecordingStarted(ctx context.Context, eventID, userID uuid.UUID) error
	MarkRecordingEnded(ctx context.Context, eventID uuid.UUID) error
}
