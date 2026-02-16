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

// Implement the error interface
func (ae *AuthError) Error() string {
	if ae.Details != "" {
		return ae.Message + ": " + ae.Details
	}
	return ae.Message
}

// NewAuthError creates an AuthError with the given status code, message, and optional details
func NewAuthError(statusCode int, message, details string) *AuthError {
	return &AuthError{
		StatusCode: statusCode,
		Message:    message,
		Details:    details,
	}
}

var (
	ErrNotFound       = errors.New("record not found")
	ErrDuplicateEntry = errors.New("duplicate entry")
)
