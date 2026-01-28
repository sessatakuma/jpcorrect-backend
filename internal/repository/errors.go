package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

// MapGormError maps GORM specific errors to domain errors.
func MapGormError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}

	// PostgreSQL unique violation usually contains "duplicate key" in the error message
	if strings.Contains(err.Error(), "duplicate key") {
		return domain.ErrDuplicateEntry
	}

	return err
}
