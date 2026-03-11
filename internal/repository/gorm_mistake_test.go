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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE id = $1 ORDER BY "mistake"."id" LIMIT $2`)).
			WithArgs(mistakeID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mistake, err := repo.GetByID(context.Background(), mistakeID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, mistake)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE id = $1 ORDER BY "mistake"."id" LIMIT $2`)).
			WithArgs(mistakeID, 1).
			WillReturnError(fmt.Errorf("db error"))

		result, err := repo.GetByID(context.Background(), mistakeID)

		assert.Error(t, err)
		assert.Nil(t, result)
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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id"}))

		mistakes, err := repo.GetByEventID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.Empty(t, mistakes)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnError(fmt.Errorf("db error"))

		result, err := repo.GetByEventID(context.Background(), eventID)

		assert.Error(t, err)
		assert.Nil(t, result)
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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}))

		mistakes, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Empty(t, mistakes)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "mistake" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("db error"))

		result, err := repo.GetByUserID(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, result)
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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		mistake := &domain.Mistake{
			EventID:    uuid.New(),
			UserID:     uuid.New(),
			Type:       domain.MistakeTypeGrammar,
			OriginText: "origin",
			FixedText:  "fixed",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "mistake"`)).
			WillReturnError(&pgconn.PgError{
				Code: "23505",
			})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), mistake)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mistake := &domain.Mistake{
			EventID:    uuid.New(),
			UserID:     uuid.New(),
			Type:       domain.MistakeTypeGrammar,
			OriginText: "origin",
			FixedText:  "fixed",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "mistake"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), mistake)

		assert.Error(t, err)
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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mistake := &domain.Mistake{
			ID:         mistakeID,
			OriginText: "updated",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "mistake"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), mistake)

		assert.Error(t, err)
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
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "mistake" WHERE id = $1`)).
			WithArgs(mistakeID).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), mistakeID)

		assert.Error(t, err)
	})
}
