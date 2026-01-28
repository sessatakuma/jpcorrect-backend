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

func TestGormGuildRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 ORDER BY "guild"."id" LIMIT $2`)).
			WithArgs(guildID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description"}).
				AddRow(guildID, "Test Guild", "A test guild"))

		guild, err := repo.GetByID(context.Background(), guildID)

		assert.NoError(t, err)
		assert.NotNil(t, guild)
		assert.Equal(t, guildID, guild.ID)
		assert.Equal(t, "Test Guild", guild.Name)
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "guild" WHERE id = $1 ORDER BY "guild"."id" LIMIT $2`)).
			WithArgs(guildID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		guild, err := repo.GetByID(context.Background(), guildID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
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
	})
}

func TestGormGuildRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormGuildRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "guild" WHERE id = $1`)).
			WithArgs(guildID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), guildID)

		assert.NoError(t, err)
	})

	t.Run("RestrictAttendees", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "guild_attendee" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		err := repo.Delete(context.Background(), guildID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has attendees")
	})
}
