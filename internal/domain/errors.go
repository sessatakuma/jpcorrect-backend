package domain

import (
	"errors"
	"net/http"
)

// AuthError represents an authentication/authorization error with HTTP status code
type AuthError struct {
	Err        error
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

var (
	ErrNotFound       = errors.New("record not found")
	ErrDuplicateEntry = errors.New("duplicate entry")

	// Auth errors
	ErrMissingAuthHeader = &AuthError{
		StatusCode: http.StatusUnauthorized,
		Message:    "missing authorization header",
	}
	ErrInvalidAuthHeader = &AuthError{
		StatusCode: http.StatusUnauthorized,
		Message:    "invalid authorization header format",
	}
	ErrInvalidToken = &AuthError{
		StatusCode: http.StatusUnauthorized,
		Message:    "invalid token",
	}
	ErrInvalidTokenClaims = &AuthError{
		StatusCode: http.StatusUnauthorized,
		Message:    "invalid token claims",
	}
	ErrJWKSNotInitialized = &AuthError{
		StatusCode: http.StatusInternalServerError,
		Message:    "JWKS not initialized",
	}
	ErrUnauthorized = &AuthError{
		StatusCode: http.StatusUnauthorized,
		Message:    "unauthorized",
	}
)
