package repository

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type postgresNoteRepository struct {
	conn Connection
}

func NewPostgresNote(conn Connection) domain.NoteRepository {
	return &postgresNoteRepository{conn: conn}
}

func (p *postgresNoteRepository) GetByID(ctx context.Context, noteID int) (*domain.Note, error) {
	return nil, nil
}
func (p *postgresNoteRepository) GetByPracticeID(ctx context.Context, practiceID int) ([]*domain.Note, error) {
	return nil, nil
}
func (p *postgresNoteRepository) Create(ctx context.Context, note *domain.Note) error {
	return nil
}
func (p *postgresNoteRepository) Update(ctx context.Context, note *domain.Note) error {
	return nil
}
func (p *postgresNoteRepository) Delete(ctx context.Context, noteID int) error {
	return nil
}
