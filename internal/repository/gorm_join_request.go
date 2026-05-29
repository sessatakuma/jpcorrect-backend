package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormJoinRequestRepository struct {
	db *gorm.DB
}

func NewGormJoinRequestRepository(db *gorm.DB) domain.JoinRequestRepository {
	return &gormJoinRequestRepository{db: db}
}

func (r *gormJoinRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.JoinRequest, error) {
	var req domain.JoinRequest
	err := r.db.WithContext(ctx).First(&req, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &req, nil
}

func (r *gormJoinRequestRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID, status *domain.JoinRequestStatus) ([]*domain.JoinRequest, error) {
	var requests []*domain.JoinRequest
	db := r.db.WithContext(ctx).Where("guild_id = ?", guildID)
	if status != nil {
		db = db.Where("status = ?", *status)
	}
	err := db.Find(&requests).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return requests, nil
}

func (r *gormJoinRequestRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.JoinRequest, error) {
	var requests []*domain.JoinRequest
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&requests).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return requests, nil
}

func (r *gormJoinRequestRepository) Create(ctx context.Context, req *domain.JoinRequest) error {
	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(req).Error)
}

func (r *gormJoinRequestRepository) Update(ctx context.Context, req *domain.JoinRequest) error {
	return MapGormError(r.db.WithContext(ctx).Save(req).Error)
}

func (r *gormJoinRequestRepository) CancelPendingByUserID(ctx context.Context, userID uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Model(&domain.JoinRequest{}).
		Where("user_id = ? AND status = ?", userID, domain.JoinRequestStatusPending).
		Update("status", domain.JoinRequestStatusCancelled).Error)
}
