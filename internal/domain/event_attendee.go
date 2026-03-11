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
	// Emcee is the event host who manages the practice session flow
	EventAttendeeRoleEmcee EventAttendeeRole = "emcee"
)

// EventAttendee represents an attendee of an event.
// Maps to jpcorrect.event_attendee table.
type EventAttendee struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey" json:"event_attendee_id"`
	EventID  uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_event_attendee_event_user,priority:1;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"event_id"`
	UserID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_event_attendee_event_user,priority:2;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_id"`
	Role     EventAttendeeRole `gorm:"default:member" json:"role"`
	JoinedAt *time.Time        `json:"joined_at"`
	LeftAt   *time.Time        `json:"left_at"`
}

type EventAttendeeRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*EventAttendee, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*EventAttendee, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*EventAttendee, error)

	Create(ctx context.Context, attendee *EventAttendee) error
	Update(ctx context.Context, attendee *EventAttendee) error
	Delete(ctx context.Context, id uuid.UUID) error
}
