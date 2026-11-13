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
	PasswordHash    *string        `gorm:"column:password_hash" json:"-"`
	IsEmailVerified bool           `gorm:"default:false" json:"is_email_verified"`
	Role            UserRole       `gorm:"default:user" json:"role"`
	Status          UserStatus     `gorm:"default:active" json:"status"`
	Timezone        string         `gorm:"default:Asia/Taipei" json:"timezone"`
	LateStreak      int            `gorm:"default:0" json:"late_streak"`
	Points          int            `gorm:"default:0" json:"points"`
	Level           int            `gorm:"default:0" json:"level"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// SupabaseUserID is the Supabase Auth user UUID taken from the verified JWT
	// `sub`. It is nullable on purpose: rows written before this mapping existed
	// have no value, so AutoMigrate adds a NULL column to a populated table and
	// those rows stay readable. The unique index behind it is the lookup index and
	// the conflict target of the idempotent insert.
	SupabaseUserID *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"supabase_user_id"`
}

type UserRepository interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByName(ctx context.Context, name string) ([]*User, error)

	// GetBySupabaseUserID resolves the internal row from the verified JWT `sub`.
	// It returns ErrNotFound while the caller has no account yet.
	GetBySupabaseUserID(ctx context.Context, supabaseUserID uuid.UUID) (*User, error)

	Create(ctx context.Context, user *User) error
	// CreateIfAbsentBySupabaseUserID inserts user only when no row already owns
	// the same Supabase subject. A retried or concurrent first login therefore
	// leaves exactly one row; the caller reads it back with
	// GetBySupabaseUserID. An email owned by another row is not linked and
	// returns ErrDuplicateEntry.
	CreateIfAbsentBySupabaseUserID(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, userID uuid.UUID) error
}
