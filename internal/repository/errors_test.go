package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "active master unique violation returns domain.ErrGuildLimitReached",
			input: &pgconn.PgError{
				Code:           "23505",
				ConstraintName: activeMasterIndex,
			},
			expected: domain.ErrGuildLimitReached,
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
		{
			name: "wrapped pgconn foreign key violation returns domain.ErrHasRelatedRecords",
			// GORM wraps driver errors, so the mapping must look through the
			// fmt.Errorf chain instead of type-asserting the top-level error.
			input: fmt.Errorf("insert guild_attendee: %w", &pgconn.PgError{
				Code:           "23503",
				ConstraintName: "fk_guild_attendee_guild",
				Message:        "insert or update on table violates foreign key constraint",
			}),
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

	// SQLSTATE codes other than 23505 and 23503 are outside the mapping's
	// contract. They must reach the caller untouched: wrapping them in a new
	// error would hide the driver detail that diagnostics rely on.
	t.Run("unknown pgconn SQLSTATE returns the original error", func(t *testing.T) {
		pgErr := &pgconn.PgError{
			Code:           "23514",
			ConstraintName: "some_check_constraint",
			Message:        "new row violates check constraint",
		}
		assert.Same(t, pgErr, MapGormError(pgErr),
			"an unhandled SQLSTATE must be passed through unchanged")
	})

	t.Run("wrapped unknown pgconn SQLSTATE returns the original wrapped error", func(t *testing.T) {
		pgErr := &pgconn.PgError{
			Code:    "40001",
			Message: "could not serialize access due to concurrent update",
		}
		wrapped := fmt.Errorf("commit transaction: %w", pgErr)

		actual := MapGormError(wrapped)
		assert.Same(t, wrapped, actual,
			"a wrapped unhandled SQLSTATE must be returned as-is, not re-wrapped")

		var mapped *pgconn.PgError
		require.True(t, errors.As(actual, &mapped), "expected *pgconn.PgError in the chain, got %T", actual)
		assert.Same(t, pgErr, mapped, "the driver error itself must not be replaced")
	})
}
