package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EventAttendeeRole represents the role of an event attendee.
type EventAttendeeRole string

const (
	EventAttendeeRoleMember EventAttendeeRole = "member"
	EventAttendeeRoleEmcee  EventAttendeeRole = "emcee"
)

// EventAttendee represents an attendee of an event.
// Maps to jpcorrect.event_attendee table.
type EventAttendee struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	EventID  uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_event_user" json:"event_id"`
	UserID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_event_user" json:"user_id"`
	Role     EventAttendeeRole `gorm:"default:member" json:"role"`
	JoinedAt *time.Time        `json:"joined_at"`
	LeavedAt *time.Time        `json:"leaved_at"`
}

type EventAttendeeRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*EventAttendee, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*EventAttendee, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*EventAttendee, error)

	Create(ctx context.Context, attendee *EventAttendee) error
	Update(ctx context.Context, attendee *EventAttendee) error
	Delete(ctx context.Context, id uuid.UUID) error
}
