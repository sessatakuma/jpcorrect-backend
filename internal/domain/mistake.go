package domain

import (
	"context"

	"github.com/google/uuid"
)

// MistakeType represents the type of a mistake.
type MistakeType string

const (
	MistakeTypeGrammar   MistakeType = "grammar"
	MistakeTypeVocab     MistakeType = "vocab"
	MistakeTypePronounce MistakeType = "pronounce"
	MistakeTypeAdvanced  MistakeType = "advanced"
)

// Mistake represents a mistake in the jpcorrect system.
// Maps to jpcorrect.mistake table.
type Mistake struct {
	ID         uuid.UUID   `gorm:"type:uuid;primaryKey" json:"mistake_id"`
	EventID    uuid.UUID   `gorm:"type:uuid;index" json:"event_id"`
	UserID     uuid.UUID   `gorm:"type:uuid;index" json:"user_id"`
	Type       MistakeType `gorm:"default:grammar" json:"type"`
	OriginText string      `json:"origin_text"`
	FixedText  string      `json:"fixed_text"`
	StartTime  float64     `json:"start_time"`
	EndTime    float64     `json:"end_time"`
	Comment    *string     `json:"comment"`
	Note       *string     `json:"note"`
}

type MistakeRepository interface {
	GetByID(ctx context.Context, mistakeID uuid.UUID) (*Mistake, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*Mistake, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Mistake, error)

	Create(ctx context.Context, m *Mistake) error
	Update(ctx context.Context, m *Mistake) error
	Delete(ctx context.Context, mistakeID uuid.UUID) error
}
