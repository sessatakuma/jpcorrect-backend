package domain

import "errors"

var (
	ErrNotFound       = errors.New("record not found")
	ErrDuplicateEntry = errors.New("duplicate entry")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
)
