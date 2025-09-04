package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresPracticeRepository struct {
	conn Connection
}

func NewPostgresPractice(conn Connection) domain.PracticeRepository {
	return &postgresPracticeRepository{conn: conn}
}

func (p *postgresPracticeRepository) GetByID(ctx context.Context, practiceID int) (*domain.Practice, error) {
	return nil, nil
}
func (p *postgresPracticeRepository) GetByUserID(ctx context.Context, userID int) ([]*domain.Practice, error) {
	return nil, nil
}
func (p *postgresPracticeRepository) Create(ctx context.Context, practice *domain.Practice) error {
	return nil
}
func (p *postgresPracticeRepository) Update(ctx context.Context, practice *domain.Practice) error {
	return nil
}
func (p *postgresPracticeRepository) Delete(ctx context.Context, practiceID int) error {
	return nil
}
