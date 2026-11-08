package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserCreateDuplicateEmailEnvelope drives a real PostgreSQL unique
// violation through a handler. The repository maps SQLSTATE 23505 to
// domain.ErrDuplicateEntry with MapGormError and the handler renders the §1.5
// envelope with 409 — no SQL mock is involved.
func TestUserCreateDuplicateEmailEnvelope(t *testing.T) {
	db := testutil.NewPostgresDB(t)

	api := NewAPI("", "", http.DefaultTransport.(*http.Transport).Clone(), db, "", nil)
	t.Cleanup(api.Close)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// The handler is mounted directly: user authentication is #73's contract,
	// this test covers the database error path.
	router.POST("/v1/users", api.UserCreateHandler)

	create := func() *httptest.ResponseRecorder {
		body := []byte(`{"email":"duplicate@example.com","name":"Duplicate"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	first := create()
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := create()
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())

	env := decodeEnvelope(t, second)
	assert.Equal(t, "CONFLICT", env.Error.Code)
	assert.Equal(t, "User already exists", env.Error.Message)
	assert.Empty(t, env.Error.Details)

	// The database error must not leak into the response.
	for _, leaked := range []string{"23505", "duplicate key value", "idx_user_email"} {
		assert.NotContains(t, second.Body.String(), leaked)
	}

	// Exactly one row exists, so the 409 came from the unique constraint.
	var users []domain.User
	require.NoError(t, db.Where("email = ?", "duplicate@example.com").Find(&users).Error)
	assert.Len(t, users, 1)
}
