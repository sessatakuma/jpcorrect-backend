package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormGuildAttendeeRepository struct {
	db *gorm.DB
}

// NewGormGuildAttendeeRepository creates a new GORM-based guild attendee repository.
func NewGormGuildAttendeeRepository(db *gorm.DB) domain.GuildAttendeeRepository {
	return &gormGuildAttendeeRepository{db: db}
}

func (r *gormGuildAttendeeRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.GuildAttendee, error) {
	var attendee domain.GuildAttendee
	err := r.db.WithContext(ctx).First(&attendee, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &attendee, nil
}

func (r *gormGuildAttendeeRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID) ([]*domain.GuildAttendee, error) {
	var attendees []*domain.GuildAttendee
	err := r.db.WithContext(ctx).Where("guild_id = ?", guildID).Find(&attendees).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return attendees, nil
}

func (r *gormGuildAttendeeRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.GuildAttendee, error) {
	var attendees []*domain.GuildAttendee
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&attendees).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return attendees, nil
}

func (r *gormGuildAttendeeRepository) Create(ctx context.Context, attendee *domain.GuildAttendee) error {
	if attendee.ID == uuid.Nil {
		attendee.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(attendee).Error)
}

func (r *gormGuildAttendeeRepository) Update(ctx context.Context, attendee *domain.GuildAttendee) error {
	return MapGormError(r.db.WithContext(ctx).Save(attendee).Error)
}

func (r *gormGuildAttendeeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.GuildAttendee{}, "id = ?", id).Error)
}
