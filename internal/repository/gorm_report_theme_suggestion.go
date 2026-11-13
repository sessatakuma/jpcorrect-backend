package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormReportThemeSuggestionRepository struct {
	db *gorm.DB
}

func NewGormReportThemeSuggestionRepository(db *gorm.DB) domain.ReportThemeSuggestionRepository {
	return &gormReportThemeSuggestionRepository{db: db}
}

func (r *gormReportThemeSuggestionRepository) List(ctx context.Context) ([]*domain.ReportThemeSuggestion, error) {
	var suggestions []*domain.ReportThemeSuggestion
	err := r.db.WithContext(ctx).Find(&suggestions).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return suggestions, nil
}

func (r *gormReportThemeSuggestionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.ReportThemeSuggestion, error) {
	var suggestion domain.ReportThemeSuggestion
	err := r.db.WithContext(ctx).First(&suggestion, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &suggestion, nil
}
