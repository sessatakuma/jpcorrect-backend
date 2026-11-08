package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"
)

// TestEventRepositoryIgnoresPopulatedAssociations proves the Activity and
// RecordingStartedByUser belongs-to fields on Event never make the repository
// write to the parent tables or overwrite activity_id / recording_started_by.
// Without Omit(clause.Associations) GORM upserts the referenced parents.
func TestEventRepositoryIgnoresPopulatedAssociations(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	ctx := context.Background()
	f := newSchemaFixture(t, db)
	repo := NewGormEventRepository(db)

	ghostActivity := &domain.Activity{ID: uuid.New(), GuildID: f.guild, SequenceNumber: 99, Mode: domain.ActivityModeReport}
	ghostUser := &domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "ghost user"}

	requireParentsUntouched := func(t *testing.T) {
		t.Helper()

		var ghostActivities, ghostUsers int64
		require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.Activity{}).
			Where("id = ?", ghostActivity.ID).Count(&ghostActivities).Error)
		require.NoError(t, db.WithContext(ctx).Unscoped().Model(&domain.User{}).
			Where("id = ?", ghostUser.ID).Count(&ghostUsers).Error)
		assert.Zero(t, ghostActivities, "a populated Activity association must not insert a parent row")
		assert.Zero(t, ghostUsers, "a populated RecordingStartedByUser association must not insert a parent row")
	}

	event := f.newEvent(f.activity, domain.EventKindReview)
	event.Activity = ghostActivity
	event.RecordingStartedByUser = ghostUser
	require.NoError(t, repo.Create(ctx, event))
	requireParentsUntouched(t)

	var stored domain.Event
	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", event.ID).Error)
	assert.Equal(t, f.activity, stored.ActivityID, "activity_id must not follow the association")
	assert.Nil(t, stored.RecordingStartedBy, "recording_started_by must not follow the association")

	event.Title = "renamed"
	require.NoError(t, repo.Update(ctx, event))
	requireParentsUntouched(t)

	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", event.ID).Error)
	assert.Equal(t, "renamed", stored.Title, "the scalar columns must still be written")
	assert.Equal(t, f.activity, stored.ActivityID)
	assert.Nil(t, stored.RecordingStartedBy)
}
