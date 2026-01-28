package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormTranscriptRepository struct {
	db *gorm.DB
}

func NewGormTranscriptRepository(db *gorm.DB) domain.TranscriptRepository {
	return &gormTranscriptRepository{db: db}
}

func (r *gormTranscriptRepository) GetByID(ctx context.Context, transcriptID uuid.UUID) (*domain.Transcript, error) {
	var transcript domain.Transcript
	err := r.db.WithContext(ctx).First(&transcript, "id = ?", transcriptID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &transcript, nil
}

func (r *gormTranscriptRepository) GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*domain.Transcript, error) {
	var transcripts []*domain.Transcript
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Find(&transcripts).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return transcripts, nil
}

func (r *gormTranscriptRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Transcript, error) {
	var transcripts []*domain.Transcript
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&transcripts).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return transcripts, nil
}

func (r *gormTranscriptRepository) Create(ctx context.Context, transcript *domain.Transcript) error {
	if transcript.ID == uuid.Nil {
		transcript.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(transcript).Error)
}

func (r *gormTranscriptRepository) Update(ctx context.Context, transcript *domain.Transcript) error {
	return MapGormError(r.db.WithContext(ctx).Save(transcript).Error)
}

func (r *gormTranscriptRepository) Delete(ctx context.Context, transcriptID uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.Transcript{}, "id = ?", transcriptID).Error)
}
