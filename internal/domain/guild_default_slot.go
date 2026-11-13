package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// GuildDefaultSlot represents the default scheduling slot for a guild.
// Maps to jpcorrect.guild_default_slot table.
// 1:1 relationship with Guild (PK is guild_id).
type GuildDefaultSlot struct {
	GuildID   uuid.UUID `gorm:"type:uuid;primaryKey;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"guild_id"`
	DayOfWeek int       `gorm:"not null" json:"day_of_week"`
	TimeOfDay string    `gorm:"size:5;not null" json:"time_of_day"`
	Timezone  string    `gorm:"default:Asia/Taipei" json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GuildDefaultSlotRepository interface {
	GetByGuildID(ctx context.Context, guildID uuid.UUID) (*GuildDefaultSlot, error)
	Upsert(ctx context.Context, slot *GuildDefaultSlot) error
	Delete(ctx context.Context, guildID uuid.UUID) error
}
