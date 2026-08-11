package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
		if attendee.ID == uuid.Nil {
			attendee.ID = uuid.New()
		}

		// Enforce the one-master-per-user limit atomically within this
		// transaction to close the check-then-act race.
		var masterCount int64
		err := tx.Model(&domain.GuildAttendee{}).
			Where("user_id = ? AND role = ? AND left_at IS NULL", attendee.UserID, domain.GuildAttendeeRoleMaster).
			Count(&masterCount).Error
		if err != nil {
			return err
		}
		if masterCount >= 1 {
			return domain.ErrGuildLimitReached
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

func (r *gormGuildRepository) TransferLeader(ctx context.Context, guildID uuid.UUID, callerID uuid.UUID, newLeaderUserID uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the guild row and verify it exists.
		var guild domain.Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", guildID).First(&guild).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}

		// Lock the caller's membership row and verify they are the current master.
		var caller domain.GuildAttendee
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("guild_id = ? AND user_id = ? AND left_at IS NULL", guildID, callerID).
			First(&caller).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotGuildMaster
		}
		if err != nil {
			return err
		}
		if caller.Role != domain.GuildAttendeeRoleMaster {
			return domain.ErrNotGuildMaster
		}

		// Verify the new leader is a current member.
		var newMaster domain.GuildAttendee
		if err := tx.Where("guild_id = ? AND user_id = ? AND left_at IS NULL", guildID, newLeaderUserID).
			First(&newMaster).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotGuildMember
		} else if err != nil {
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
	}))
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

func (r *gormGuildRepository) CreateInviteLinkWithTx(ctx context.Context, guildID uuid.UUID, newInvite *domain.GuildInvite, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update the `expires_at` timestamp for all of the guild's currently valid (unexpired) old links to `now`.
		err := tx.Model(&domain.GuildInvite{}).
			Where("guild_id = ?", guildID).
			Where("expires_at IS NULL OR expires_at > ?", now).
			Update("expires_at", now).Error
		if err != nil {
			return err
		}

		if err := tx.Create(newInvite).Error; err != nil {
			return err
		}

		return nil
	})
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

func (r *gormGuildAttendeeRepository) GetByGuildAndUser(ctx context.Context, guildID uuid.UUID, userID uuid.UUID) (*domain.GuildAttendee, error) {
	var attendee domain.GuildAttendee
	err := r.db.WithContext(ctx).
		Where("guild_id = ? AND user_id = ?", guildID, userID).
		First(&attendee).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &attendee, nil
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
