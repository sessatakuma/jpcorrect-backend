package domain

import (
	"context"
)

// AICorrection represents the jpcorrect.ai_correction table
type User struct {
	UserID int    `db:"user_id" json:"user_id"`
	Name   string `db:"name" json:"name"`
}

type UserRepository interface {
	GetByID(ctx context.Context, userID int) (*User, error)
	GetByName(ctx context.Context, name string) ([]*User, error)

	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, userID int) error
}
