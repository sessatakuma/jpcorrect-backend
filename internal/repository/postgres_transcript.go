package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresTranscriptRepository struct {
	conn Connection
}

func NewPostgresTranscript(conn Connection) domain.TranscriptRepository {
	return &postgresTranscriptRepository{conn: conn}
}

func (p *postgresTranscriptRepository) GetByID(ctx context.Context, transcriptID int) (*domain.Transcript, error) {
	return nil, nil
}
func (p *postgresTranscriptRepository) GetByErrorID(ctx context.Context, errorID int) ([]*domain.Transcript, error) {
	return nil, nil
}
func (p *postgresTranscriptRepository) Create(ctx context.Context, transcript *domain.Transcript) error {
	return nil
}
func (p *postgresTranscriptRepository) Update(ctx context.Context, transcript *domain.Transcript) error {
	return nil
}
func (p *postgresTranscriptRepository) Delete(ctx context.Context, transcriptID int) error {
	return nil
}
