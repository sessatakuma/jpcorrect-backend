package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InviteLink represents an invitation link for joining a guild.
// Maps to jpcorrect.invite_link table.
type InviteLink struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"invite_link_id"`
	GuildID         uuid.UUID `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"guild_id"`
	Token           string    `gorm:"uniqueIndex;size:32" json:"token"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedByUserID uuid.UUID `gorm:"type:uuid;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"created_by_user_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type InviteLinkRepository interface {
	GetByToken(ctx context.Context, token string) (*InviteLink, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*InviteLink, error)
	Create(ctx context.Context, link *InviteLink) error
	Update(ctx context.Context, link *InviteLink) error
}
