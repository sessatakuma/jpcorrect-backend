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

func (p *postgresNoteRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.Note, error) {
	rows, err := p.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []*domain.Note
	for rows.Next() {
		var note domain.Note
		if err := rows.Scan(
			&note.NoteID,
			&note.PracticeID,
			&note.Content,
		); err != nil {
			return nil, err
		}
		notes = append(notes, &note)
	}
	return notes, nil
}

func (p *postgresNoteRepository) GetByID(ctx context.Context, noteID int) (*domain.Note, error) {
	query := `
		SELECT note_id, practice_id, content
		FROM jpcorrect.note
		WHERE note_id = $1`

	notes, err := p.fetch(ctx, query, noteID)
	if err != nil {
		return nil, err
	}
	if len(notes) == 0 {
		return nil, domain.ErrNotFound
	}
	return notes[0], nil
}

func (p *postgresNoteRepository) GetByPracticeID(ctx context.Context, practiceID int) (*domain.Note, error) {
	query := `
		SELECT note_id, practice_id, content
		FROM jpcorrect.note
		WHERE practice_id = $1`

	notes, err := p.fetch(ctx, query, practiceID)
	if err != nil {
		return nil, err
	}
	if len(notes) == 0 {
		return nil, domain.ErrNotFound
	}
	return notes[0], nil
}

func (p *postgresNoteRepository) Create(ctx context.Context, note *domain.Note) error {
	query := `
		INSERT INTO jpcorrect.note (practice_id, content)
		VALUES ($1, $2)
		RETURNING note_id`

	if err := p.conn.QueryRow(ctx, query, note.PracticeID, note.Content).Scan(&note.NoteID); err != nil {
		return err
	}
	return nil
}

func (p *postgresNoteRepository) Update(ctx context.Context, note *domain.Note) error {
	query := `
		UPDATE jpcorrect.note
		SET practice_id = $1, content = $2
		WHERE note_id = $3`

	if _, err := p.conn.Exec(ctx, query, note.PracticeID, note.Content, note.NoteID); err != nil {
		return err
	}
	return nil
}

func (p *postgresNoteRepository) Delete(ctx context.Context, noteID int) error {
	query := `
		DELETE FROM jpcorrect.note
		WHERE note_id = $1`

	if _, err := p.conn.Exec(ctx, query, noteID); err != nil {
		return err
	}
	return nil
}
