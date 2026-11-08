package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEventModeHasNoDiscussion pins spec §2.1: discussion practice was dropped
// in Phase 0, so `discussion` must not be a supported EventMode any more.
func TestEventModeHasNoDiscussion(t *testing.T) {
	modes := []EventMode{
		EventModeReport,
		EventModeConversation,
		EventModeReview,
	}

	assert.NotContains(t, modes, EventMode("discussion"),
		"spec §2.1 removes the discussion EventMode")
	assert.Equal(t, []EventMode{"report", "conversation", "review"}, modes)
}

// TestMistakeEnums pins the spec §2.1 enum values of the new Mistake columns.
func TestMistakeEnums(t *testing.T) {
	assert.Equal(t, []MistakeConfidence{"low", "medium", "high"}, []MistakeConfidence{
		MistakeConfidenceLow,
		MistakeConfidenceMedium,
		MistakeConfidenceHigh,
	})
	assert.Equal(t, []MistakeCorrectionSource{"initial", "chatbot"}, []MistakeCorrectionSource{
		MistakeCorrectionSourceInitial,
		MistakeCorrectionSourceChatbot,
	})
}
