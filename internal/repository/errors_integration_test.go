package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/testutil"
)

// TestPostgresHelperConnectionSearchPath proves the helper's isolation claim:
// every connection the pool hands out resolves unqualified names inside the
// per-test schema, including when several queries run concurrently.
func TestPostgresHelperConnectionSearchPath(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	var expected string
	require.NoError(t, db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&expected))
	require.NotEmpty(t, expected)
	require.NotEqual(t, "public", expected, "test schema must not be public")

	const workers = 8
	schemas := make([]string, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = db.WithContext(context.Background()).Raw("SELECT current_schema()").Row().Scan(&schemas[i])
		}(i)
	}
	wg.Wait()

	for i := 0; i < workers; i++ {
		require.NoError(t, errs[i], "concurrent current_schema() query %d", i)
		assert.Equal(t, expected, schemas[i], "connection %d used the wrong search_path", i)
	}
}
