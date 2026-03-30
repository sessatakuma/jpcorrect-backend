package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

func TestMapGormError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected error
	}{
		{
			name:     "nil error returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "gorm.ErrRecordNotFound returns domain.ErrNotFound",
			input:    gorm.ErrRecordNotFound,
			expected: domain.ErrNotFound,
		},
		{
			name:     "wrapped gorm.ErrRecordNotFound returns domain.ErrNotFound",
			input:    fmt.Errorf("wrap: %w", gorm.ErrRecordNotFound),
			expected: domain.ErrNotFound,
		},
		{
			name: "pgconn unique violation returns domain.ErrDuplicateEntry",
			input: &pgconn.PgError{
				Code:    "23505",
				Message: "duplicate key value violates unique constraint \"users_email_key\"",
			},
			expected: domain.ErrDuplicateEntry,
		},
		{
			name: "wrapped pgconn unique violation returns domain.ErrDuplicateEntry",
			input: fmt.Errorf("wrap: %w", &pgconn.PgError{
				Code:    "23505",
				Message: "duplicate key value violates unique constraint \"users_email_key\"",
			}),
			expected: domain.ErrDuplicateEntry,
		},
		{
			name: "pgconn foreign key violation returns domain.ErrHasRelatedRecords",
			input: &pgconn.PgError{
				Code:    "23503",
				Message: "insert or update on table violates foreign key constraint",
			},
			expected: domain.ErrHasRelatedRecords,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := MapGormError(tt.input)
			if tt.expected == nil {
				assert.Nil(t, actual, "expected nil")
			} else {
				assert.ErrorIs(t, actual, tt.expected, "expected %v, got %v", tt.expected, actual)
			}
		})
	}

	t.Run("unknown error returns original error", func(t *testing.T) {
		unknownErr := errors.New("some random error")
		actual := MapGormError(unknownErr)
		assert.Same(t, unknownErr, actual, "expected original error to be returned")
	})
}
