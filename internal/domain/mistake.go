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

// MistakeConfidence is the AI confidence of a correction (spec §2.1). Low
// confidence is flagged in the feedback UI.
type MistakeConfidence string

const (
	MistakeConfidenceLow    MistakeConfidence = "low"
	MistakeConfidenceMedium MistakeConfidence = "medium"
	MistakeConfidenceHigh   MistakeConfidence = "high"
)

// MistakeCorrectionSource says where the latest correction text came from
// (spec §2.1).
type MistakeCorrectionSource string

const (
	MistakeCorrectionSourceInitial MistakeCorrectionSource = "initial"
	MistakeCorrectionSourceChatbot MistakeCorrectionSource = "chatbot"
)

// Mistake represents a mistake in the jpcorrect system.
// Maps to jpcorrect.mistake table.
//
// The v1.2 additions come from spec §2.1. LatestCorrectionText is NULL until
// the chatbot has iterated on the correction; the frontend then falls back to
// FixedText, the initial version. A Mistake hangs off EventID, not an Activity.
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

	// v1.2 additions (spec §2.1).
	Confidence             MistakeConfidence       `gorm:"default:medium" json:"confidence"`
	LatestCorrectionText   *string                 `gorm:"type:text" json:"latest_correction_text"`
	LatestCorrectionSource MistakeCorrectionSource `gorm:"default:initial" json:"latest_correction_source"`
	InteractedAt           *time.Time              `json:"interacted_at"`
}

type MistakeRepository interface {
	GetByID(ctx context.Context, mistakeID uuid.UUID) (*Mistake, error)
	GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*Mistake, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*Mistake, error)

	Create(ctx context.Context, m *Mistake) error
	Update(ctx context.Context, m *Mistake) error
	Delete(ctx context.Context, mistakeID uuid.UUID) error
}
