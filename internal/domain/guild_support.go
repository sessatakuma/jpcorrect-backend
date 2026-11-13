package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// GuildDefaultSlot is a guild's default weekly practice slot (spec §2.2). A
// guild without a row has not set one, which is fully manual scheduling; the
// slot is only used to compute practice_at for auto-scheduling.
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none. guild_id is the primary key: the relation is
// 1:1.
type GuildDefaultSlot struct {
	GuildID   uuid.UUID      `gorm:"type:uuid;not null;primaryKey" json:"guild_id"`
	DayOfWeek int            `gorm:"not null" json:"day_of_week"`
	TimeOfDay datatypes.Time `gorm:"type:time;not null" json:"time_of_day"`
	Timezone  string         `gorm:"not null;default:Asia/Taipei" json:"timezone"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	Guild *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// JoinRequestStatus is the state of a guild board join request (spec §2.2).
type JoinRequestStatus string

const (
	JoinRequestStatusPending   JoinRequestStatus = "pending"
	JoinRequestStatusApproved  JoinRequestStatus = "approved"
	JoinRequestStatusRejected  JoinRequestStatus = "rejected"
	JoinRequestStatusCancelled JoinRequestStatus = "cancelled"
)

// JoinRequest is a request to join a guild from the guild board (spec §2.2).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type JoinRequest struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey" json:"id"`
	GuildID   uuid.UUID         `gorm:"type:uuid;not null;index" json:"guild_id"`
	UserID    uuid.UUID         `gorm:"type:uuid;not null;index;index:idx_join_request_user_status_pending,priority:1,where:status = 'pending'" json:"user_id"`
	Status    JoinRequestStatus `gorm:"not null;default:pending;index:idx_join_request_user_status_pending,priority:2" json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`

	Guild *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	User  *User  `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
