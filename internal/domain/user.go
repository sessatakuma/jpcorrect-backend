package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the jpcorrect system.
// Maps to jpcorrect.user table.
type User struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"user_id"`
	Email      string         `gorm:"uniqueIndex" json:"email"`
	Name       string         `json:"name"`
	AvatarURL  *string        `json:"avatar_url"`
	Timezone   string         `gorm:"default:Asia/Taipei" json:"timezone"`
	LateStreak int            `gorm:"default:0" json:"late_streak"`
	Points     int            `gorm:"default:0" json:"points"`
	Level      int            `gorm:"default:0" json:"level"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type UserRepository interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByName(ctx context.Context, name string) ([]*User, error)

	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, userID uuid.UUID) error
}
