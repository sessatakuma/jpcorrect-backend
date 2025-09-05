package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresMistakeRepository struct {
	conn Connection
}

func NewPostgresMistake(conn Connection) domain.MistakeRepository {
	return &postgresMistakeRepository{conn: conn}
}

func (p *postgresMistakeRepository) GetByID(ctx context.Context, mistakeID int) (*domain.Mistake, error) {
	return nil, nil
}
func (p *postgresMistakeRepository) GetByPracticeID(ctx context.Context, practiceID int) ([]*domain.Mistake, error) {
	return nil, nil
}
func (p *postgresMistakeRepository) Create(ctx context.Context, m *domain.Mistake) error {
	return nil
}
func (p *postgresMistakeRepository) Update(ctx context.Context, m *domain.Mistake) error {
	return nil
}
func (p *postgresMistakeRepository) Delete(ctx context.Context, mistakeID int) error {
	return nil
}
