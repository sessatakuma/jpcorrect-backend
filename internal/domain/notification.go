package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// NotificationOutboxStatus is the dispatch state of an outbox row (spec §2.2).
type NotificationOutboxStatus string

const (
	NotificationOutboxStatusPending    NotificationOutboxStatus = "pending"
	NotificationOutboxStatusProcessing NotificationOutboxStatus = "processing"
	NotificationOutboxStatusCompleted  NotificationOutboxStatus = "completed"
	NotificationOutboxStatusDead       NotificationOutboxStatus = "dead"
)

// NotificationOutbox is one logical notification for one recipient and the
// durable source of truth of the dispatch system (spec §2.2, §6.5).
//
// aggregate_id is a plain UUID, not a foreign key: the aggregate differs per
// kind (activity, guild, ...). Foreign-key actions are the project default
// (ON UPDATE CASCADE / ON DELETE RESTRICT); the spec gives none.
type NotificationOutbox struct {
	ID              uuid.UUID                `gorm:"type:uuid;primaryKey" json:"id"`
	DedupKey        string                   `gorm:"not null;uniqueIndex" json:"dedup_key"`
	Kind            string                   `gorm:"not null" json:"kind"`
	AggregateType   string                   `gorm:"not null" json:"aggregate_type"`
	AggregateID     uuid.UUID                `gorm:"type:uuid;not null" json:"aggregate_id"`
	RecipientUserID uuid.UUID                `gorm:"type:uuid;not null;index" json:"recipient_user_id"`
	Payload         datatypes.JSON           `gorm:"type:jsonb;not null" json:"payload"`
	Status          NotificationOutboxStatus `gorm:"default:pending" json:"status"`
	AttemptCount    int                      `gorm:"default:0" json:"attempt_count"`
	NextAttemptAt   *time.Time               `json:"next_attempt_at"`
	LastError       *string                  `gorm:"type:text" json:"last_error"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`

	RecipientUser *User `gorm:"foreignKey:RecipientUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// Notification is one in-app notification for one user (spec §2.2). outbox_id
// is unique, which is what makes in-app delivery idempotent: delivering the
// same outbox row twice cannot produce a second Notification (§6.5).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type Notification struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index;index:idx_notification_user_read_created,priority:1;index:idx_notification_user_unread,where:read_at IS NULL" json:"user_id"`
	Kind      string         `gorm:"not null" json:"kind"`
	Payload   datatypes.JSON `gorm:"type:jsonb;not null" json:"payload"`
	ReadAt    *time.Time     `gorm:"index;index:idx_notification_user_read_created,priority:2" json:"read_at"`
	OutboxID  uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"outbox_id"`
	CreatedAt time.Time      `gorm:"index;index:idx_notification_user_read_created,priority:3,sort:desc" json:"created_at"`

	User   *User               `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	Outbox *NotificationOutbox `gorm:"foreignKey:OutboxID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// PushSubscription is one browser Web Push subscription of a user (spec §2.2).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type PushSubscription struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Endpoint  string         `gorm:"type:text;not null;uniqueIndex" json:"endpoint"`
	Keys      datatypes.JSON `gorm:"type:jsonb;not null" json:"keys"`
	CreatedAt time.Time      `json:"created_at"`

	User *User `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
