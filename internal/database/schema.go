package database

import (
	"context"

	"gorm.io/gorm"
)

// legacyIndexes lists indexes that an earlier release created and the current
// schema does not define. AutoMigrate only creates and alters: once an index tag
// is removed from a model, the index stays in every database that already has
// it. Each name below is therefore dropped explicitly.
//
// idx_guild_attendee_active_master was a partial unique index on
// guild_attendee(user_id) WHERE role = 'master' AND left_at IS NULL. It
// enforced the one-guild-per-leader quota (§3.3.1) in the database, but it also
// counted the membership row of a soft-deleted guild, so a leader whose guild
// was deleted could never create another one. The quota now lives in the
// repository (lockLeaderQuota in internal/repository/gorm_guild.go), which joins
// guild and skips the deleted rows.
var legacyIndexes = []string{
	"idx_guild_attendee_active_master",
}

// DropLegacyIndexes drops every index in legacyIndexes. It is idempotent:
// DROP INDEX IF EXISTS succeeds on a fresh database, on a database that still
// carries the index, and on a database that already ran this step. The names are
// compile-time constants, never user input.
func DropLegacyIndexes(ctx context.Context, db *gorm.DB) error {
	for _, index := range legacyIndexes {
		if err := db.WithContext(ctx).Exec("DROP INDEX IF EXISTS " + index).Error; err != nil {
			return err
		}
	}
	return nil
}
