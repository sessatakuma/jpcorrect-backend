package domain

import (
	"context"

	"github.com/google/uuid"
)

// Transcript represents a transcript in the jpcorrect system.
// Maps to jpcorrect.transcript table.
type Transcript struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"transcript_id"`
	EventID    uuid.UUID `gorm:"type:uuid;index" json:"event_id"`
	UserID     uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Transcript string    `json:"transcript"`
	Accent     string    `gorm:"type:jsonb" json:"accent"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Note       *string   `json:"note"`
}

type TranscriptRepository interface {
	GetByID(ctx context.Context, transcriptID uuid.UUID) (*Transcript, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*Transcript, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Transcript, error)

	Create(ctx context.Context, transcript *Transcript) error
	Update(ctx context.Context, transcript *Transcript) error
	Delete(ctx context.Context, transcriptID uuid.UUID) error
}
