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

func TestGormJoinRequestRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE id = $1 ORDER BY "join_request"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}).
				AddRow(id, uuid.New(), uuid.New(), domain.JoinRequestStatusPending))

		req, err := repo.GetByID(context.Background(), id)

		assert.NoError(t, err)
		assert.NotNil(t, req)
		assert.Equal(t, id, req.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE id = $1 ORDER BY "join_request"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req, err := repo.GetByID(context.Background(), id)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, req)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE id = $1 ORDER BY "join_request"."id" LIMIT $2`)).
			WithArgs(id, 1).
			WillReturnError(fmt.Errorf("db error"))

		req, err := repo.GetByID(context.Background(), id)

		assert.Error(t, err)
		assert.Nil(t, req)
	})
}

func TestGormJoinRequestRepository_GetByGuildID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)
	guildID := uuid.New()

	t.Run("SuccessWithoutStatus", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}).
				AddRow(uuid.New(), guildID, uuid.New(), domain.JoinRequestStatusPending).
				AddRow(uuid.New(), guildID, uuid.New(), domain.JoinRequestStatusApproved))

		reqs, err := repo.GetByGuildID(context.Background(), guildID, nil)

		assert.NoError(t, err)
		assert.Len(t, reqs, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("SuccessWithStatus", func(t *testing.T) {
		status := domain.JoinRequestStatusPending
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE guild_id = $1 AND status = $2`)).
			WithArgs(guildID, status).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}).
				AddRow(uuid.New(), guildID, uuid.New(), domain.JoinRequestStatusPending))

		reqs, err := repo.GetByGuildID(context.Background(), guildID, &status)

		assert.NoError(t, err)
		assert.Len(t, reqs, 1)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}))

		reqs, err := repo.GetByGuildID(context.Background(), guildID, nil)

		assert.NoError(t, err)
		assert.Empty(t, reqs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE guild_id = $1`)).
			WithArgs(guildID).
			WillReturnError(fmt.Errorf("db error"))

		reqs, err := repo.GetByGuildID(context.Background(), guildID, nil)

		assert.Error(t, err)
		assert.Nil(t, reqs)
	})
}

func TestGormJoinRequestRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}).
				AddRow(uuid.New(), uuid.New(), userID, domain.JoinRequestStatusPending))

		reqs, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, reqs, 1)
		assert.Equal(t, userID, reqs[0].UserID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "user_id", "status"}))

		reqs, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Empty(t, reqs)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "join_request" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnError(fmt.Errorf("db error"))

		reqs, err := repo.GetByUserID(context.Background(), userID)

		assert.Error(t, err)
		assert.Nil(t, reqs)
	})
}

func TestGormJoinRequestRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)

	t.Run("Success", func(t *testing.T) {
		req := &domain.JoinRequest{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Status:  domain.JoinRequestStatusPending,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "join_request"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), req)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, req.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		req := &domain.JoinRequest{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Status:  domain.JoinRequestStatusPending,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "join_request"`)).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), req)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		req := &domain.JoinRequest{
			GuildID: uuid.New(),
			UserID:  uuid.New(),
			Status:  domain.JoinRequestStatusPending,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "join_request"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), req)

		assert.Error(t, err)
	})
}

func TestGormJoinRequestRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)
	id := uuid.New()

	t.Run("Success", func(t *testing.T) {
		req := &domain.JoinRequest{
			ID:     id,
			Status: domain.JoinRequestStatusApproved,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "join_request"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), req)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		req := &domain.JoinRequest{
			ID:     id,
			Status: domain.JoinRequestStatusApproved,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "join_request"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), req)

		assert.Error(t, err)
	})
}

func TestGormJoinRequestRepository_CancelPendingByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormJoinRequestRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "join_request" SET "status"=$1,"updated_at"=$2 WHERE user_id = $3 AND status = $4`)).
			WithArgs(domain.JoinRequestStatusCancelled, sqlmock.AnyArg(), userID, domain.JoinRequestStatusPending).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.CancelPendingByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "join_request" SET "status"=$1,"updated_at"=$2 WHERE user_id = $3 AND status = $4`)).
			WithArgs(domain.JoinRequestStatusCancelled, sqlmock.AnyArg(), userID, domain.JoinRequestStatusPending).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.CancelPendingByUserID(context.Background(), userID)

		assert.Error(t, err)
	})
}
