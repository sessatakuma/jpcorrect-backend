package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the single error envelope for every error the backend
// produces itself (§1.5). It is exported so the Swagger annotations can refer
// to it as api.ErrorResponse.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody carries the catalog code, a human-readable message and an optional
// structured payload. Details is always a JSON object so clients never have to
// branch on its type; every code in the current catalog leaves it empty.
type ErrorBody struct {
	Code    string         `json:"code" example:"NOT_FOUND" description:"Stable machine-readable error code"`
	Message string         `json:"message" example:"record not found" description:"Human-readable reason"`
	Details map[string]any `json:"details" description:"Structured payload; an empty object for the current catalog"`
}

// errorMapping is one row of the code table: the sentinel a repository,
// middleware or handler returns, the HTTP status §1.5 requires for it, and the
// code string the envelope carries.
type errorMapping struct {
	sentinel error
	status   int
	code     string
}

// errorMappings is the one place that turns an error into a response. Order
// only matters for sentinels that could wrap each other; the current set is
// flat.
//
// Code set: §1.5 holds the 11 business codes and the nine generic codes #70
// chose (Appendix E row 31). This table is the one place that maps them:
//
//	400 INVALID_REQUEST        malformed path parameter, query or request body
//	401 UNAUTHORIZED           missing, malformed or invalid credentials
//	403 FORBIDDEN              authenticated caller without the permission,
//	                           including a non-member or non-leader of a guild
//	404 NOT_FOUND              ErrNotFound
//	409 CONFLICT               ErrDuplicateEntry
//	409 RELATED_RECORDS_EXIST  ErrHasRelatedRecords
//	429 RATE_LIMITED           request throttled
//	500 INTERNAL_ERROR         unmapped or internal failure
//	502 UPSTREAM_UNAVAILABLE   api-tools proxy could not reach its upstream
//
// Nothing outside this list and the §1.5 catalog is ever emitted, and a new
// business code is added only when §1.5 gains one.
var errorMappings = []errorMapping{
	// 400
	{domain.ErrInvalidRequest, http.StatusBadRequest, "INVALID_REQUEST"},

	// 401
	{domain.ErrUserNotInitialized, http.StatusUnauthorized, "USER_NOT_INITIALIZED"},
	{domain.ErrUnauthorized, http.StatusUnauthorized, "UNAUTHORIZED"},

	// 403
	{domain.ErrNotMistakeOwner, http.StatusForbidden, "NOT_MISTAKE_OWNER"},
	{domain.ErrForbidden, http.StatusForbidden, "FORBIDDEN"},
	{domain.ErrNotGuildMember, http.StatusForbidden, "FORBIDDEN"},
	{domain.ErrNotGuildMaster, http.StatusForbidden, "FORBIDDEN"},

	// 404
	{domain.ErrInviteInvalid, http.StatusNotFound, "INVITE_INVALID"},
	{domain.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},

	// 409
	{domain.ErrGuildLimitReached, http.StatusConflict, "GUILD_LIMIT_REACHED"},
	{domain.ErrGuildJoinLimitReached, http.StatusConflict, "GUILD_JOIN_LIMIT_REACHED"},
	{domain.ErrAlreadyGuildMember, http.StatusConflict, "ALREADY_GUILD_MEMBER"},
	{domain.ErrPendingApplicationExists, http.StatusConflict, "PENDING_APPLICATION_EXISTS"},
	{domain.ErrSelectionLimitReached, http.StatusConflict, "SELECTION_LIMIT_REACHED"},
	{domain.ErrNoSelectedSentence, http.StatusConflict, "NO_SELECTED_SENTENCE"},
	{domain.ErrActivityNotInState, http.StatusConflict, "ACTIVITY_NOT_IN_STATE"},
	{domain.ErrIdempotencyKeyReused, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED"},
	{domain.ErrDuplicateEntry, http.StatusConflict, "CONFLICT"},
	{domain.ErrHasRelatedRecords, http.StatusConflict, "RELATED_RECORDS_EXIST"},

	// 429
	{domain.ErrRateLimited, http.StatusTooManyRequests, "RATE_LIMITED"},

	// 500
	{domain.ErrInternal, http.StatusInternalServerError, "INTERNAL_ERROR"},

	// 502
	{domain.ErrUpstreamUnavailable, http.StatusBadGateway, "UPSTREAM_UNAVAILABLE"},
}

// codedError binds a catalog sentinel to the message a client should read.
// It keeps Unwrap, so wrapping or unwrapping it never defeats errors.Is.
type codedError struct {
	sentinel error
	message  string
}

func (e *codedError) Error() string { return e.message }

func (e *codedError) Unwrap() error { return e.sentinel }

// errorWithMessage pairs sentinel with the message the client should read
// instead of the sentinel's own text, e.g. a resource-specific "guild not
// found" for ErrNotFound. Status and code still come from the mapping table.
func errorWithMessage(sentinel error, message string) error {
	return &codedError{sentinel: sentinel, message: message}
}

// resolveError maps err to its catalog entry. It always uses errors.Is, so a
// sentinel wrapped by a repository or middleware resolves the same way as a
// bare one. Anything unmapped becomes the generic 500: the internal error text
// is never echoed, it stays in the logs of the layer that wrapped it.
func resolveError(err error) (status int, body ErrorBody) {
	var coded *codedError

	for _, m := range errorMappings {
		if !errors.Is(err, m.sentinel) {
			continue
		}

		message := m.sentinel.Error()
		if errors.As(err, &coded) && coded.message != "" {
			message = coded.message
		}

		return m.status, ErrorBody{
			Code:    m.code,
			Message: message,
			Details: map[string]any{},
		}
	}

	return http.StatusInternalServerError, ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: domain.ErrInternal.Error(),
		Details: map[string]any{},
	}
}

// writeError writes the §1.5 envelope for err. Every handler and middleware
// uses it for errors the backend produces; only upstream api-tools bodies are
// passed through unchanged.
func writeError(c *gin.Context, err error) {
	status, body := resolveError(err)
	c.JSON(status, ErrorResponse{Error: body})
}

// writeErrorRaw writes the same envelope without gin, for callers that own the
// raw http.ResponseWriter (the api-tools reverse proxy error handler).
func writeErrorRaw(w http.ResponseWriter, err error) {
	status, body := resolveError(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: body})
}
