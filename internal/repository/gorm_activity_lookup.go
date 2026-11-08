package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormActivityLookupRepository struct {
	db *gorm.DB
}

// NewGormActivityLookupRepository returns the read-only Activity lookup used by
// the permission checks (issue #74). It is not the full Activity repository.
func NewGormActivityLookupRepository(db *gorm.DB) domain.ActivityLookupRepository {
	return &gormActivityLookupRepository{db: db}
}

// GetByID returns one non-deleted Activity. A soft-deleted or missing row maps
// to domain.ErrNotFound.
func (r *gormActivityLookupRepository) GetByID(ctx context.Context, activityID uuid.UUID) (*domain.Activity, error) {
	var activity domain.Activity
	if err := r.db.WithContext(ctx).First(&activity, "id = ?", activityID).Error; err != nil {
		return nil, MapGormError(err)
	}
	return &activity, nil
}
