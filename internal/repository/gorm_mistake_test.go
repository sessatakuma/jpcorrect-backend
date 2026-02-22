package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"jpcorrect-backend/internal/domain"
)

func TestGormMistakeRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)
	mistakeID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE id = $1 ORDER BY "mistake"."id" LIMIT $2`)).
			WithArgs(mistakeID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "type", "origin_text", "fixed_text"}).
				AddRow(mistakeID, uuid.New(), uuid.New(), "grammar", "origin", "fixed"))

		mistake, err := repo.GetByID(context.Background(), mistakeID)

		assert.NoError(t, err)
		assert.NotNil(t, mistake)
		assert.Equal(t, mistakeID, mistake.ID)
	})
}

func TestGormMistakeRepository_GetByEventID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id"}).
				AddRow(uuid.New(), eventID).
				AddRow(uuid.New(), eventID))

		mistakes, err := repo.GetByEventID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.Len(t, mistakes, 2)
	})
}

func TestGormMistakeRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).
				AddRow(uuid.New(), userID))

		mistakes, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, mistakes, 1)
	})
}

func TestGormMistakeRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)

	t.Run("Success", func(t *testing.T) {
		mistake := &domain.Mistake{
			EventID:    uuid.New(),
			UserID:     uuid.New(),
			Type:       domain.MistakeTypeGrammar,
			OriginText: "origin",
			FixedText:  "fixed",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "mistake"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), mistake)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, mistake.ID)
	})
}

func TestGormMistakeRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)
	mistakeID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mistake := &domain.Mistake{
			ID:         mistakeID,
			OriginText: "updated",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "mistake"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), mistake)

		assert.NoError(t, err)
	})
}

func TestGormMistakeRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormMistakeRepository(db)
	mistakeID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "mistake" WHERE id = $1`)).
			WithArgs(mistakeID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), mistakeID)

		assert.NoError(t, err)
	})
}
