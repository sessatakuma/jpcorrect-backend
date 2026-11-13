package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormActivityRepository struct {
	db *gorm.DB
}

func NewGormActivityRepository(db *gorm.DB) domain.ActivityRepository {
	return &gormActivityRepository{db: db}
}

func (r *gormActivityRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Activity, error) {
	var activity domain.Activity
	err := r.db.WithContext(ctx).First(&activity, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &activity, nil
}

func (r *gormActivityRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID, page, perPage int) ([]*domain.Activity, error) {
	var activities []*domain.Activity
	offset := (page - 1) * perPage
	err := r.db.WithContext(ctx).
		Where("guild_id = ?", guildID).
		Offset(offset).
		Limit(perPage).
		Order("created_at DESC").
		Find(&activities).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return activities, nil
}

func (r *gormActivityRepository) Create(ctx context.Context, activity *domain.Activity) error {
	if activity.ID == uuid.Nil {
		activity.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(activity).Error)
}

func (r *gormActivityRepository) Update(ctx context.Context, activity *domain.Activity) error {
	return MapGormError(r.db.WithContext(ctx).Save(activity).Error)
}

func (r *gormActivityRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.ActivityStatus) error {
	return MapGormError(r.db.WithContext(ctx).Model(&domain.Activity{}).Where("id = ?", id).Update("status", status).Error)
}

func (r *gormActivityRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.Activity{}, "id = ?", id).Error)
}
