package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

func TestGormReportThemeSuggestionRepository_GetByID(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormReportThemeSuggestionRepository(db)
	suggestionID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		title := "私の週末"
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion" WHERE id = $1 ORDER BY "report_theme_suggestion"."id" LIMIT $2`)).
			WithArgs(suggestionID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title", "description"}).
				AddRow(suggestionID, title, nil))

		suggestion, err := repo.GetByID(context.Background(), suggestionID)

		assert.NoError(t, err)
		assert.Equal(t, suggestionID, suggestion.ID)
		assert.Equal(t, title, suggestion.Title)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("NotFound", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion" WHERE id = $1 ORDER BY "report_theme_suggestion"."id" LIMIT $2`)).
			WithArgs(suggestionID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		suggestion, err := repo.GetByID(context.Background(), suggestionID)

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, suggestion)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion" WHERE id = $1 ORDER BY "report_theme_suggestion"."id" LIMIT $2`)).
			WithArgs(suggestionID, 1).
			WillReturnError(fmt.Errorf("db error"))

		suggestion, err := repo.GetByID(context.Background(), suggestionID)

		assert.Error(t, err)
		assert.Nil(t, suggestion)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGormReportThemeSuggestionRepository_List(t *testing.T) {
	db, mock := setupMockDB(t)
	repo := NewGormReportThemeSuggestionRepository(db)

	t.Run("Success", func(t *testing.T) {
		id1 := uuid.New()
		id2 := uuid.New()
		desc := "説明文"
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title", "description"}).
				AddRow(id1, "テーマ1", nil).
				AddRow(id2, "テーマ2", &desc))

		suggestions, err := repo.List(context.Background())

		assert.NoError(t, err)
		assert.Len(t, suggestions, 2)
		assert.Equal(t, id1, suggestions[0].ID)
		assert.Equal(t, id2, suggestions[1].ID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("EmptyResult", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "title", "description"}))

		suggestions, err := repo.List(context.Background())

		assert.NoError(t, err)
		assert.Empty(t, suggestions)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DBError", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "report_theme_suggestion"`)).
			WillReturnError(fmt.Errorf("db error"))

		suggestions, err := repo.List(context.Background())

		assert.Error(t, err)
		assert.Nil(t, suggestions)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
