package repository

import (
	"errors"

	"github.com/jackc/pgconn"
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

	// PostgreSQL errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // Unique violation
			return domain.ErrDuplicateEntry
		case "23503": // Foreign key violation
			return domain.ErrHasRelatedRecords
		}
	}

	return err
}
