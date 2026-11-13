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

// InviteLink is a guild invitation link (spec §2.2, table invite_link). It
// replaces the legacy GuildInvite model.
//
// The spec gives the fields, the unique token and the (guild_id, expires_at)
// index. It does not give nullability, defaults, foreign-key actions or the
// response JSON keys, so the following are our own decisions, not spec text:
//   - expires_at is NOT NULL: the spec says every link expires "7 days after
//     creation", so a link without an expiry does not exist. Only expires_at
//     decides whether a link is valid.
//   - created_by_user_id is NOT NULL: #72 lists the nullable foreign keys
//     explicitly and this one is not among them.
//   - Both foreign keys use ON UPDATE CASCADE / ON DELETE RESTRICT, the existing
//     GuildAttendee pattern ("follow the existing FK pattern").
//   - The JSON key for the token is `token`, not the `code` that #54 shipped.
//     The spec does not define response keys; `token` matches the spec's column
//     name and the `/v1/invites/:token` path. This breaks any client that reads
//     `code`.
type InviteLink struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	GuildID         uuid.UUID `gorm:"type:uuid;not null;index:idx_invite_link_guild_expires,priority:1" json:"guild_id"`
	Token           string    `gorm:"type:varchar(32);uniqueIndex;not null" json:"token"`
	ExpiresAt       time.Time `gorm:"not null;index:idx_invite_link_guild_expires,priority:2" json:"expires_at"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;not null" json:"created_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`

	// TTLSeconds is a request-only option and is not a column. It is kept from
	// #54 as an optional override of the default 7-day lifetime; the spec itself
	// only defines the 7 days.
	TTLSeconds *int64 `gorm:"-" json:"ttl_seconds,omitempty"`

	// Guild and CreatedByUser are belongs-to associations only, so AutoMigrate
	// emits the foreign keys (see the note on GuildAttendee.Guild: the
	// `constraint` tag must live on the association). They are never serialised,
	// and the repository writes with Omit(clause.Associations), so even a Go
	// caller that populates them cannot make a write upsert the guild or user
	// rows or overwrite GuildID / CreatedByUserID.
	Guild         *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	CreatedByUser *User  `gorm:"foreignKey:CreatedByUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

type GuildRepository interface {
	GetByID(ctx context.Context, guildID uuid.UUID) (*Guild, error)
	CreateWithMaster(ctx context.Context, guild *Guild, attendee *GuildAttendee) error
	TransferLeader(ctx context.Context, guildID uuid.UUID, callerID uuid.UUID, newLeaderID uuid.UUID) error

	Create(ctx context.Context, guild *Guild) error
	Update(ctx context.Context, guild *Guild) error
	Delete(ctx context.Context, guildID uuid.UUID) error

	GetActiveInviteByGuildID(ctx context.Context, guildID uuid.UUID, now time.Time) (*InviteLink, error)
	GetInviteByToken(ctx context.Context, token string) (*InviteLink, error)
	CreateInviteLinkWithTx(ctx context.Context, guildID uuid.UUID, newInvite *InviteLink, now time.Time) error
}

// GuildAttendee represents a member of a guild.
// Maps to jpcorrect.guild_attendee table.
type GuildAttendee struct {
	ID       uuid.UUID         `gorm:"type:uuid;primaryKey" json:"guild_attendee_id"`
	GuildID  uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_attendee_guild_user,priority:1" json:"guild_id"`
	UserID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_guild_attendee_guild_user,priority:2;uniqueIndex:idx_guild_attendee_active_master,where:role = 'master' AND left_at IS NULL" json:"user_id"`
	Role     GuildAttendeeRole `gorm:"default:member" json:"role"`
	JoinedAt *time.Time        `json:"joined_at"`
	LeftAt   *time.Time        `json:"left_at"`

	// Guild and User are belongs-to associations only: the scalar GuildID and
	// UserID columns above stay the single source of truth, and the relations
	// are what make GORM's AutoMigrate emit the foreign key constraints. The
	// `constraint` tag has to live on the association, not on the scalar
	// columns, or no constraint is created at all.
	//
	// They are never serialised, so the HTTP/Swagger contract is unchanged.
	// The attendee repositories write with Omit(clause.Associations), so even a
	// Go caller that populates them cannot make a write upsert the guild or user
	// rows or overwrite GuildID / UserID.
	Guild *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	User  *User  `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

type GuildAttendeeRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*GuildAttendee, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*GuildAttendee, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*GuildAttendee, error)
	GetByGuildAndUser(ctx context.Context, guildID uuid.UUID, userID uuid.UUID) (*GuildAttendee, error)

	Create(ctx context.Context, attendee *GuildAttendee) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, attendee *GuildAttendee) error
	Update(ctx context.Context, attendee *GuildAttendee) error
	Delete(ctx context.Context, id uuid.UUID) error
}
