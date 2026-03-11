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

func TestGormEventAttendeeRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE id = $1 ORDER BY "event_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "role"}).
				AddRow(id, uuid.New(), uuid.New(), domain.EventAttendeeRoleMember))

		attendee, err := repo.GetByID(context.Background(), id)

		assert.NoError(t, err)
		assert.NotNil(t, attendee)
		assert.Equal(t, id, attendee.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE id = $1 ORDER BY "event_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		attendee, err := repo.GetByID(context.Background(), id)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, attendee)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE id = $1 ORDER BY "event_attendee"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(fmt.Errorf("db error"))

		attendee, err := repo.GetByID(context.Background(), id)

		assert.Error(t, err)
		assert.Nil(t, attendee)
	})
}

func TestGormEventAttendeeRepository_GetByEventID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "role"}).
				AddRow(uuid.New(), eventID, uuid.New(), domain.EventAttendeeRoleMember).
				AddRow(uuid.New(), eventID, uuid.New(), domain.EventAttendeeRoleEmcee))

		attendees, err := repo.GetByEventID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.Len(t, attendees, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "role"}))

		attendees, err := repo.GetByEventID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.Empty(t, attendees)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnError(fmt.Errorf("db error"))

		attendees, err := repo.GetByEventID(context.Background(), eventID)

		assert.Error(t, err)
		assert.Nil(t, attendees)
	})
}

func TestGormEventAttendeeRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "role"}).
				AddRow(uuid.New(), uuid.New(), userID, domain.EventAttendeeRoleMember))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, attendees, 1)
		assert.Equal(t, userID, attendees[0].UserID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "user_id", "role"}))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Empty(t, attendees)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "event_attendee" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("db error"))

		attendees, err := repo.GetByUserID(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, attendees)
	})
}

func TestGormEventAttendeeRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)

	t.Run("Success", func(t *testing.T) {
		attendee := &domain.EventAttendee{
			EventID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.EventAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event_attendee"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), attendee)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, attendee.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		attendee := &domain.EventAttendee{
			EventID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.EventAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event_attendee"`)).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), attendee)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		attendee := &domain.EventAttendee{
			EventID: uuid.New(),
			UserID:  uuid.New(),
			Role:    domain.EventAttendeeRoleMember,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "event_attendee"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), attendee)

		assert.Error(t, err)
	})
}

func TestGormEventAttendeeRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		attendee := &domain.EventAttendee{
			ID:   id,
			Role: domain.EventAttendeeRoleEmcee,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "event_attendee"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), attendee)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		attendee := &domain.EventAttendee{
			ID:   id,
			Role: domain.EventAttendeeRoleEmcee,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "event_attendee"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), attendee)

		assert.Error(t, err)
	})
}

func TestGormEventAttendeeRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormEventAttendeeRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "event_attendee" WHERE id = $1`)).
			WithArgs(id).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), id)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "event_attendee" WHERE id = $1`)).
			WithArgs(id).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), id)

		assert.Error(t, err)
	})
}
