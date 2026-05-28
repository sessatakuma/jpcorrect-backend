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

func TestGormActivityRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)
	activityID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE id = $1 AND "activity"."deleted_at" IS NULL ORDER BY "activity"."id" LIMIT $2`)).
			WithArgs(activityID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "sequence_number", "status", "mode"}).
				AddRow(activityID, uuid.New(), 1, domain.ActivityStatusPendingPractice, domain.ActivityModeReport))

		activity, err := repo.GetByID(context.Background(), activityID)

		assert.NoError(t, err)
		assert.NotNil(t, activity)
		assert.Equal(t, activityID, activity.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE id = $1 AND "activity"."deleted_at" IS NULL ORDER BY "activity"."id" LIMIT $2`)).
			WithArgs(activityID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		activity, err := repo.GetByID(context.Background(), activityID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, activity)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE id = $1 AND "activity"."deleted_at" IS NULL ORDER BY "activity"."id" LIMIT $2`)).
			WithArgs(activityID, 1).
			WillReturnError(fmt.Errorf("db error"))

		activity, err := repo.GetByID(context.Background(), activityID)

		assert.Error(t, err)
		assert.Nil(t, activity)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormActivityRepository_GetByGuildID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)
	guildID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE guild_id = $1 AND "activity"."deleted_at" IS NULL ORDER BY created_at DESC LIMIT $2`)).
			WithArgs(guildID, 10).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "sequence_number", "status", "mode"}).
				AddRow(uuid.New(), guildID, 1, domain.ActivityStatusPendingPractice, domain.ActivityModeReport).
				AddRow(uuid.New(), guildID, 2, domain.ActivityStatusDone, domain.ActivityModeConversation))

		activities, err := repo.GetByGuildID(context.Background(), guildID, 1, 10)

		assert.NoError(t, err)
		assert.Len(t, activities, 2)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE guild_id = $1 AND "activity"."deleted_at" IS NULL ORDER BY created_at DESC LIMIT $2`)).
			WithArgs(guildID, 10).
			WillReturnRows(sqlmock.NewRows([]string{"id", "guild_id", "sequence_number", "status", "mode"}))

		activities, err := repo.GetByGuildID(context.Background(), guildID, 1, 10)

		assert.NoError(t, err)
		assert.Empty(t, activities)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "activity" WHERE guild_id = $1 AND "activity"."deleted_at" IS NULL ORDER BY created_at DESC LIMIT $2`)).
			WithArgs(guildID, 10).
			WillReturnError(fmt.Errorf("db error"))

		activities, err := repo.GetByGuildID(context.Background(), guildID, 1, 10)

		assert.Error(t, err)
		assert.Nil(t, activities)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormActivityRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)

	t.Run("Success", func(t *testing.T) {
		activity := &domain.Activity{
			GuildID:        uuid.New(),
			SequenceNumber: 1,
			Status:         domain.ActivityStatusPendingPractice,
			Mode:           domain.ActivityModeReport,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "activity"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), activity)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, activity.ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DuplicateEntry", func(t *testing.T) {
		activity := &domain.Activity{
			GuildID:        uuid.New(),
			SequenceNumber: 1,
			Status:         domain.ActivityStatusPendingPractice,
			Mode:           domain.ActivityModeReport,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "activity"`)).
			WillReturnError(&pgconn.PgError{Code: "23505"})
		mock.ExpectRollback()

		err := repo.Create(context.Background(), activity)

		assert.ErrorIs(t, err, domain.ErrDuplicateEntry)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		activity := &domain.Activity{
			GuildID:        uuid.New(),
			SequenceNumber: 1,
			Status:         domain.ActivityStatusPendingPractice,
			Mode:           domain.ActivityModeReport,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "activity"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Create(context.Background(), activity)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormActivityRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)
	activityID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		activity := &domain.Activity{
			ID:             activityID,
			GuildID:        uuid.New(),
			SequenceNumber: 1,
			Status:         domain.ActivityStatusInPractice,
			Mode:           domain.ActivityModeReport,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), activity)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		activity := &domain.Activity{
			ID:             activityID,
			GuildID:        uuid.New(),
			SequenceNumber: 1,
			Status:         domain.ActivityStatusInPractice,
			Mode:           domain.ActivityModeReport,
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity"`)).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Update(context.Background(), activity)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormActivityRepository_UpdateStatus(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)
	activityID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity" SET "status"=$1,"updated_at"=$2 WHERE id = $3 AND "activity"."deleted_at" IS NULL`)).
			WithArgs(domain.ActivityStatusInPractice, sqlmock.AnyArg(), activityID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.UpdateStatus(context.Background(), activityID, domain.ActivityStatusInPractice)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity" SET "status"=$1,"updated_at"=$2 WHERE id = $3 AND "activity"."deleted_at" IS NULL`)).
			WithArgs(domain.ActivityStatusInPractice, sqlmock.AnyArg(), activityID).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.UpdateStatus(context.Background(), activityID, domain.ActivityStatusInPractice)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormActivityRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormActivityRepository(db)
	activityID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity" SET "deleted_at"=$1 WHERE id = $2 AND "activity"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), activityID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), activityID)

		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "activity" SET "deleted_at"=$1 WHERE id = $2 AND "activity"."deleted_at" IS NULL`)).
			WithArgs(sqlmock.AnyArg(), activityID).
			WillReturnError(fmt.Errorf("db error"))
		mock.ExpectRollback()

		err := repo.Delete(context.Background(), activityID)

		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
