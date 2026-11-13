package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
)

// newPageQueryContext builds a gin context for the given raw query string, so
// parsePageQuery can be exercised without a router.
func newPageQueryContext(t *testing.T, rawQuery string) *gin.Context {
	t.Helper()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/things"+rawQuery, nil)
	return c
}

// feedPostKey is the (created_at, id) extractor a feed handler would pass to
// newPage.
func feedPostKey(post domain.FeedPost) repository.PageCursor {
	return repository.PageCursor{CreatedAt: post.CreatedAt, ID: post.ID}
}

// requireInvalidRequest asserts that err is a malformed-request error and that
// the §1.5 envelope renders it as 400 INVALID_REQUEST.
func requireInvalidRequest(t *testing.T, err error, message string) {
	t.Helper()

	require.Error(t, err, message)
	require.ErrorIs(t, err, domain.ErrInvalidRequest, message)

	status, body := resolveError(err)
	assert.Equal(t, http.StatusBadRequest, status, message)
	assert.Equal(t, "INVALID_REQUEST", body.Code, message)
	assert.NotEmpty(t, body.Message, message)
	assert.Empty(t, body.Details, message)
}

// TestParsePageQueryLimit pins the §1.6 limit contract, including the decision
// this implementation made for a limit above the maximum: it is capped at 100
// instead of rejected, so an over-eager client still gets a bounded page.
func TestParsePageQueryLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		query   string
		want    int
		wantErr bool
	}{
		{name: "omitted defaults to 20", query: "", want: defaultPageLimit},
		{name: "empty value defaults to 20", query: "?limit=", want: defaultPageLimit},
		{name: "one is the smallest page", query: "?limit=1", want: 1},
		{name: "explicit value", query: "?limit=37", want: 37},
		{name: "maximum is accepted as is", query: "?limit=100", want: maxPageLimit},
		{name: "one above the maximum is capped", query: "?limit=101", want: maxPageLimit},
		{name: "far above the maximum is capped", query: "?limit=1000000", want: maxPageLimit},
		{name: "zero is malformed", query: "?limit=0", wantErr: true},
		{name: "negative is malformed", query: "?limit=-1", wantErr: true},
		{name: "not a number is malformed", query: "?limit=abc", wantErr: true},
		{name: "fractional is malformed", query: "?limit=1.5", wantErr: true},
		{name: "out of range is malformed", query: "?limit=99999999999999999999", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, err := parsePageQuery(newPageQueryContext(t, tc.query))

			if tc.wantErr {
				requireInvalidRequest(t, err, tc.query)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, query.Limit)
			assert.Nil(t, query.Cursor, "a request without a cursor starts at the first page")
		})
	}
}

// TestParsePageQueryCursor proves a cursor the server produced parses back into
// the same position, with microsecond precision.
func TestParsePageQueryCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	id := uuid.MustParse("8b0d3a3e-1f6f-4f0e-9a3e-2b7c1d4e5f60")
	createdAt := time.Date(2026, 10, 19, 12, 34, 56, 123456000, time.UTC)

	query, err := parsePageQuery(newPageQueryContext(t, "?limit=5&cursor="+encodePageCursor(repository.PageCursor{
		CreatedAt: createdAt,
		ID:        id,
	})))
	require.NoError(t, err)

	assert.Equal(t, 5, query.Limit)
	require.NotNil(t, query.Cursor)
	assert.True(t, createdAt.Equal(query.Cursor.CreatedAt), "created_at must survive the round trip")
	assert.Equal(t, 123456000, query.Cursor.CreatedAt.Nanosecond(), "microseconds must not be truncated")
	assert.Equal(t, id, query.Cursor.ID)
}

// TestParsePageQueryMalformedCursor covers every way a cursor can be broken:
// bad base64, bad JSON, a bad timestamp and a bad UUID. All of them are the
// same 400 INVALID_REQUEST the §1.5 envelope defines.
func TestParsePageQueryMalformedCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	id := uuid.New()
	createdAt := time.Date(2026, 10, 19, 12, 34, 56, 0, time.UTC)
	valid := encodePageCursor(repository.PageCursor{CreatedAt: createdAt, ID: id})

	// cursorWith encodes an arbitrary JSON payload the way the server would.
	cursorWith := func(payload string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(payload))
	}

	tests := []struct {
		name            string
		cursor          string
		wantMessagePart string
	}{
		{
			name:            "not base64",
			cursor:          "not//base64!!",
			wantMessagePart: "base64",
		},
		{
			// Only the canonical unpadded URL-safe form is accepted; the
			// standard alphabet and padding are not, because the server never
			// emits them and a client echoes the cursor back unchanged.
			name:            "padded standard-alphabet base64",
			cursor:          base64.StdEncoding.EncodeToString([]byte(`{"created_at":"x"`)),
			wantMessagePart: "base64",
		},
		{
			name:            "not JSON",
			cursor:          cursorWith("created_at=2026-10-19"),
			wantMessagePart: "JSON",
		},
		{
			name:            "JSON array instead of an object",
			cursor:          cursorWith(`["2026-10-19T12:34:56.000000Z"]`),
			wantMessagePart: "JSON",
		},
		{
			name:            "missing created_at",
			cursor:          cursorWith(`{"id":"` + id.String() + `"}`),
			wantMessagePart: "created_at",
		},
		{
			name:            "created_at without microseconds",
			cursor:          cursorWith(`{"created_at":"2026-10-19T12:34:56Z","id":"` + id.String() + `"}`),
			wantMessagePart: "created_at",
		},
		{
			name:            "created_at with nanoseconds",
			cursor:          cursorWith(`{"created_at":"2026-10-19T12:34:56.123456789Z","id":"` + id.String() + `"}`),
			wantMessagePart: "created_at",
		},
		{
			name:            "created_at that is not a timestamp",
			cursor:          cursorWith(`{"created_at":"yesterday","id":"` + id.String() + `"}`),
			wantMessagePart: "created_at",
		},
		{
			name:            "missing id",
			cursor:          cursorWith(`{"created_at":"2026-10-19T12:34:56.000000Z"}`),
			wantMessagePart: "id",
		},
		{
			name:            "id that is not a UUID",
			cursor:          cursorWith(`{"created_at":"2026-10-19T12:34:56.000000Z","id":"42"}`),
			wantMessagePart: "id",
		},
		{
			name:            "id is not a string",
			cursor:          cursorWith(`{"created_at":"2026-10-19T12:34:56.000000Z","id":42}`),
			wantMessagePart: "JSON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, err := parsePageQuery(newPageQueryContext(t, "?cursor="+tc.cursor))

			requireInvalidRequest(t, err, tc.cursor)
			assert.Nil(t, query.Cursor)
			if tc.wantMessagePart != "" {
				_, body := resolveError(err)
				assert.Contains(t, body.Message, tc.wantMessagePart)
			}
		})
	}

	// The control case: the cursor the server itself produced is accepted, so
	// the failures above are about the payload and not about the prefix.
	query, err := parsePageQuery(newPageQueryContext(t, "?cursor="+valid))
	require.NoError(t, err)
	require.NotNil(t, query.Cursor)
	assert.Equal(t, id, query.Cursor.ID)
}

// TestPageCursorEncodingIsOpaqueAndCanonical pins the wire form: URL-safe
// unpadded base64 over the JSON object §1.6 defines, UTC with microseconds.
func TestPageCursorEncodingIsOpaqueAndCanonical(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	// 03:04:05.987654 JST is 18:04:05.987654 UTC the previous day.
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 987654000, jst)
	id := uuid.MustParse("0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0")

	encoded := encodePageCursor(repository.PageCursor{CreatedAt: createdAt, ID: id})

	assert.NotContains(t, encoded, id.String(), "the cursor is opaque, not a readable id")
	assert.NotContains(t, encoded, "created_at", "the cursor is opaque, not readable JSON")

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	require.NoError(t, err, "the cursor uses the unpadded URL-safe alphabet")

	var decoded pageCursorPayload
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, "2026-01-01T18:04:05.987654Z", decoded.CreatedAt, "UTC with microsecond precision")
	assert.Equal(t, id.String(), decoded.ID)

	cursor, err := decodePageCursor(encoded)
	require.NoError(t, err)
	assert.True(t, createdAt.Equal(cursor.CreatedAt), "the same instant, independent of the input zone")
	assert.Equal(t, time.UTC, cursor.CreatedAt.Location())
	assert.Equal(t, id, cursor.ID)

	assert.Equal(t, encoded, encodePageCursor(*cursor), "encoding is canonical: same position, same cursor")
}

// feedPosts returns n saved-looking posts, newest first, one second apart.
func feedPosts(n int) []domain.FeedPost {
	posts := make([]domain.FeedPost, n)

	newest := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := range posts {
		posts[i] = domain.FeedPost{
			ID:        uuid.New(),
			CreatedAt: newest.Add(-time.Duration(i) * time.Second),
		}
	}

	return posts
}

// TestNewPageHasMore proves the lookahead row is dropped, has_more is set and
// next_cursor points at the last row the client sees.
func TestNewPageHasMore(t *testing.T) {
	posts := feedPosts(4) // three rows for a limit of 3, plus the lookahead

	page := newPage(posts, 3, feedPostKey)

	require.Len(t, page.Items, 3, "the lookahead row is not returned")
	assert.Equal(t, posts[0].ID, page.Items[0].ID)
	assert.Equal(t, posts[2].ID, page.Items[2].ID)
	assert.True(t, page.HasMore)
	require.NotNil(t, page.NextCursor)

	next, err := decodePageCursor(*page.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, posts[2].ID, next.ID, "the next page starts after the last row returned")
	assert.True(t, posts[2].CreatedAt.Equal(next.CreatedAt))
}

// TestNewPageLastPage covers the two ways a page ends the list: exactly limit
// rows, and a short page.
func TestNewPageLastPage(t *testing.T) {
	t.Run("exactly the limit", func(t *testing.T) {
		page := newPage(feedPosts(3), 3, feedPostKey)

		assert.Len(t, page.Items, 3)
		assert.False(t, page.HasMore)
		assert.Nil(t, page.NextCursor, "there is no next page")
	})

	t.Run("short page", func(t *testing.T) {
		page := newPage(feedPosts(2), 3, feedPostKey)

		assert.Len(t, page.Items, 2)
		assert.False(t, page.HasMore)
		assert.Nil(t, page.NextCursor)
	})
}

// TestNewPageEmptyResult proves an empty list serialises as [] and never null.
func TestNewPageEmptyResult(t *testing.T) {
	for name, rows := range map[string][]domain.FeedPost{
		"nil rows":   nil,
		"empty rows": {},
	} {
		t.Run(name, func(t *testing.T) {
			page := newPage(rows, 20, feedPostKey)

			require.NotNil(t, page.Items)
			assert.Empty(t, page.Items)
			assert.False(t, page.HasMore)
			assert.Nil(t, page.NextCursor)

			body, err := json.Marshal(page)
			require.NoError(t, err)
			assert.JSONEq(t, `{"items":[],"next_cursor":null,"has_more":false}`, string(body))
		})
	}
}

// TestNewPageNonPositiveLimit covers the defensive limit guard. Without it a
// limit below 1 would truncate the rows to zero while has_more stayed true, and
// the next_cursor would index items[-1] and panic. The guard falls back to
// defaultPageLimit, so the page keeps the first 20 rows and a correct cursor
// instead of crashing.
func TestNewPageNonPositiveLimit(t *testing.T) {
	posts := feedPosts(defaultPageLimit + 5) // more rows than one default page

	tests := []struct {
		name  string
		limit int
	}{
		{name: "zero", limit: 0},
		{name: "negative", limit: -10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var page pageResponse[domain.FeedPost]
			require.NotPanics(t, func() {
				page = newPage(posts, test.limit, feedPostKey)
			}, "a non-positive limit must not panic")

			require.Len(t, page.Items, defaultPageLimit, "a non-positive limit falls back to the default page size")
			assert.Equal(t, posts[0].ID, page.Items[0].ID)
			assert.Equal(t, posts[defaultPageLimit-1].ID, page.Items[defaultPageLimit-1].ID)
			assert.True(t, page.HasMore, "the remaining rows are a next page")

			require.NotNil(t, page.NextCursor)
			next, err := decodePageCursor(*page.NextCursor)
			require.NoError(t, err)
			assert.Equal(t, posts[defaultPageLimit-1].ID, next.ID, "the cursor is the last row kept")
			assert.True(t, posts[defaultPageLimit-1].CreatedAt.Equal(next.CreatedAt))
		})
	}
}

// TestPageResponseJSONShape pins the response keys §1.6 fixes.
func TestPageResponseJSONShape(t *testing.T) {
	posts := feedPosts(3)

	body, err := json.Marshal(newPage(posts, 2, feedPostKey))
	require.NoError(t, err)

	var decoded struct {
		Items      []domain.FeedPost `json:"items"`
		NextCursor *string           `json:"next_cursor"`
		HasMore    bool              `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))

	require.Len(t, decoded.Items, 2)
	assert.Equal(t, posts[0].ID, decoded.Items[0].ID)
	require.NotNil(t, decoded.NextCursor)
	assert.True(t, decoded.HasMore)

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &keys))
	assert.Len(t, keys, 3, "the envelope has exactly items, next_cursor and has_more")
}

// TestPaginatedRequestErrorsUseEnvelope drives the parsing errors through a
// handler, so the 400 and the §1.5 body are proven end to end.
func TestPaginatedRequestErrorsUseEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/things", func(c *gin.Context) {
		if _, err := parsePageQuery(c); err != nil {
			writeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})

	serve := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec
	}

	for _, target := range []string{
		"/things?limit=abc",
		"/things?limit=0",
		"/things?limit=-10",
		"/things?cursor=%21%21not-base64%21%21",
	} {
		rec := serve(target)
		require.Equal(t, http.StatusBadRequest, rec.Code, target)
		assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code, target)
	}

	assert.Equal(t, http.StatusNoContent, serve("/things?limit=20").Code)
}
