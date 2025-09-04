package domain

import (
	"context"
)

// Practice represents the jpcorrect.practice table
type Practice struct {
	PracticeID int `db:"practice_id" json:"practice_id"`
	UserID     int `db:"user_id" json:"user_id"`
}

type PracticeRepository interface {
	GetByID(ctx context.Context, practiceID int) (*Practice, error)
	GetByUserID(ctx context.Context, userID int) ([]*Practice, error)

	Create(ctx context.Context, practice *Practice) error
	Update(ctx context.Context, practice *Practice) error
	Delete(ctx context.Context, practiceID int) error
}
