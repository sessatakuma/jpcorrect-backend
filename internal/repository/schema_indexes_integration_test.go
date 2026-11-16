package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/testutil"
)

// indexCase is one index of spec §2.2 that must exist in a real PostgreSQL.
// columns is checked against pg_get_indexdef output, where String() shows the
// column list exactly as defined (for example "(guild_id, created_at DESC)").
type indexCase struct {
	table   string
	index   string
	unique  bool
	columns string
	partial string // expected WHERE fragment; empty for a full index
}

// tableIndexes returns schema.indexdef for one table, keyed by index name.
func tableIndexes(t *testing.T, db *gorm.DB, schema, table string) map[string]string {
	t.Helper()

	type row struct {
		IndexName string `gorm:"column:indexname"`
		IndexDef  string `gorm:"column:indexdef"`
	}
	var rows []row
	require.NoError(t, db.WithContext(context.Background()).Raw(
		`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = ? AND tablename = ?`, schema, table).
		Scan(&rows).Error)
	require.NotEmpty(t, rows, "%s has no indexes", table)

	indexes := make(map[string]string, len(rows))
	for _, r := range rows {
		indexes[r.IndexName] = r.IndexDef
	}
	return indexes
}

// TestSchemaIndexes asserts the non-unique and partial indexes of spec §2.2
// exist through pg_indexes. Unique indexes are listed too, so a rename or a
// dropped uniqueness is caught here as well.
func TestSchemaIndexes(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	schema := requireCurrentSchema(t, db)

	cases := []indexCase{
		// Activity: dashboard / cron / worker scans and the per-guild sequence.
		{table: "activity", index: "idx_activity_status", columns: "(status)"},
		{table: "activity", index: "idx_activity_ai_status", columns: "(ai_status)"},
		{table: "activity", index: "idx_activity_youtube_status", columns: "(youtube_status)"},
		{table: "activity", index: "idx_activity_guild_sequence", unique: true, columns: "(guild_id, sequence_number)"},
		{table: "activity", index: "idx_activity_guild_id", columns: "(guild_id)"},

		// Event: one practice + one review Event per Activity.
		{table: "event", index: "idx_event_activity_kind", unique: true, columns: "(activity_id, kind)"},
		{table: "event", index: "idx_event_activity_id", columns: "(activity_id)"},

		// User mapping and guild invite lookups.
		{table: "user", index: "idx_user_supabase_user_id", unique: true, columns: "(supabase_user_id)"},
		{table: "user", index: "idx_user_email", unique: true, columns: "(email)"},
		{table: "invite_link", index: "idx_invite_link_token", unique: true, columns: "(token)"},
		{table: "invite_link", index: "idx_invite_link_guild_expires", columns: "(guild_id, expires_at)"},
		{table: "join_request", index: "idx_join_request_user_status_pending",
			columns: "user_id, status", partial: "WHERE (status = 'pending'::text)"},

		// Feedback and review.
		{table: "feedback_progress", index: "idx_feedback_progress_activity_status", columns: "(activity_id, status)"},
		{table: "selected_sentence", index: "idx_selected_sentence_mistake_user",
			unique: true, columns: "(mistake_id, selected_by_user_id)"},
		{table: "selected_sentence", index: "idx_selected_sentence_user_activity",
			columns: "(selected_by_user_id, activity_id)"},
		{table: "selected_sentence", index: "idx_selected_sentence_activity_mistake",
			columns: "(activity_id, mistake_id)"},

		// Feed.
		{table: "feed_post", index: "idx_feed_post_guild_created", columns: "(guild_id, created_at DESC)"},
		{table: "feed_comment", index: "idx_feed_comment_post_created", columns: "(post_id, created_at)"},

		// Notifications.
		{table: "notification", index: "idx_notification_user_read_created",
			columns: "(user_id, read_at, created_at DESC)"},
		{table: "notification", index: "idx_notification_user_unread",
			columns: "(user_id)", partial: "WHERE (read_at IS NULL)"},
		{table: "notification", index: "idx_notification_outbox_id", unique: true, columns: "(outbox_id)"},
		{table: "notification_outbox", index: "idx_notification_outbox_dedup_key",
			unique: true, columns: "(dedup_key)"},
		{table: "push_subscription", index: "idx_push_subscription_endpoint",
			unique: true, columns: "(endpoint)"},

		// AI analysis idempotency key.
		{table: "ai_analysis_submission", index: "idx_ai_analysis_submission_activity_id",
			unique: true, columns: "(activity_id)"},
	}

	byTable := make(map[string]map[string]string)
	for _, tc := range cases {
		indexes, ok := byTable[tc.table]
		if !ok {
			indexes = tableIndexes(t, db, schema, tc.table)
			byTable[tc.table] = indexes
		}

		def, ok := indexes[tc.index]
		if !assert.True(t, ok, "missing index %s on %s; found %v", tc.index, tc.table, keys(indexes)) {
			continue
		}

		t.Logf("index %s.%s: %s", tc.table, tc.index, def)
		assert.Contains(t, def, tc.columns, "index %s must cover %s", tc.index, tc.columns)
		if tc.unique {
			assert.Contains(t, def, "CREATE UNIQUE INDEX", "index %s must be unique", tc.index)
		} else {
			assert.NotContains(t, def, "CREATE UNIQUE INDEX", "index %s must not be unique", tc.index)
		}
		if tc.partial == "" {
			assert.NotContains(t, def, " WHERE ", "index %s must not be partial", tc.index)
		} else {
			assert.Contains(t, def, tc.partial, "index %s must be partial", tc.index)
		}
	}

	// The retired partial unique index on guild_attendee must not come back:
	// the spec (#72) leaves the leader quota to the repository, because a
	// partial index on user_id also counts the row of a soft-deleted guild.
	guildAttendeeIndexes := tableIndexes(t, db, schema, "guild_attendee")
	_, retired := guildAttendeeIndexes["idx_guild_attendee_active_master"]
	assert.False(t, retired, "the retired partial unique index must not exist; found %v", keys(guildAttendeeIndexes))
	_, guildUser := guildAttendeeIndexes["idx_guild_attendee_guild_user"]
	assert.True(t, guildUser, "the (guild_id, user_id) unique index must stay; found %v", keys(guildAttendeeIndexes))
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
