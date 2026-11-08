package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormActivityStatusRepository struct {
	db *gorm.DB
}

// NewGormActivityStatusRepository returns the repository that changes the
// status of an Activity. db may be the root handle or a transaction.
func NewGormActivityStatusRepository(db *gorm.DB) domain.ActivityStatusRepository {
	return &gormActivityStatusRepository{db: db}
}

// TransitionStatus is a compare-and-swap: the single UPDATE only touches a row
// that is still in one of the from statuses and not yet in to, and it writes
// status_entered_at in the same statement, so two concurrent callers cannot
// both win and the timestamp never lags behind the status.
func (r *gormActivityStatusRepository) TransitionStatus(ctx context.Context, activityID uuid.UUID, from []domain.ActivityStatus, to domain.ActivityStatus) (bool, error) {
	if len(from) == 0 {
		return false, nil
	}

	result := r.db.WithContext(ctx).Model(&domain.Activity{}).
		Where("id = ? AND status IN ? AND status <> ?", activityID, from, to).
		Updates(map[string]any{
			"status":            to,
			"status_entered_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return false, MapGormError(result.Error)
	}

	return result.RowsAffected == 1, nil
}
