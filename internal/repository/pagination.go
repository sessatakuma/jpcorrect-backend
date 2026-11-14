package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PageCursor is a position in a keyset-ordered list: the (created_at, id) pair
// of the last row a client has already seen.
//
// It is only meaningful for the one order the §1.6 pagination contract defines,
// (created_at DESC, id DESC). The pair is the whole sort key, so rows sharing a
// created_at still have an unambiguous position and page boundaries never
// duplicate or skip them. A list with its own domain order (for example guild
// discover) must define its own cursor in its own feature issue; it must not
// reuse PageAfter with a different sort.
//
// The base64/JSON wire form of the cursor is a transport concern and lives in
// internal/api; the repository layer only needs the decoded position.
type PageCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// PageAfter returns a GORM scope that applies the §1.6 keyset page to a query:
//
//	WHERE (created_at, id) < (?, ?)      -- only when cursor is not nil
//	ORDER BY created_at DESC, id DESC
//	LIMIT limit + 1
//
// The row-wise comparison is PostgreSQL's lexicographic tuple comparison, so a
// result keeps exactly the (created_at DESC, id DESC) order: rows older than
// the cursor's created_at come first, and rows sharing that timestamp are
// ordered and compared by id.
//
// The scope asks for limit+1 rows on purpose. The extra row is the caller's
// lookahead: after fetching, len(rows) > limit means there is a next page, and
// the response builder in internal/api drops the extra row and returns the
// cursor of the last row it keeps. That way "is there another page" is answered
// without a second COUNT query.
//
// limit must be at least 1; the caller validates it (internal/api caps it at
// 100). A nil cursor starts at the first page. The scope never chooses the
// model, the filters, or the soft-delete behaviour: the caller composes it with
// its own WithContext/Where clauses and the model's own scopes still apply.
func PageAfter(cursor *PageCursor, limit int) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		query := db.Order("created_at DESC, id DESC")
		if cursor != nil {
			query = query.Where("(created_at, id) < (?, ?)", cursor.CreatedAt.UTC(), cursor.ID)
		}
		return query.Limit(limit + 1)
	}
}
