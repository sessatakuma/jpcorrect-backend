package domain

import (
	"context"
)

// Transcript represents the jpcorrect.transcript table
type Transcript struct {
	TranscriptID int    `db:"transcript_id" json:"transcript_id"`
	MistakeID    int    `db:"mistake_id" json:"mistake_id"`
	Content      string `db:"content" json:"content"`
	Furigana     string `db:"furigana" json:"furigana"`
	Accent       string `db:"accent" json:"accent"`
}

type TranscriptRepository interface {
	GetByID(ctx context.Context, transcriptID int) (*Transcript, error)
	GetByMistakeID(ctx context.Context, mistakeID int) ([]*Transcript, error)

	Create(ctx context.Context, transcript *Transcript) error
	Update(ctx context.Context, transcript *Transcript) error
	Delete(ctx context.Context, transcriptID int) error
}
