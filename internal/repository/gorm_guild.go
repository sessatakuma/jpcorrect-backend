package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormGuildRepository struct {
	db *gorm.DB
}

// NewGormGuildRepository creates a new GORM-based guild repository.
func NewGormGuildRepository(db *gorm.DB) domain.GuildRepository {
	return &gormGuildRepository{db: db}
}

func (r *gormGuildRepository) GetByID(ctx context.Context, guildID uuid.UUID) (*domain.Guild, error) {
	var guild domain.Guild
	err := r.db.WithContext(ctx).First(&guild, "id = ?", guildID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &guild, nil
}

func (r *gormGuildRepository) Create(ctx context.Context, guild *domain.Guild) error {
	if guild.ID == uuid.Nil {
		guild.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(guild).Error)
}

func (r *gormGuildRepository) Update(ctx context.Context, guild *domain.Guild) error {
	return MapGormError(r.db.WithContext(ctx).Save(guild).Error)
}

func (r *gormGuildRepository) Delete(ctx context.Context, guildID uuid.UUID) error {
	var attendeeCount int64
	err := r.db.WithContext(ctx).Model(&domain.GuildAttendee{}).Where("guild_id = ?", guildID).Count(&attendeeCount).Error
	if err != nil {
		return MapGormError(err)
	}
	if attendeeCount > 0 {
		return errors.New("cannot delete guild: has attendees")
	}

	return MapGormError(r.db.WithContext(ctx).Delete(&domain.Guild{}, "id = ?", guildID).Error)
}
