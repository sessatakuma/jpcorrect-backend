package domain

import (
	"context"

	"github.com/google/uuid"
)

// Guild represents a guild in the jpcorrect system.
// Maps to jpcorrect.guild table.
type Guild struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"guild_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	AvatarURL   *string   `json:"avatar_url"`
	Level       int       `gorm:"default:0" json:"level"`
}

type GuildRepository interface {
	GetByID(ctx context.Context, guildID uuid.UUID) (*Guild, error)

	Create(ctx context.Context, guild *Guild) error
	Update(ctx context.Context, guild *Guild) error
	Delete(ctx context.Context, guildID uuid.UUID) error
}
