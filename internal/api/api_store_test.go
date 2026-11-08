package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
	"jpcorrect-backend/internal/testutil"
)

// TestNewAPIBuildsStoreOnTheSameDatabase proves that the API exposes a Store.Tx
// entry point built on the root handle the single-repository fields already use.
func TestNewAPIBuildsStoreOnTheSameDatabase(t *testing.T) {
	db := testutil.NewPostgresDB(t)
	api := NewAPI("", "", "", &http.Transport{}, db, "", nil)
	t.Cleanup(api.Close)

	require.NotNil(t, api.store)

	ctx := context.Background()
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.com", Name: "api store wiring"}
	require.NoError(t, api.store.Tx(ctx, func(r repository.RepoSet) error {
		return r.UserRepo.Create(ctx, &user)
	}))

	// The transaction wrote through the API's own handle, so a plain read sees it.
	committed, err := repository.NewGormUserRepository(db).GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, user.Email, committed.Email)
}
