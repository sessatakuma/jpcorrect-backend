package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormMistakeRepository struct {
	db *gorm.DB
}

// NewGormMistakeRepository creates a new GORM-based mistake repository.
func NewGormMistakeRepository(db *gorm.DB) domain.MistakeRepository {
	return &gormMistakeRepository{db: db}
}

func (r *gormMistakeRepository) GetByID(ctx context.Context, mistakeID uuid.UUID) (*domain.Mistake, error) {
	var mistake domain.Mistake
	err := r.db.WithContext(ctx).First(&mistake, "id = ?", mistakeID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &mistake, nil
}

func (r *gormMistakeRepository) GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*domain.Mistake, error) {
	var mistakes []*domain.Mistake
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Find(&mistakes).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return mistakes, nil
}

func (r *gormMistakeRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Mistake, error) {
	var mistakes []*domain.Mistake
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&mistakes).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return mistakes, nil
}

func (r *gormMistakeRepository) Create(ctx context.Context, m *domain.Mistake) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(m).Error)
}

func (r *gormMistakeRepository) Update(ctx context.Context, m *domain.Mistake) error {
	return MapGormError(r.db.WithContext(ctx).Save(m).Error)
}

func (r *gormMistakeRepository) Delete(ctx context.Context, mistakeID uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.Mistake{}, "id = ?", mistakeID).Error)
}
