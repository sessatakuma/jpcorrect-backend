package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
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
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"event_id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	StartTime   time.Time `json:"start_time"`
	ExpDuration float64   `json:"exp_duration"`
	ActDuration *float64  `json:"act_duration"`
	RecordLink  *string   `json:"record_link"`
	Mode        EventMode `gorm:"default:report" json:"mode"`
	Note        *string   `json:"note"`
}

type EventRepository interface {
	GetByID(ctx context.Context, eventID uuid.UUID) (*Event, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Event, error)

	Create(ctx context.Context, event *Event) error
	Update(ctx context.Context, event *Event) error
	Delete(ctx context.Context, eventID uuid.UUID) error
}
