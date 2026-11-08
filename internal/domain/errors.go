package domain

import (
	"errors"
)

// Sentinels in this file are the only source of client-visible error codes.
// internal/api/errors.go maps each one to an HTTP status and a code for the
// §1.5 error envelope; repositories translate persistence errors with
// MapGormError before they reach a handler.
var (
	// Retained pre-catalog sentinels (§1.5 "existing sentinels retained").

	ErrNotFound          = errors.New("record not found")
	ErrDuplicateEntry    = errors.New("duplicate entry")
	ErrHasRelatedRecords = errors.New("record has related records")

	// Business codes from the §1.5 catalog. A feature issue raises its own.

	ErrUserNotInitialized       = errors.New("user is not initialized")
	ErrGuildLimitReached        = errors.New("user has already created a guild")
	ErrGuildJoinLimitReached    = errors.New("user has already joined the maximum number of guilds")
	ErrAlreadyGuildMember       = errors.New("user is already a member of this guild")
	ErrPendingApplicationExists = errors.New("a pending join application already exists")
	ErrInviteInvalid            = errors.New("invite token is invalid or expired")
	ErrSelectionLimitReached    = errors.New("selection limit reached")
	ErrNoSelectedSentence       = errors.New("no sentence selected")
	ErrActivityNotInState       = errors.New("activity is not in the required state")
	ErrNotMistakeOwner          = errors.New("user is not the owner of this mistake")
	ErrIdempotencyKeyReused     = errors.New("idempotency key reused with a different payload")

	// Permission sentinels. §1.5 defines NOT_MISTAKE_OWNER for a 403 and leaves
	// the remaining permission failures to the generic FORBIDDEN code.

	ErrNotGuildMember = errors.New("user is not a member of this guild")
	ErrNotGuildMaster = errors.New("user is not the master of this guild")

	// Generic status sentinels. §1.5 leaves the code strings for statuses
	// outside the catalog to the implementer; the chosen set is documented
	// next to the mapping table in internal/api/errors.go.

	ErrInvalidRequest      = errors.New("invalid request")
	ErrUnauthorized        = errors.New("authentication required")
	ErrForbidden           = errors.New("permission denied")
	ErrRateLimited         = errors.New("too many requests")
	ErrUpstreamUnavailable = errors.New("upstream service unavailable")
	ErrInternal            = errors.New("internal server error")
)
