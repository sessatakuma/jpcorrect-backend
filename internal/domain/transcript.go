package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Transcript represents a transcript in the jpcorrect system.
// Maps to jpcorrect.transcript table.
type Transcript struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"transcript_id"`
	EventID        uuid.UUID      `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"event_id"`
	UserID         uuid.UUID      `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_id"`
	Content        string         `gorm:"type:text" json:"content"`
	Accent         datatypes.JSON `gorm:"type:jsonb" json:"accent"`
	StartOffsetSec float64        `json:"start_offset_sec"`
	EndOffsetSec   float64        `json:"end_offset_sec"`
	Note           *string        `gorm:"type:text" json:"note"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type TranscriptRepository interface {
	GetByID(ctx context.Context, transcriptID uuid.UUID) (*Transcript, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*Transcript, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Transcript, error)

	Create(ctx context.Context, transcript *Transcript) error
	Update(ctx context.Context, transcript *Transcript) error
	Delete(ctx context.Context, transcriptID uuid.UUID) error
}
