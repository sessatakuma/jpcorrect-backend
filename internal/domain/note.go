package domain

import (
	"context"
)

// Note represents the jpcorrect.note table
type Note struct {
	NoteID     int    `db:"note_id" json:"note_id"`
	PracticeID int    `db:"practice_id" json:"practice_id"`
	Content    string `db:"content" json:"content"`
}

type NoteRepository interface {
	GetByID(ctx context.Context, noteID int) (*Note, error)
	GetByPracticeID(ctx context.Context, practiceID int) (*Note, error)

	Create(ctx context.Context, note *Note) error
	Update(ctx context.Context, note *Note) error
	Delete(ctx context.Context, noteID int) error
}
