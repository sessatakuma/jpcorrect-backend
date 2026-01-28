package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserRole represents the role of a user.
type UserRole string

const (
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
	UserRoleStaff UserRole = "staff"
)

// UserStatus represents the status of a user account.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusBanned    UserStatus = "banned"
	UserStatusSuspended UserStatus = "suspended"
)

// User represents a user in the jpcorrect system.
// Maps to jpcorrect.user table.
type User struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"user_id"`
	Email           string         `gorm:"uniqueIndex" json:"email"`
	Name            string         `json:"name"`
	AvatarURL       *string        `json:"avatar_url"`
	PasswordHash    *string        `json:"-"` // Nullable: third-party login users have NULL
	IsEmailVerified bool           `gorm:"default:false" json:"is_email_verified"`
	Role            UserRole       `gorm:"default:user" json:"role"`
	Status          UserStatus     `gorm:"default:active" json:"status"`
	Timezone        string         `gorm:"default:Asia/Taipei" json:"timezone"`
	LateStreak      int            `gorm:"default:0" json:"late_streak"`
	Points          int            `gorm:"default:0" json:"points"`
	Level           int            `gorm:"default:0" json:"level"`
	CreatedAt       time.Time      `json:"created_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type UserRepository interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByName(ctx context.Context, name string) ([]*User, error)

	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, userID uuid.UUID) error
}
