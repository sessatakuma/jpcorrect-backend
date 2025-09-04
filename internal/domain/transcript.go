package domain

import (
	"context"
)

// Transcript represents the jpcorrect.transcript table
type Transcript struct {
	TranscriptID int    `db:"transcript_id" json:"transcript_id"`
	ErrorID      int    `db:"error_id" json:"error_id"`
	Content      string `db:"content" json:"content"`
	Furigana     string `db:"furigana" json:"furigana"`
	Accent       string `db:"accent" json:"accent"`
}

type TranscriptRepository interface {
	GetByID(ctx context.Context, transcriptID int) (*Transcript, error)
	GetByErrorID(ctx context.Context, errorID int) ([]*Transcript, error)

	Create(ctx context.Context, transcript *Transcript) error
	Update(ctx context.Context, transcript *Transcript) error
	Delete(ctx context.Context, transcriptID int) error
}
