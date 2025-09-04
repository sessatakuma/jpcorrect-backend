package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresErrorRepository struct {
	conn Connection
}

func NewPostgresError(conn Connection) domain.ErrorRepository {
	return &postgresErrorRepository{conn: conn}
}

func (p *postgresErrorRepository) GetByID(ctx context.Context, errorID int) (*domain.Error, error) {
	return nil, nil
}
func (p *postgresErrorRepository) GetByPracticeID(ctx context.Context, practiceID int) ([]*domain.Error, error) {
	return nil, nil
}
func (p *postgresErrorRepository) Create(ctx context.Context, error *domain.Error) error {
	return nil
}
func (p *postgresErrorRepository) Update(ctx context.Context, error *domain.Error) error {
	return nil
}
func (p *postgresErrorRepository) Delete(ctx context.Context, errorID int) error {
	return nil
}
