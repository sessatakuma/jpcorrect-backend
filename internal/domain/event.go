package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// EventMode represents the mode of an event.
//
// EventModeDiscussion was removed in spec §2.1: discussion practice is Won't
// Have in Phase 0. `discussion` is therefore not a valid mode any more.
type EventMode string

const (
	EventModeReport       EventMode = "report"
	EventModeConversation EventMode = "conversation"
	EventModeReview       EventMode = "review"
)

// EventKind distinguishes the two Events one Activity pairs (spec §2.1): a
// practice Event and its review Event, with at most one of each per Activity.
type EventKind string

const (
	EventKindPractice EventKind = "practice"
	EventKindReview   EventKind = "review"
)

// Event represents an event in the jpcorrect system.
// Maps to jpcorrect.event table.
//
// The v1.2 fields come from spec §2.1. activity_id and kind are NOT NULL: an
// Event always belongs to an Activity and is always either the practice or the
// review half. The practice Event's StartTime is practice_at and the review
// Event's StartTime is review_at; no separate schedule column is added.
//
// Foreign keys use the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none. The constraint lives on the belongs-to
// association, not on the scalar column.
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

	// v1.2 additions (spec §2.1).
	ActivityID         uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_event_activity_kind,priority:1" json:"activity_id"`
	Kind               EventKind      `gorm:"not null;uniqueIndex:idx_event_activity_kind,priority:2" json:"kind"`
	CurrentStep        int            `gorm:"default:0" json:"current_step"`
	StepDurations      datatypes.JSON `gorm:"type:jsonb" json:"step_durations"`
	RecordingStartedBy *uuid.UUID     `gorm:"type:uuid" json:"recording_started_by"`
	RecordingStartedAt *time.Time     `json:"recording_started_at"`
	RecordingEndedAt   *time.Time     `json:"recording_ended_at"`

	// Associations exist only to emit the foreign keys and are never
	// serialised.
	Activity               *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	RecordingStartedByUser *User     `gorm:"foreignKey:RecordingStartedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

type EventRepository interface {
	GetByID(ctx context.Context, eventID uuid.UUID) (*Event, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Event, error)

	Create(ctx context.Context, event *Event) error
	Update(ctx context.Context, event *Event) error
	Delete(ctx context.Context, eventID uuid.UUID) error
}
