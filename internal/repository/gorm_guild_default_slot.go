package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormGuildDefaultSlotRepository struct {
	db *gorm.DB
}

func NewGormGuildDefaultSlotRepository(db *gorm.DB) domain.GuildDefaultSlotRepository {
	return &gormGuildDefaultSlotRepository{db: db}
}

func (r *gormGuildDefaultSlotRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID) (*domain.GuildDefaultSlot, error) {
	var slot domain.GuildDefaultSlot
	err := r.db.WithContext(ctx).First(&slot, "guild_id = ?", guildID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &slot, nil
}

func (r *gormGuildDefaultSlotRepository) Upsert(ctx context.Context, slot *domain.GuildDefaultSlot) error {
	return MapGormError(r.db.WithContext(ctx).Save(slot).Error)
}

func (r *gormGuildDefaultSlotRepository) Delete(ctx context.Context, guildID uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.GuildDefaultSlot{}, "guild_id = ?", guildID).Error)
}
