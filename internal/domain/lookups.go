package domain

import (
	"context"

	"github.com/google/uuid"
)

// Read-only lookups the spec §1.3 permission checks need (issue #74).
//
// They are deliberately narrow: the feature that owns each aggregate adds its
// full repository (writes, listings) in its own issue, while the permission
// helpers depend only on the read side they actually use.

// ActivityLookupRepository reads one Activity for a permission check. It is
// not the full ActivityRepository; a feature issue that writes Activities adds
// that separately.
type ActivityLookupRepository interface {
	GetByID(ctx context.Context, activityID uuid.UUID) (*Activity, error)
}

// SelectedSentenceLookupRepository reads one SelectedSentence for the
// ownership check of spec §3.11.2. It is read-only on purpose: selection
// writes belong to the feature that owns them.
type SelectedSentenceLookupRepository interface {
	GetByID(ctx context.Context, selectedSentenceID uuid.UUID) (*SelectedSentence, error)
}
