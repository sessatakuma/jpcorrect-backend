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

func TestGormEventRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event" WHERE id = $1 AND "event"."deleted_at" IS NULL ORDER BY "event"."id" LIMIT $2`)).
			WithArgs(eventID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).
				AddRow(eventID, "Test Event"))

		event, err := repo.GetByID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.NotNil(t, event)
		assert.Equal(t, eventID, event.ID)
		assert.Equal(t, "Test Event", event.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event" WHERE id = $1 AND "event"."deleted_at" IS NULL ORDER BY "event"."id" LIMIT $2`)).
			WithArgs(eventID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		event, err := repo.GetByID(context.Background(), eventID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, event)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event" WHERE id = $1 AND "event"."deleted_at" IS NULL ORDER BY "event"."id" LIMIT $2`)).
			WithArgs(eventID, 1).
			WillReturnError(fmt.Errorf("db error"))

		event, err := repo.GetByID(context.Background(), eventID)

		assert.Error(t, err)
		assert.Nil(t, event)
	})
}

func TestGormEventRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT "event"."id","event"."title","event"."description","event"."start_time","event"."expected_duration","event"."actual_duration","event"."record_link","event"."mode","event"."note","event"."created_at","event"."updated_at","event"."deleted_at" FROM "event" JOIN event_attendee ON event_attendee.event_id = event.id WHERE event_attendee.user_id = $1 AND "event"."deleted_at" IS NULL`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}).
				AddRow(uuid.New(), "Event 1").
				AddRow(uuid.New(), "Event 2"))

		events, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, events, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT "event"."id","event"."title","event"."description","event"."start_time","event"."expected_duration","event"."actual_duration","event"."record_link","event"."mode","event"."note","event"."created_at","event"."updated_at","event"."deleted_at" FROM "event" JOIN event_attendee ON event_attendee.event_id = event.id WHERE event_attendee.user_id = $1 AND "event"."deleted_at" IS NULL`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title"}))

		events, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Empty(t, events)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT "event"."id","event"."title","event"."description","event"."start_time","event"."expected_duration","event"."actual_duration","event"."record_link","event"."mode","event"."note","event"."created_at","event"."updated_at","event"."deleted_at" FROM "event" JOIN event_attendee ON event_attendee.event_id = event.id WHERE event_attendee.user_id = $1 AND "event"."deleted_at" IS NULL`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("db error"))

		events, err := repo.GetByUserID(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, events)
	})
}

func TestGormEventRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventRepository(db)

	t.Run("Success", func(t *testing.T) {
		event := &domain.Event{
			Title: "New Event",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), event)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, event.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		event := &domain.Event{
			Title: "New Event",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event"`)).
			WillReturnError(&pgconn.PgError{
				Code: "23505",
			})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), event)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		event := &domain.Event{
			Title: "DB Error Event",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), event)

		assert.Error(t, err)
	})
}

func TestGormEventRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		event := &domain.Event{
			ID:    eventID,
			Title: "Updated Event",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "event"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), event)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		event := &domain.Event{
			ID:    eventID,
			Title: "DB Error Event",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "event"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), event)

		assert.Error(t, err)
	})
}

func TestGormEventRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "transcript" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "mistake" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "event" SET "deleted_at"=$1 WHERE id = $2 AND "event"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), eventID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), eventID)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RestrictAttendees", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), eventID)

		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RestrictTranscripts", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "transcript" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), eventID)

		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RestrictMistakes", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "transcript" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "mistake" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), eventID)

		assert.ErrorIs(t, err, domain.ErrHasRelatedRecords)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), eventID)

		assert.Error(t, err)
	})
}
