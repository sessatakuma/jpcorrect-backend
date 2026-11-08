package api

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
)

const (
	// defaultPageLimit is the §1.6 page size used when the request has no
	// limit.
	defaultPageLimit = 20
	// maxPageLimit is the §1.6 maximum page size.
	maxPageLimit = 100
)

// pageCursorTimeLayout is the timestamp format inside a cursor. It is fixed at
// six fractional digits because PostgreSQL's timestamptz stores microseconds:
// a value read back from the database always has at most six, so the encoded
// cursor round-trips exactly instead of being rounded to a different position.
// Cursors are always formatted in UTC ("Z"), so the same instant always encodes
// to the same string.
const pageCursorTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

// pageCursorPayload is the JSON body of a cursor before base64 encoding. §1.6
// fixes the two keys.
type pageCursorPayload struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

// pageQuery is the parsed ?limit=&cursor= pair of one paginated request.
//
// Cursor is nil for the first page; a non-nil cursor is a position in the
// (created_at DESC, id DESC) order handed to repository.PageAfter.
type pageQuery struct {
	Limit  int
	Cursor *repository.PageCursor
}

// pageResponse is the §1.6 list envelope shared by every paginated endpoint:
//
//	{"items": [...], "next_cursor": "<opaque>"|null, "has_more": bool}
//
// Items is always a JSON array: an empty result encodes as [] and never null.
// NextCursor is null exactly when HasMore is false.
type pageResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// parsePageQuery reads and validates ?limit= and ?cursor= from the request.
//
// Behaviour the spec leaves to the implementer, decided once here so every
// paginated endpoint behaves the same:
//
//   - no limit, or an empty limit, uses defaultPageLimit (20);
//   - a limit above maxPageLimit is capped at 100 instead of rejected, so a
//     client asking for "everything" still gets a bounded page rather than an
//     error;
//   - a limit that is not an integer, or is below 1, is a malformed request:
//     errorWithMessage(domain.ErrInvalidRequest, ...) renders the §1.5 envelope
//     with 400 INVALID_REQUEST;
//   - a cursor that is not valid unpadded URL-safe base64, not JSON, or carries
//     an unparsable created_at or id is malformed in the same way.
//
// Every error returned is already wrapped for writeError, so a handler only has
// to writeError(c, err) and return.
//
// The cursor is opaque to clients, not signed and not expiring: it carries a
// sort position only, so a tampered cursor can at worst select a different
// (still valid) page, and it grants no access beyond whatever filters the
// handler already applies.
func parsePageQuery(c *gin.Context) (pageQuery, error) {
	limit := defaultPageLimit

	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return pageQuery{}, errorWithMessage(domain.ErrInvalidRequest, "limit must be a positive integer")
		}
		limit = min(parsed, maxPageLimit)
	}

	query := pageQuery{Limit: limit}

	if raw := c.Query("cursor"); raw != "" {
		cursor, err := decodePageCursor(raw)
		if err != nil {
			return pageQuery{}, err
		}
		query.Cursor = cursor
	}

	return query, nil
}

// newPage turns the rows a repository.PageAfter query returned into the §1.6
// response.
//
// rows holds at most limit+1 rows because the scope asks for one extra: when
// it returns limit+1 rows the extra one is dropped and has_more becomes true,
// with next_cursor set to the position of the last row kept, so the next
// request continues exactly after it. With limit rows or fewer this is the last
// page: has_more is false and next_cursor is null.
//
// keyOf extracts the (created_at, id) pair of a row; it is the caller's because
// the helper is model-agnostic.
func newPage[T any](rows []T, limit int, keyOf func(T) repository.PageCursor) pageResponse[T] {
	items := rows
	hasMore := false

	if len(items) > limit {
		items = items[:limit]
		hasMore = true
	}

	// An empty result must serialise as [] and not null.
	if items == nil {
		items = make([]T, 0)
	}

	response := pageResponse[T]{Items: items, HasMore: hasMore}
	if hasMore {
		next := encodePageCursor(keyOf(items[len(items)-1]))
		response.NextCursor = &next
	}

	return response
}

// encodePageCursor renders the §1.6 cursor: base64(JSON{"created_at","id"}).
//
// The base64 alphabet is URL-safe and unpadded, because a cursor travels in a
// query string and the standard alphabet's "+" and "/" would need escaping on
// every round trip. The timestamp is UTC with microsecond precision.
func encodePageCursor(cursor repository.PageCursor) string {
	// json.Marshal cannot fail on a struct of two strings.
	payload, _ := json.Marshal(pageCursorPayload{
		CreatedAt: cursor.CreatedAt.UTC().Format(pageCursorTimeLayout),
		ID:        cursor.ID.String(),
	})

	return base64.RawURLEncoding.EncodeToString(payload)
}

// decodePageCursor reverses encodePageCursor. Every failure is reported as a
// malformed request through the §1.5 envelope; the distinct messages say which
// part was wrong without echoing attacker-controlled content back.
func decodePageCursor(raw string) (*repository.PageCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errorWithMessage(domain.ErrInvalidRequest, "cursor is not valid base64")
	}

	var decoded pageCursorPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, errorWithMessage(domain.ErrInvalidRequest, "cursor is not valid JSON")
	}

	createdAt, err := time.Parse(pageCursorTimeLayout, decoded.CreatedAt)
	if err != nil {
		return nil, errorWithMessage(domain.ErrInvalidRequest, "cursor has an invalid created_at")
	}

	id, err := uuid.Parse(decoded.ID)
	if err != nil {
		return nil, errorWithMessage(domain.ErrInvalidRequest, "cursor has an invalid id")
	}

	return &repository.PageCursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}
