package domain

import (
	"context"
)

// Error represents the jpcorrect.error table
type Error struct {
	ErrorID     int     `db:"error_id" json:"error_id"`
	PracticeID  int     `db:"practice_id" json:"practice_id"`
	UserID      int     `db:"user_id" json:"user_id"`
	StartTime   float64 `db:"start_time" json:"start_time"`
	EndTime     float64 `db:"end_time" json:"end_time"`
	ErrorStatus string  `db:"error_status" json:"error_status"`
	ErrorType   string  `db:"error_type" json:"error_type"`
}

type ErrorRepository interface {
	GetByID(ctx context.Context, errorID int) (*Error, error)
	GetByPracticeID(ctx context.Context, practiceID int) ([]*Error, error)

	Create(ctx context.Context, err *Error) error
	Update(ctx context.Context, err *Error) error
	Delete(ctx context.Context, errorID int) error
}
