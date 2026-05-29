package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormInviteLinkRepository struct {
	db *gorm.DB
}

func NewGormInviteLinkRepository(db *gorm.DB) domain.InviteLinkRepository {
	return &gormInviteLinkRepository{db: db}
}

func (r *gormInviteLinkRepository) GetByToken(ctx context.Context, token string) (*domain.InviteLink, error) {
	var link domain.InviteLink
	err := r.db.WithContext(ctx).First(&link, "token = ?", token).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &link, nil
}

func (r *gormInviteLinkRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*domain.InviteLink, error) {
	var links []*domain.InviteLink
	err := r.db.WithContext(ctx).Where("guild_id = ?", guildID).Find(&links).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return links, nil
}

func (r *gormInviteLinkRepository) Create(ctx context.Context, link *domain.InviteLink) error {
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(link).Error)
}

func (r *gormInviteLinkRepository) Update(ctx context.Context, link *domain.InviteLink) error {
	return MapGormError(r.db.WithContext(ctx).Save(link).Error)
}
