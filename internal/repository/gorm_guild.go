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

type guildApplicationRepository struct {
	db *gorm.DB
}

func NewGuildApplicationRepository(db *gorm.DB) domain.GuildApplicationRepository {
	return &guildApplicationRepository{db: db}
}

func (r *guildApplicationRepository) GetByID(ctx context.Context, appID uuid.UUID) (*domain.GuildApplication, error) {
	var app domain.GuildApplication
	err := r.db.WithContext(ctx).First(&app, "id = ?", appID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &app, nil
}

func (r *guildApplicationRepository) GetPendingByGuildAndUser(ctx context.Context, guildID, userID uuid.UUID) (*domain.GuildApplication, error) {
	var app domain.GuildApplication
	err := r.db.WithContext(ctx).
		Where("guild_id = ? AND user_id = ? AND status = ?", guildID, userID, domain.GuildApplicationStatusPending).
		First(&app).Error

	if err != nil {
		return nil, MapGormError(err)
	}
	return &app, nil
}

func (r *guildApplicationRepository) ListPendingByGuildID(ctx context.Context, guildID uuid.UUID) ([]*domain.GuildApplication, error) {
	var apps []*domain.GuildApplication
	err := r.db.WithContext(ctx).
		Where("guild_id = ? AND status = ?", guildID, domain.GuildApplicationStatusPending).
		Order("created_at ASC").
		Find(&apps).Error

	if err != nil {
		return nil, MapGormError(err)
	}
	return apps, nil
}

func (r *guildApplicationRepository) ApproveWithTx(ctx context.Context, app *domain.GuildApplication, newAttendee *domain.GuildAttendee) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		app.Status = domain.GuildApplicationStatusApproved
		app.UpdatedAt = now
		if err := tx.Save(app).Error; err != nil {
			return err
		}

		if newAttendee.ID == uuid.Nil {
			newAttendee.ID = uuid.New()
		}
		newAttendee.JoinedAt = &now
		if err := tx.Create(newAttendee).Error; err != nil {
			return err
		}

		// Reject all pending from this applicant under other guilds
		err := tx.Model(&domain.GuildApplication{}).
			Where("user_id = ? AND status = ? AND id != ?", app.UserID, domain.GuildApplicationStatusPending, app.ID).
			Updates(map[string]interface{}{
				"status":     domain.GuildApplicationStatusRejected,
				"updated_at": now,
			}).Error
		if err != nil {
			return err
		}

		return nil
	})
}

func (r *guildApplicationRepository) Create(ctx context.Context, app *domain.GuildApplication) error {
	if app.ID == uuid.Nil {
		app.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(app).Error)
}

func (r *guildApplicationRepository) Update(ctx context.Context, app *domain.GuildApplication) error {
	app.UpdatedAt = time.Now()
	return MapGormError(r.db.WithContext(ctx).Save(app).Error)
}

type guildDefaultSlotRepository struct {
	db *gorm.DB
}

func NewGuildDefaultSlotRepository(db *gorm.DB) domain.GuildDefaultSlotRepository {
	return &guildDefaultSlotRepository{db: db}
}

func (r *guildDefaultSlotRepository) GetByGuildID(ctx context.Context, guildID uuid.UUID) (*domain.GuildDefaultSlot, error) {
	var slot domain.GuildDefaultSlot
	err := r.db.WithContext(ctx).Where("guild_id = ?", guildID).First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &slot, nil
}

func (r *guildDefaultSlotRepository) DeleteByGuildID(ctx context.Context, guildID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("guild_id = ?", guildID).Delete(&domain.GuildDefaultSlot{})
	if res.Error != nil {
		return MapGormError(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *guildDefaultSlotRepository) Upsert(ctx context.Context, slot *domain.GuildDefaultSlot) error {
	if slot.ID == uuid.Nil {
		slot.ID = uuid.New()
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "guild_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"day_of_week", "start_time", "end_time", "updated_at"}),
	}).Create(slot).Error

	return MapGormError(err)
}
