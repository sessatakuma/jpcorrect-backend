package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MistakeType represents the type of a mistake.
type MistakeType string

const (
	MistakeTypeGrammar       MistakeType = "grammar"
	MistakeTypeVocab         MistakeType = "vocab"
	MistakeTypePronunciation MistakeType = "pronunciation"
	MistakeTypeAdvanced      MistakeType = "advanced"
)

// Mistake represents a mistake in the jpcorrect system.
// Maps to jpcorrect.mistake table.
type Mistake struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey" json:"mistake_id"`
	EventID        uuid.UUID   `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"event_id"`
	UserID         uuid.UUID   `gorm:"type:uuid;index;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_id"`
	Type           MistakeType `gorm:"default:grammar" json:"type"`
	OriginText     string      `gorm:"type:text" json:"origin_text"`
	FixedText      string      `gorm:"type:text" json:"fixed_text"`
	StartOffsetSec float64     `json:"start_offset_sec"`
	EndOffsetSec   float64     `json:"end_offset_sec"`
	Comment        *string     `gorm:"type:text" json:"comment"`
	Note           *string     `gorm:"type:text" json:"note"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type MistakeRepository interface {
	GetByID(ctx context.Context, mistakeID uuid.UUID) (*Mistake, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*Mistake, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Mistake, error)

	Create(ctx context.Context, m *Mistake) error
	Update(ctx context.Context, m *Mistake) error
	Delete(ctx context.Context, mistakeID uuid.UUID) error
}
