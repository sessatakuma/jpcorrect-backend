package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/testutil"
)

// TestStateAndScalarDefaultColumnsAreNotNull pins the nullability half of the
// #123 review: a column with a spec default is NOT NULL unless the spec or #72
// marks it nullable. A NULL status never matches ActivityStatusRepository's
// `status IN (?)` guard, so it would be stuck forever and invisible to the
// stuck-activity cron (§5.3); PostgreSQL must reject it instead of relying on
// the insert path.
func TestStateAndScalarDefaultColumnsAreNotNull(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	schema := requireCurrentSchema(t, db)

	columns := []struct {
		table  string
		column string
	}{
		{"activity", "status"},
		{"activity", "ai_status"},
		{"activity", "youtube_status"},
		{"feedback_progress", "status"},
		{"join_request", "status"},
		{"mistake", "confidence"},
		{"mistake", "latest_correction_source"},
		{"notification_outbox", "status"},
		{"notification_outbox", "attempt_count"},
		{"event", "current_step"},
		{"guild_default_slot", "timezone"},
		{"selected_sentence", "is_degraded"},
	}

	for _, c := range columns {
		t.Run(c.table+"."+c.column, func(t *testing.T) {
			var nullable string
			require.NoError(t, db.WithContext(ctx).
				Raw(`SELECT is_nullable FROM information_schema.columns
					WHERE table_schema = ? AND table_name = ? AND column_name = ?`,
					schema, c.table, c.column).
				Row().Scan(&nullable),
				"column %s.%s must exist", c.table, c.column)
			assert.Equal(t, "NO", nullable, "%s.%s must be NOT NULL", c.table, c.column)
		})
	}
}
