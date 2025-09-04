package domain

import (
	"context"
)

// AICorrection represents the jpcorrect.ai_correction table
type AICorrection struct {
	AICorrectionID int    `db:"ai_correction_id" json:"ai_correction_id"`
	ErrorID        int    `db:"error_id" json:"error_id"`
	Content        string `db:"content" json:"content"`
}

type AICorrectionRepository interface {
	GetByID(ctx context.Context, aiCorrectionID int) (*AICorrection, error)
	GetByErrorID(ctx context.Context, errorID int) ([]*AICorrection, error)

	Create(ctx context.Context, aiCorrection *AICorrection) error
	Update(ctx context.Context, aiCorrection *AICorrection) error
	Delete(ctx context.Context, aiCorrectionID int) error
}
