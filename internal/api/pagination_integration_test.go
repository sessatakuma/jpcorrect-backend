package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
	"jpcorrect-backend/internal/testutil"
)

// These tests run the whole §1.6 path against real PostgreSQL: parse the query,
// apply repository.PageAfter, and build the response with newPage. They use
// FeedPost because it already carries created_at + a uuid id and the
// (guild_id, created_at) index the spec lists; no endpoint is involved.

// createFeedGuild persists the guild the test posts belong to.
func createFeedGuild(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()

	guild := domain.Guild{ID: uuid.New(), Name: "pagination guild " + uuid.NewString()[:8]}
	require.NoError(t, db.WithContext(context.Background()).Create(&guild).Error)
	return guild.ID
}

// createFeedPostAt persists one post with an explicit created_at, so a test can
// build ties, gaps and microsecond steps. GORM keeps a non-zero CreatedAt.
func createFeedPostAt(t *testing.T, db *gorm.DB, guildID uuid.UUID, createdAt time.Time) domain.FeedPost {
	t.Helper()

	post := domain.FeedPost{
		ID:        uuid.New(),
		GuildID:   guildID,
		Type:      domain.FeedPostTypeSystem,
		Content:   "pagination post " + uuid.NewString()[:8],
		CreatedAt: createdAt,
	}
	require.NoError(t, db.WithContext(context.Background()).Create(&post).Error)
	return post
}

// fetchFeedPage is one page exactly the way a handler would serve it: parse
// ?limit=&cursor= from the request, run the keyset scope, render the §1.6
// response. rawQuery includes the leading "?".
func fetchFeedPage(t *testing.T, db *gorm.DB, guildID uuid.UUID, rawQuery string) pageResponse[domain.FeedPost] {
	t.Helper()
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/feed"+rawQuery, nil)

	query, err := parsePageQuery(c)
	require.NoError(t, err)

	var posts []domain.FeedPost
	require.NoError(t, db.WithContext(c.Request.Context()).
		Where("guild_id = ?", guildID).
		Scopes(repository.PageAfter(query.Cursor, query.Limit)).
		Find(&posts).Error)

	return newPage(posts, query.Limit, feedPostKey)
}

// mustMarshal renders v as JSON for shape assertions.
func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	require.NoError(t, err)
	return string(body)
}

// nextPageQuery renders the next request's query string from a page's cursor,
// keeping the page size the test asked for.
func nextPageQuery(t *testing.T, page pageResponse[domain.FeedPost], limit int) string {
	t.Helper()
	require.NotNil(t, page.NextCursor)
	return "?limit=" + strconv.Itoa(limit) + "&cursor=" + *page.NextCursor
}

// cursorOnlyQuery renders the next request without a limit, so the default page
// size applies again.
func cursorOnlyQuery(t *testing.T, page pageResponse[domain.FeedPost]) string {
	t.Helper()
	require.NotNil(t, page.NextCursor)
	return "?cursor=" + *page.NextCursor
}

// sortByPageOrder sorts posts the way the contract orders them.
func sortByPageOrder(posts []domain.FeedPost) {
	sort.Slice(posts, func(i, j int) bool {
		if !posts[i].CreatedAt.Equal(posts[j].CreatedAt) {
			return posts[i].CreatedAt.After(posts[j].CreatedAt)
		}
		// uuid strings are canonical, so comparing them bytewise matches
		// PostgreSQL's uuid comparison.
		return posts[i].ID.String() > posts[j].ID.String()
	})
}

// TestPageAfterPagesTiedCreatedAtWithoutDuplicatesOrOmissions walks every page
// of a list whose rows share created_at values. Each tie group is wider than
// the page size, so the tiebreaker has to work: the collected ids must be the
// exact list with no duplicate and no omission, in (created_at DESC, id DESC)
// order.
func TestPageAfterPagesTiedCreatedAtWithoutDuplicatesOrOmissions(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	guildID := createFeedGuild(t, db)

	newest := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	groups := []struct {
		createdAt time.Time
		count     int
	}{
		{newest, 10},
		{newest.Add(-time.Minute), 7},
		{newest.Add(-2 * time.Minute), 5},
	}

	var want []domain.FeedPost
	for _, group := range groups {
		for range group.count {
			want = append(want, createFeedPostAt(t, db, guildID, group.createdAt))
		}
	}

	require.Len(t, want, 22)
	sortByPageOrder(want)

	const limit = 3

	var got []domain.FeedPost
	query := "?limit=" + strconv.Itoa(limit)

	for pages := 1; ; pages++ {
		page := fetchFeedPage(t, db, guildID, query)

		require.NotEmpty(t, page.Items, "page %d is empty before the list ended", pages)
		require.LessOrEqual(t, len(page.Items), limit, "the lookahead row is never returned")

		got = append(got, page.Items...)

		if !page.HasMore {
			assert.Nil(t, page.NextCursor, "the last page has no next_cursor")
			break
		}

		require.NotNil(t, page.NextCursor, "a page with has_more needs a next_cursor")
		query = nextPageQuery(t, page, limit)
	}

	require.Len(t, got, len(want), "every row is returned exactly once")

	seen := make(map[uuid.UUID]struct{}, len(got))
	for i, post := range got {
		assert.Equal(t, want[i].ID, post.ID, "position %d follows (created_at DESC, id DESC)", i)
		_, duplicate := seen[post.ID]
		assert.False(t, duplicate, "row %s was returned twice", post.ID)
		seen[post.ID] = struct{}{}
	}
	assert.Len(t, seen, len(want), "no row is omitted")
}

// TestPageAfterInsertAtTopDoesNotDisturbLaterPages inserts rows at the top while
// a client is paging: later pages must contain exactly the older rows that were
// already there, with no duplicate and no omission, and the new rows belong to
// pages the client never reaches.
func TestPageAfterInsertAtTopDoesNotDisturbLaterPages(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	guildID := createFeedGuild(t, db)

	oldest := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)

	var original []domain.FeedPost
	for i := range 10 {
		original = append(original, createFeedPostAt(t, db, guildID, oldest.Add(time.Duration(i)*time.Second)))
	}
	sortByPageOrder(original)

	const limit = 4

	firstPage := fetchFeedPage(t, db, guildID, "?limit="+strconv.Itoa(limit))
	require.Len(t, firstPage.Items, limit)
	require.True(t, firstPage.HasMore)
	for i, post := range firstPage.Items {
		assert.Equal(t, original[i].ID, post.ID, "the first page starts at the newest row")
	}

	// Someone posts while the client is scrolling: the new rows are newer than
	// everything the client has seen so far.
	var inserted []domain.FeedPost
	for i := range 6 {
		inserted = append(inserted, createFeedPostAt(t, db, guildID, time.Now().UTC().Add(time.Duration(i)*time.Second)))
	}

	var later []domain.FeedPost
	query := nextPageQuery(t, firstPage, limit)
	for pages := 1; ; pages++ {
		page := fetchFeedPage(t, db, guildID, query)
		require.NotEmpty(t, page.Items, "page %d after the insert is empty", pages)
		later = append(later, page.Items...)

		if !page.HasMore {
			assert.Nil(t, page.NextCursor)
			break
		}
		query = nextPageQuery(t, page, limit)
	}

	// Exactly the rows that were already older than the first page's cursor.
	require.Len(t, later, len(original)-limit, "no older row is skipped or duplicated")

	for i, post := range later {
		assert.Equal(t, original[limit+i].ID, post.ID, "later page position %d", i)
	}

	laterIDs := make(map[uuid.UUID]struct{}, len(later))
	for _, post := range later {
		laterIDs[post.ID] = struct{}{}
	}
	for _, post := range inserted {
		_, seen := laterIDs[post.ID]
		assert.False(t, seen, "a row inserted above the cursor must not appear in a later page")
	}
}

// TestPageAfterKeepsMicroseconds proves the cursor round-trips the timestamp
// PostgreSQL stores. Rows one microsecond apart are only orderable if the
// cursor keeps all six fractional digits; a millisecond or second precision
// would return several rows per page or skip one.
func TestPageAfterKeepsMicroseconds(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	guildID := createFeedGuild(t, db)

	base := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := range 5 {
		createFeedPostAt(t, db, guildID, base.Add(time.Duration(i)*time.Microsecond))
	}

	// PostgreSQL is the authority on the stored values, and the ORDER BY here is
	// the same order the scope uses.
	var stored []domain.FeedPost
	require.NoError(t, db.WithContext(context.Background()).
		Where("guild_id = ?", guildID).
		Order("created_at DESC, id DESC").
		Find(&stored).Error)
	require.Len(t, stored, 5)

	for i := 0; i < len(stored)-1; i++ {
		require.Equal(t, time.Microsecond, stored[i].CreatedAt.Sub(stored[i+1].CreatedAt),
			"the fixture rows are exactly one microsecond apart")
	}
	const limit = 1

	query := "?limit=" + strconv.Itoa(limit)
	for i, want := range stored {
		page := fetchFeedPage(t, db, guildID, query)
		require.Len(t, page.Items, 1, "page %d", i)
		assert.Equal(t, want.ID, page.Items[0].ID, "page %d returns the next microsecond", i)
		assert.True(t, want.CreatedAt.Equal(page.Items[0].CreatedAt))

		if !page.HasMore {
			assert.Nil(t, page.NextCursor, "the last page has no next_cursor")
			require.Equal(t, len(stored)-1, i, "the list ends on the oldest row")
			break
		}

		next, err := decodePageCursor(*page.NextCursor)
		require.NoError(t, err)
		assert.True(t, want.CreatedAt.Equal(next.CreatedAt),
			"the cursor carries the stored microsecond value %s, got %s", want.CreatedAt, next.CreatedAt)
		assert.Equal(t, want.ID, next.ID)

		query = nextPageQuery(t, page, limit)
	}
}

// TestPageAfterEmptyResult proves an empty list is items: [] in the JSON body,
// has_more false and next_cursor null.
func TestPageAfterEmptyResult(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	guildID := createFeedGuild(t, db)

	page := fetchFeedPage(t, db, guildID, "")

	require.NotNil(t, page.Items, "items is an empty slice, never nil")
	assert.Empty(t, page.Items)
	assert.False(t, page.HasMore)
	assert.Nil(t, page.NextCursor)
	assert.JSONEq(t, `{"items":[],"next_cursor":null,"has_more":false}`, mustMarshal(t, page))
}

// TestPageAfterDefaultLimitOnRealRows drives the default limit through the
// database: 25 rows with no ?limit= come back 20 at a time.
func TestPageAfterDefaultLimitOnRealRows(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	guildID := createFeedGuild(t, db)

	newest := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := range defaultPageLimit + 5 {
		createFeedPostAt(t, db, guildID, newest.Add(-time.Duration(i)*time.Second))
	}

	first := fetchFeedPage(t, db, guildID, "")
	assert.Len(t, first.Items, defaultPageLimit, "the default page size is 20")
	assert.True(t, first.HasMore)

	second := fetchFeedPage(t, db, guildID, cursorOnlyQuery(t, first))
	assert.Len(t, second.Items, 5, "the rest of the list fits in the second page")
	assert.False(t, second.HasMore)
	assert.Nil(t, second.NextCursor)
}
