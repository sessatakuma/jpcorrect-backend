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

// lockLeaderQuota enforces the leader quota of spec §3.3.1: a user may lead at
// most one guild that still exists.
//
// The check must run inside the write transaction, after locking the user row:
// the row lock serialises two concurrent requests for the same user, so the
// second one counts the guild the first one committed. A plain SELECT COUNT
// would let both callers through.
//
// Guild carries a soft delete, so the guild_attendee row of a leader whose guild
// was deleted survives with role = 'master' and left_at IS NULL. The join skips
// those rows, which is why the quota cannot be a partial unique index on
// guild_attendee: an index sees the membership row, not the deleted guild.
//
// excludeAttendeeID skips one attendee row in the count. Callers that promote an
// existing row pass its id, so the row cannot count itself.
func lockLeaderQuota(tx *gorm.DB, userID uuid.UUID, excludeAttendeeID uuid.UUID) error {
	var user domain.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		Where("id = ?", userID).
		First(&user).Error; err != nil {
		return err
	}

	led := tx.Model(&domain.GuildAttendee{}).
		Joins("JOIN guild ON guild.id = guild_attendee.guild_id AND guild.deleted_at IS NULL").
		Where("guild_attendee.user_id = ?", userID).
		Where("guild_attendee.role = ?", domain.GuildAttendeeRoleMaster).
		Where("guild_attendee.left_at IS NULL")
	if excludeAttendeeID != uuid.Nil {
		led = led.Where("guild_attendee.id <> ?", excludeAttendeeID)
	}

	var count int64
	if err := led.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return domain.ErrGuildLimitReached
	}
	return nil
}

func (r *gormGuildRepository) CreateWithMaster(ctx context.Context, guild *domain.Guild, attendee *domain.GuildAttendee) error {
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if guild.ID == uuid.Nil {
			guild.ID = uuid.New()
		}
		if attendee.ID == uuid.Nil {
			attendee.ID = uuid.New()
		}

		if err := lockLeaderQuota(tx, attendee.UserID, uuid.Nil); err != nil {
			return err
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

		// Omit associations: GuildAttendee.Guild/User exist only to emit the
		// foreign keys and must never make a write touch the parent tables.
		if err := tx.Omit(clause.Associations).Create(attendee).Error; err != nil {
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

		// The new leader must not already lead another guild. The row lock and
		// the count are the database half of the leader quota (§3.3.1).
		if err := lockLeaderQuota(tx, newLeaderUserID, newMaster.ID); err != nil {
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

func (r *gormGuildRepository) GetActiveInviteByGuildID(ctx context.Context, guildID uuid.UUID, now time.Time) (*domain.InviteLink, error) {
	var invite domain.InviteLink

	err := r.db.WithContext(ctx).
		Where("guild_id = ?", guildID).
		Where("expires_at > ?", now).
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

func (r *gormGuildRepository) CreateInviteLinkWithTx(ctx context.Context, guildID uuid.UUID, newInvite *domain.InviteLink, now time.Time) error {
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize rotations for this guild, including when it has no invites yet.
		var guild domain.Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", guildID).First(&guild).Error; err != nil {
			return err
		}

		// Update the `expires_at` timestamp for all of the guild's currently valid (unexpired) old links to `now`.
		err := tx.Model(&domain.InviteLink{}).
			Where("guild_id = ?", guildID).
			Where("expires_at > ?", now).
			Update("expires_at", now).Error
		if err != nil {
			return err
		}

		// Omit associations: InviteLink.Guild/CreatedByUser exist only to emit the
		// foreign keys and must never make a write touch the parent tables.
		if err := tx.Omit(clause.Associations).Create(newInvite).Error; err != nil {
			return err
		}

		return nil
	}))
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
		Where("guild_id = ? AND user_id = ? AND left_at IS NULL", guildID, userID).
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
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if isActiveMaster(attendee) {
			if err := lockLeaderQuota(tx, attendee.UserID, uuid.Nil); err != nil {
				return err
			}
		}
		// Omit associations: see CreateWithMaster.
		return tx.Omit(clause.Associations).Create(attendee).Error
	}))
}

func (r *gormGuildAttendeeRepository) Update(ctx context.Context, attendee *domain.GuildAttendee) error {
	return MapGormError(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if isActiveMaster(attendee) {
			// Skip the row itself: it may already carry role = master.
			if err := lockLeaderQuota(tx, attendee.UserID, attendee.ID); err != nil {
				return err
			}
		}
		// Omit associations: see CreateWithMaster.
		return tx.Omit(clause.Associations).Save(attendee).Error
	}))
}

// isActiveMaster reports whether attendee is, or is about to become, the active
// leader of a guild. Only those writes are subject to the leader quota.
func isActiveMaster(attendee *domain.GuildAttendee) bool {
	return attendee.Role == domain.GuildAttendeeRoleMaster && attendee.LeftAt == nil
}

func (r *gormGuildAttendeeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.GuildAttendee{}, "id = ?", id).Error)
}
