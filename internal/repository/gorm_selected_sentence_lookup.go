package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormSelectedSentenceLookupRepository struct {
	db *gorm.DB
}

// NewGormSelectedSentenceLookupRepository returns the read-only SelectedSentence
// lookup used by the ownership check (issue #74).
func NewGormSelectedSentenceLookupRepository(db *gorm.DB) domain.SelectedSentenceLookupRepository {
	return &gormSelectedSentenceLookupRepository{db: db}
}

// GetByID returns one SelectedSentence. A missing row maps to
// domain.ErrNotFound.
func (r *gormSelectedSentenceLookupRepository) GetByID(ctx context.Context, selectedSentenceID uuid.UUID) (*domain.SelectedSentence, error) {
	var selected domain.SelectedSentence
	if err := r.db.WithContext(ctx).First(&selected, "id = ?", selectedSentenceID).Error; err != nil {
		return nil, MapGormError(err)
	}
	return &selected, nil
}
