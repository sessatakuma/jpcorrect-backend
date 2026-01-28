package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// GuildAttendeeRole represents the role of a guild attendee.
type GuildAttendeeRole string

const (
	GuildAttendeeRoleMember GuildAttendeeRole = "member"
	GuildAttendeeRoleMaster GuildAttendeeRole = "master"
)

// GuildAttendee represents an attendee of a guild.
// Maps to jpcorrect.guild_attendee table.
type GuildAttendee struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	GuildID  uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_user" json:"guild_id"`
	UserID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_user" json:"user_id"`
	Role     GuildAttendeeRole `gorm:"default:member" json:"role"`
	JoinedAt *time.Time        `json:"joined_at"`
	LeavedAt *time.Time        `json:"leaved_at"`
}

type GuildAttendeeRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*GuildAttendee, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*GuildAttendee, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*GuildAttendee, error)

	Create(ctx context.Context, attendee *GuildAttendee) error
	Update(ctx context.Context, attendee *GuildAttendee) error
	Delete(ctx context.Context, id uuid.UUID) error
}
