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
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"user_id"`
	Email           string     `gorm:"uniqueIndex" json:"email"`
	Name            string     `json:"name"`
	AvatarURL       *string    `json:"avatar_url"`
	PasswordHash    *string    `gorm:"column:password_hash" json:"-"`
	IsEmailVerified bool       `gorm:"default:false" json:"is_email_verified"`
	Role            UserRole   `gorm:"default:user" json:"role"`
	Status          UserStatus `gorm:"default:active" json:"status"`
	Timezone        string     `gorm:"default:Asia/Taipei" json:"timezone"`

	// SupabaseUserID is the Supabase Auth user UUID looked up from JWT.sub
	// (spec §2.1). It is nullable: a row created before the mapping exists has
	// no value yet. The unique index doubles as the lookup index. It is a UUID
	// column, not text, so PostgreSQL rejects a malformed identifier.
	SupabaseUserID *uuid.UUID     `gorm:"type:uuid;uniqueIndex" json:"supabase_user_id"`
	LateStreak     int            `gorm:"default:0" json:"late_streak"`
	Points         int            `gorm:"default:0" json:"points"`
	Level          int            `gorm:"default:0" json:"level"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type UserRepository interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByName(ctx context.Context, name string) ([]*User, error)

	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, userID uuid.UUID) error
}
