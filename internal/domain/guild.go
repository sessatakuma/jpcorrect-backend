package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GuildAttendeeRole represents the role of a guild member.
type GuildAttendeeRole string

const (
	GuildAttendeeRoleMember GuildAttendeeRole = "member"
	// Master is the guild leader with full administrative privileges over the guild
	GuildAttendeeRoleMaster GuildAttendeeRole = "master"
)

// Guild represents a guild in the jpcorrect system.
// Maps to jpcorrect.guild table.
type Guild struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"guild_id"`
	Name        string         `json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	AvatarURL   *string        `json:"avatar_url"`
	Level       int            `gorm:"default:0" json:"level"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type GuildInvite struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	GuildID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"guild_id"`
	Code      string     `gorm:"type:varchar(32);uniqueIndex;not null" json:"code"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type GuildRepository interface {
	GetByID(ctx context.Context, guildID uuid.UUID) (*Guild, error)
	CountMasterGuildsByUserID(ctx context.Context, userID uuid.UUID) (int64, error)
	CreateWithMaster(ctx context.Context, guild *Guild, attendee *GuildAttendee) error
	TransferLeader(ctx context.Context, guildID uuid.UUID, newLeaderID uuid.UUID) error

	Create(ctx context.Context, guild *Guild) error
	Update(ctx context.Context, guild *Guild) error
	Delete(ctx context.Context, guildID uuid.UUID) error

	GetActiveInviteByGuildID(ctx context.Context, guildID uuid.UUID, now time.Time) (*GuildInvite, error)
}

// GuildAttendee represents a member of a guild.
// Maps to jpcorrect.guild_attendee table.
type GuildAttendee struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey" json:"guild_attendee_id"`
	GuildID  uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_attendee_guild_user,priority:1;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"guild_id"`
	UserID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_attendee_guild_user,priority:2;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_id"`
	Role     GuildAttendeeRole `gorm:"default:member" json:"role"`
	JoinedAt *time.Time        `json:"joined_at"`
	LeftAt   *time.Time        `json:"left_at"`
}

type GuildAttendeeRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*GuildAttendee, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*GuildAttendee, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*GuildAttendee, error)

	Create(ctx context.Context, attendee *GuildAttendee) error
	Update(ctx context.Context, attendee *GuildAttendee) error
	Delete(ctx context.Context, id uuid.UUID) error
}
