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
	EventModeDiscussion   EventMode = "discussion"
	EventModeReview       EventMode = "review"
)

// Event represents an event in the jpcorrect system.
// Maps to jpcorrect.event table.
type Event struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey" json:"event_id"`
	Title            string         `json:"title"`
	Description      *string        `gorm:"type:text" json:"description"`
	StartTime        time.Time      `json:"start_time"`
	ExpectedDuration float64        `json:"expected_duration"`
	ActualDuration   *float64       `json:"actual_duration"`
	RecordLink       *string        `json:"record_link"`
	Mode             EventMode      `gorm:"default:report" json:"mode"`
	Note             *string        `gorm:"type:text" json:"note"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type EventRepository interface {
	GetByID(ctx context.Context, eventID uuid.UUID) (*Event, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Event, error)

	Create(ctx context.Context, event *Event) error
	Update(ctx context.Context, event *Event) error
	Delete(ctx context.Context, eventID uuid.UUID) error
}
