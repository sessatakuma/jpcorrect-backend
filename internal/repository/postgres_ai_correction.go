package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresAICorrectionRepository struct {
	conn Connection
}

func NewPostgresAICorrection(conn Connection) domain.AICorrectionRepository {
	return &postgresAICorrectionRepository{conn: conn}
}

func (p *postgresAICorrectionRepository) GetByID(ctx context.Context, aiCorrectionID int) (*domain.AICorrection, error) {
	return nil, nil
}
func (p *postgresAICorrectionRepository) GetByMistakeID(ctx context.Context, mistakeID int) ([]*domain.AICorrection, error) {
	return nil, nil
}
func (p *postgresAICorrectionRepository) Create(ctx context.Context, aiCorrection *domain.AICorrection) error {
	return nil
}
func (p *postgresAICorrectionRepository) Update(ctx context.Context, aiCorrection *domain.AICorrection) error {
	return nil
}
func (p *postgresAICorrectionRepository) Delete(ctx context.Context, aiCorrectionID int) error {
	return nil
}
