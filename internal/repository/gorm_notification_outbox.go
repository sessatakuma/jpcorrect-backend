package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"jpcorrect-backend/internal/domain"
)

// gormNotificationOutboxRepository writes notification intents to
// NotificationOutbox (spec §2.2, §6.5). It is the write side only: it never
// claims, dispatches or retries a row.
type gormNotificationOutboxRepository struct {
	db *gorm.DB
}

// NewGormNotificationOutboxRepository returns the outbox write side bound to
// db. A caller that must commit an intent together with a state change uses the
// repository of the Store.Tx RepoSet instead, so both share one transaction.
func NewGormNotificationOutboxRepository(db *gorm.DB) domain.NotificationOutboxRepository {
	return &gormNotificationOutboxRepository{db: db}
}

// CreateIntents inserts every intent in one batch statement, ignoring intents
// whose dedup key already exists (spec §2.2, §6.5).
func (r *gormNotificationOutboxRepository) CreateIntents(ctx context.Context, intents []*domain.NotificationOutbox) error {
	if len(intents) == 0 {
		return nil
	}

	for _, intent := range intents {
		if intent.ID == uuid.Nil {
			intent.ID = uuid.New()
		}
	}

	// ON CONFLICT (dedup_key) DO NOTHING makes a duplicate intent a successful
	// no-op, so the caller's transaction can still commit. PostgreSQL also
	// ignores a duplicate key inside this one statement, so a batch that
	// contains the same key twice inserts the first row only.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dedup_key"}},
			DoNothing: true,
		}).
		Create(intents).Error

	return MapGormError(err)
}
