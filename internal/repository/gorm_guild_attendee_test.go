package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

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
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild_attendee" WHERE id = $1 ORDER BY "guild_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		attendee, err := repo.GetByID(context.Background(), id)

		assert.ErrorIs(t, err, domain.ErrNotFound)
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
	})
}
