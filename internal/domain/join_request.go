package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// JoinRequestStatus represents the status of a guild join request.
type JoinRequestStatus string

const (
	JoinRequestStatusPending   JoinRequestStatus = "pending"
	JoinRequestStatusApproved  JoinRequestStatus = "approved"
	JoinRequestStatusRejected  JoinRequestStatus = "rejected"
	JoinRequestStatusCancelled JoinRequestStatus = "cancelled"
)

// JoinRequest represents a request to join a guild.
// Maps to jpcorrect.join_request table.
type JoinRequest struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey" json:"join_request_id"`
	GuildID   uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_join_request_guild_user,priority:1;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"guild_id"`
	UserID    uuid.UUID         `gorm:"type:uuid;uniqueIndex:idx_join_request_guild_user,priority:2;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"user_id"`
	Status    JoinRequestStatus `gorm:"default:pending" json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type JoinRequestRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*JoinRequest, error)
	GetByGuildID(ctx context.Context, guildID uuid.UUID, status *JoinRequestStatus) ([]*JoinRequest, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*JoinRequest, error)
	Create(ctx context.Context, req *JoinRequest) error
	Update(ctx context.Context, req *JoinRequest) error
	CancelPendingByUserID(ctx context.Context, userID uuid.UUID) error
}
