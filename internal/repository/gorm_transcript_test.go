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

func TestGormTranscriptRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)
	transcriptID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transcript" WHERE id = $1 ORDER BY "transcript"."id" LIMIT $2`)).
			WithArgs(transcriptID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "transcript"}).
				AddRow(transcriptID, "test transcript"))

		transcript, err := repo.GetByID(context.Background(), transcriptID)

		assert.NoError(t, err)
		if transcript != nil {
			assert.Equal(t, transcriptID, transcript.ID)
			assert.Equal(t, "test transcript", transcript.Transcript)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transcript" WHERE id = $1 ORDER BY "transcript"."id" LIMIT $2`)).
			WithArgs(transcriptID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		transcript, err := repo.GetByID(context.Background(), transcriptID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, transcript)
	})
}

func TestGormTranscriptRepository_GetByEventID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)
	eventID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transcript" WHERE event_id = $1`)).
			WithArgs(eventID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "transcript"}).
				AddRow(uuid.New(), eventID, "transcript 1").
				AddRow(uuid.New(), eventID, "transcript 2"))

		transcripts, err := repo.GetByEventID(context.Background(), eventID)

		assert.NoError(t, err)
		assert.Len(t, transcripts, 2)
		assert.Equal(t, eventID, transcripts[0].EventID)
	})
}

func TestGormTranscriptRepository_GetByUserID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transcript" WHERE user_id = $1`)).
			WithArgs(userID).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "transcript"}).
				AddRow(uuid.New(), userID, "transcript 1"))

		transcripts, err := repo.GetByUserID(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, transcripts, 1)
		assert.Equal(t, userID, transcripts[0].UserID)
	})
}

func TestGormTranscriptRepository_Create(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)

	t.Run("Success", func(t *testing.T) {
		transcript := &domain.Transcript{
			EventID:    uuid.New(),
			UserID:     uuid.New(),
			Transcript: "new transcript",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "transcript"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Create(context.Background(), transcript)

		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, transcript.ID)
	})
}

func TestGormTranscriptRepository_Update(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)
	transcriptID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		transcript := &domain.Transcript{
			ID:         transcriptID,
			Transcript: "updated transcript",
		}

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "transcript"`)).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Update(context.Background(), transcript)

		assert.NoError(t, err)
	})
}

func TestGormTranscriptRepository_Delete(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormTranscriptRepository(db)
	transcriptID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "transcript" WHERE id = $1`)).
			WithArgs(transcriptID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := repo.Delete(context.Background(), transcriptID)

		assert.NoError(t, err)
	})
}
