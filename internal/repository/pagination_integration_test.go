package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// TestPageAfterScope pins the scope's own contract on real PostgreSQL: the
// (created_at DESC, id DESC) order, the strict row-wise comparison that makes
// rows sharing created_at pageable, and the limit+1 lookahead the response
// builder in internal/api relies on. FeedPost is used because it already has
// created_at and a uuid id.
func TestPageAfterScope(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()

	guild := domain.Guild{ID: uuid.New(), Name: "pagination scope guild"}
	require.NoError(t, db.WithContext(ctx).Create(&guild).Error)

	tied := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	older := tied.Add(-time.Minute)

	for i := range 4 {
		post := domain.FeedPost{
			ID:        uuid.New(),
			GuildID:   guild.ID,
			Type:      domain.FeedPostTypeSystem,
			Content:   "tied post " + uuid.NewString()[:8],
			CreatedAt: tied,
		}
		require.NoError(t, db.WithContext(ctx).Create(&post).Error, "create tied post %d", i)
	}
	for i := range 2 {
		post := domain.FeedPost{
			ID:        uuid.New(),
			GuildID:   guild.ID,
			Type:      domain.FeedPostTypeSystem,
			Content:   "older post " + uuid.NewString()[:8],
			CreatedAt: older,
		}
		require.NoError(t, db.WithContext(ctx).Create(&post).Error, "create older post %d", i)
	}

	// The expected order, read with the same ORDER BY and no limit.
	var ordered []domain.FeedPost
	require.NoError(t, db.WithContext(ctx).
		Where("guild_id = ?", guild.ID).
		Order("created_at DESC, id DESC").
		Find(&ordered).Error)
	require.Len(t, ordered, 6)
	require.True(t, ordered[0].CreatedAt.Equal(ordered[2].CreatedAt),
		"the fixture really has a created_at tie wider than one page")

	pageIDs := func(posts []domain.FeedPost) []uuid.UUID {
		ids := make([]uuid.UUID, len(posts))
		for i, post := range posts {
			ids[i] = post.ID
		}
		return ids
	}
	wantIDs := func(from, count int) []uuid.UUID { return pageIDs(ordered[from : from+count]) }

	t.Run("no cursor starts at the top with the lookahead row", func(t *testing.T) {
		var top []domain.FeedPost
		require.NoError(t, db.WithContext(ctx).
			Where("guild_id = ?", guild.ID).
			Scopes(PageAfter(nil, 5)).
			Find(&top).Error)

		require.Len(t, top, 6, "limit+1 rows, so the caller can tell there is a next page")
		assert.Equal(t, wantIDs(0, 6), pageIDs(top))
	})

	t.Run("a cursor continues after its own row inside a tie", func(t *testing.T) {
		cursor := &PageCursor{CreatedAt: ordered[1].CreatedAt, ID: ordered[1].ID}

		var page []domain.FeedPost
		require.NoError(t, db.WithContext(ctx).
			Where("guild_id = ?", guild.ID).
			Scopes(PageAfter(cursor, 2)).
			Find(&page).Error)

		require.Len(t, page, 3, "limit+1 rows")
		assert.Equal(t, wantIDs(2, 3), pageIDs(page), "the cursor's own row is excluded, the rest of the tie follows")
	})

	t.Run("a cursor normalises its timestamp to UTC", func(t *testing.T) {
		local := ordered[1].CreatedAt.In(time.FixedZone("JST", 9*60*60))
		cursor := &PageCursor{CreatedAt: local, ID: ordered[1].ID}

		var page []domain.FeedPost
		require.NoError(t, db.WithContext(ctx).
			Where("guild_id = ?", guild.ID).
			Scopes(PageAfter(cursor, 2)).
			Find(&page).Error)

		require.Len(t, page, 3)
		assert.Equal(t, wantIDs(2, 3), pageIDs(page), "the same instant selects the same rows whatever zone it is in")
	})
}
