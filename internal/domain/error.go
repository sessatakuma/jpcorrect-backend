package domain

import (
	"context"
)

// Error represents the jpcorrect.error table
type Error struct {
	ErrorID        int     `db:"error_id" json:"error_id"`
	PracticeID     int     `db:"practice_id" json:"practice_id"`
	UserID         int     `db:"user_id" json:"user_id"`
	ErrorType      string  `db:"error_type" json:"error_type"`
	AIDetected     bool    `db:"ai_detected" json:"ai_detected"`
	AIMiscorrected bool    `db:"ai_miscorrected" json:"ai_miscorrected"`
	HumanCorrected bool    `db:"human_corrected" json:"human_corrected"`
	StartTime      float64 `db:"start_time" json:"start_time"`
	EndTime        float64 `db:"end_time" json:"end_time"`
}

type ErrorRepository interface {
	GetByID(ctx context.Context, errorID int) (*Error, error)
	GetByPracticeID(ctx context.Context, practiceID int) ([]*Error, error)

	Create(ctx context.Context, err *Error) error
	Update(ctx context.Context, err *Error) error
	Delete(ctx context.Context, errorID int) error
}
