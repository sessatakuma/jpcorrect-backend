package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

func TestGormGuildRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
			WithArgs(guildID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "level"}).
				AddRow(guildID, "Test Guild", "A guild for testing", 0))

		guild, err := repo.GetByID(context.Background(), guildID)

		assert.NoError(t, err)
		assert.NotNil(t, guild)
		assert.Equal(t, guildID, guild.ID)
		assert.Equal(t, "Test Guild", guild.Name)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
			WithArgs(guildID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		guild, err := repo.GetByID(context.Background(), guildID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, guild)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 AND "guild"."deleted_at" IS NULL ORDER BY "guild"."id" LIMIT $2`)).
			WithArgs(guildID, 1).
			WillReturnError(fmt.Errorf("db error"))

		guild, err := repo.GetByID(context.Background(), guildID)

		assert.Error(t, err)
		assert.Nil(t, guild)
	})
}

func TestGormGuildRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)

	t.Run("Success", func(t *testing.T) {
		guild := &domain.Guild{
			Name:        "New Guild",
			Description: "A new guild",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), guild)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, guild.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		guild := &domain.Guild{
			Name:        "Duplicate Guild",
			Description: "A guild with duplicate name",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), guild)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		guild := &domain.Guild{
			Name:        "DB Error Guild",
			Description: "A guild that errors",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), guild)

		assert.Error(t, err)
	})
}

func TestGormGuildRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		guild := &domain.Guild{
			ID:          guildID,
			Name:        "Updated Guild",
			Description: "Updated description",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), guild)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		guild := &domain.Guild{
			ID:          guildID,
			Name:        "DB Error Guild",
			Description: "DB error description",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), guild)

		assert.Error(t, err)
	})
}

func TestGormGuildRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild" SET "deleted_at"=$1 WHERE id = $2 AND "guild"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), guildID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), guildID)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("HasRelatedRecords", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), guildID)

		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), guildID)

		assert.Error(t, err)
	})
}

func TestGormGuildAttendeeRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
				AddRow(id, uuid.New(), uuid.New(), domain.GuildAttendeeRoleMember))

		attendee, err := repo.GetByID(context.Background(), id)

		assert.NoError(t, err)
		assert.NotNil(t, attendee)
		assert.Equal(t, id, attendee.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		attendee, err := repo.GetByID(context.Background(), id)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, attendee)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(fmt.Errorf("db error"))

		attendee, err := repo.GetByID(context.Background(), id)

		assert.Error(t, err)
		assert.Nil(t, attendee)
	})
}

func TestGormGuildAttendeeRepository_GetByGuildID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
				AddRow(uuid.New(), guildID, uuid.New(), domain.GuildAttendeeRoleMember).
				AddRow(uuid.New(), guildID, uuid.New(), domain.GuildAttendeeRoleMaster))

		attendees, err := repo.GetByGuildID(context.Background(), guildID)

		assert.NoError(t, err)
		assert.Len(t, attendees, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}))

		attendees, err := repo.GetByGuildID(context.Background(), guildID)

		assert.NoError(t, err)
		assert.Empty(t, attendees)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnError(fmt.Errorf("db error"))

		attendees, err := repo.GetByGuildID(context.Background(), guildID)

		assert.Error(t, err)
		assert.Nil(t, attendees)
	})
}

func TestGormGuildAttendeeRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}).
				AddRow(uuid.New(), uuid.New(), userID, domain.GuildAttendeeRoleMember))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, attendees, 1)
		assert.Equal(t, userID, attendees[0].UserID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "role"}))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Empty(t, attendees)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("db error"))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, attendees)
	})
}

func TestGormGuildAttendeeRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)

	t.Run("Success", func(t *testing.T) {
		attendee := &domain.GuildAttendee{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.GuildAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), attendee)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, attendee.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		attendee := &domain.GuildAttendee{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.GuildAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), attendee)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		attendee := &domain.GuildAttendee{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.GuildAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "guild_attendee"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), attendee)

		assert.Error(t, err)
	})
}

func TestGormGuildAttendeeRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		attendee := &domain.GuildAttendee{
			ID:   id,
			Role: domain.GuildAttendeeRoleMaster,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild_attendee"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), attendee)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		attendee := &domain.GuildAttendee{
			ID:   id,
			Role: domain.GuildAttendeeRoleMaster,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "guild_attendee"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), attendee)

		assert.Error(t, err)
	})
}

func TestGormGuildAttendeeRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "guild_attendee" WHERE id = $1`)).
			WithArgs(id).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), id)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "guild_attendee" WHERE id = $1`)).
			WithArgs(id).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), id)

		assert.Error(t, err)
	})
}
