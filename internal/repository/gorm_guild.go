package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormGuildRepository struct {
	db *gorm.DB
}

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

func (r *gormGuildRepository) CountMasterGuildsByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.GuildAttendee{}).
		Where("user_id = ? AND role = ? AND left_at IS NULL", userID, domain.GuildAttendeeRoleMaster).
		Count(&count).Error

	if err != nil {
		return 0, MapGormError(err)
	}
	return count, nil
}

func (r *gormGuildRepository) CreateWithMaster(ctx context.Context, guild *domain.Guild, attendee *domain.GuildAttendee) error {
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if guild.ID == uuid.Nil {
			guild.ID = uuid.New()
		}
		if err := tx.Create(guild).Error; err != nil {
			return err
		}

		attendee.GuildID = guild.ID
		attendee.Role = domain.GuildAttendeeRoleMaster
		now := time.Now()
		if attendee.JoinedAt == nil {
			attendee.JoinedAt = &now
		}

		if err := tx.Create(attendee).Error; err != nil {
			return err
		}

		return nil
	}))
}

func (r *gormGuildRepository) TransferLeader(ctx context.Context, guildID uuid.UUID, newLeaderUserID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check whether the new master is the guild,s member
		var newMaster domain.GuildAttendee
		err := tx.Where("guild_id = ? AND user_id = ?", guildID, newLeaderUserID).
			First(&newMaster).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotGuildMember
			}
			return err
		}

		// Downgrade the original master to member.
		err = tx.Model(&domain.GuildAttendee{}).
			Where("guild_id = ? AND role = ?", guildID, domain.GuildAttendeeRoleMaster).
			Update("role", domain.GuildAttendeeRoleMember).Error
		if err != nil {
			return err
		}

		// Promote the new leader to master.
		err = tx.Model(&domain.GuildAttendee{}).
			Where("guild_id = ? AND user_id = ?", guildID, newLeaderUserID).
			Update("role", domain.GuildAttendeeRoleMaster).Error
		if err != nil {
			return err
		}

		// Update the updated_at timestamp
		err = tx.Model(&domain.Guild{}).
			Where("id = ?", guildID).
			Update("updated_at", time.Now()).Error
		if err != nil {
			return err
		}

		return nil
	})
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attendeeCount int64
		err := tx.Model(&domain.GuildAttendee{}).Where("guild_id = ?", guildID).Count(&attendeeCount).Error
		if err != nil {
			return MapGormError(err)
		}
		if attendeeCount > 0 {
			return domain.ErrHasRelatedRecords
		}
		return MapGormError(tx.Delete(&domain.Guild{}, "id = ?", guildID).Error)
	})
}

func (r *gormGuildRepository) GetActiveInviteByGuildID(ctx context.Context, guildID uuid.UUID, now time.Time) (*domain.GuildInvite, error) {
	var invite domain.GuildInvite

	err := r.db.WithContext(ctx).
		Where("guild_id = ?", guildID).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Order("created_at DESC").
		First(&invite).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &invite, nil
}

type gormGuildAttendeeRepository struct {
	db *gorm.DB
}

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
