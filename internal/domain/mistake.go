package domain

import (
	"context"
)

// Mistake represents the jpcorrect.mistake table
type Mistake struct {
	MistakeID     int     `db:"mistake_id" json:"mistake_id"`
	PracticeID    int     `db:"practice_id" json:"practice_id"`
	UserID        int     `db:"user_id" json:"user_id"`
	StartTime     float64 `db:"start_time" json:"start_time"`
	EndTime       float64 `db:"end_time" json:"end_time"`
	MistakeStatus string  `db:"mistake_status" json:"mistake_status"`
	MistakeType   string  `db:"mistake_type" json:"mistake_type"`
}

type MistakeRepository interface {
	GetByID(ctx context.Context, mistakeID int) (*Mistake, error)
	GetByPracticeID(ctx context.Context, practiceID int) ([]*Mistake, error)
	GetByUserID(ctx context.Context, userID int) ([]*Mistake, error)

	Create(ctx context.Context, m *Mistake) error
	Update(ctx context.Context, m *Mistake) error
	Delete(ctx context.Context, mistakeID int) error
}
