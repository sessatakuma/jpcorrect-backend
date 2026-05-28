package domain

import (
	"errors"
)

// AuthError represents an authentication/authorization error with HTTP status code
type AuthError struct {
	StatusCode int
	Message    string
	Details    string // Detailed error information (optional)
}

func (ae *AuthError) Error() string {
	if ae.Details != "" {
		return ae.Message + ": " + ae.Details
	}
	return ae.Message
}

func NewAuthError(statusCode int, message, details string) *AuthError {
	return &AuthError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
	}
}

var (
	ErrNotFound          = errors.New("record not found")
	ErrDuplicateEntry    = errors.New("duplicate entry")
	ErrHasRelatedRecords = errors.New("record has related records")
)
