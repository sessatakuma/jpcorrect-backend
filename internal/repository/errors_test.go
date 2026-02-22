package repository

import (
	"errors"
	"testing"

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
			name:     "duplicate key error message returns domain.ErrDuplicateEntry",
			input:    errors.New("ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)"),
			expected: domain.ErrDuplicateEntry,
		},
		{
			name:     "unknown error returns original error",
			input:    errors.New("some random error"),
			expected: errors.New("some random error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := MapGormError(tt.input)
			if tt.expected == nil {
				if actual != nil {
					t.Errorf("expected nil, got %v", actual)
				}
			} else if tt.name == "unknown error returns original error" {
				if actual.Error() != tt.expected.Error() {
					t.Errorf("expected %v, got %v", tt.expected, actual)
				}
			} else {
				if !errors.Is(actual, tt.expected) {
					t.Errorf("expected %v, got %v", tt.expected, actual)
				}
			}
		})
	}
}
