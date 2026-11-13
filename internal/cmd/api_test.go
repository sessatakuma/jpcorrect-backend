package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/testutil"
)

// TestMigrationModelsMatchTestBaseline fails when the production AutoMigrate
// list and the integration-test baseline drift apart: the schema tests must run
// against the schema the server actually creates.
func TestMigrationModelsMatchTestBaseline(t *testing.T) {
	require.Equal(t, testutil.BaselineModels(), migrationModels(),
		"internal/cmd migrationModels() and testutil.BaselineModels() must list the same models in the same order")
}
