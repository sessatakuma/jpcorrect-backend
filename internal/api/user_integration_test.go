package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
	"jpcorrect-backend/internal/testutil"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newRegisterIntegrationAPI wires Register() against a real PostgreSQL schema,
// so the handlers exercise the GORM repository instead of a mock.
func newRegisterIntegrationAPI(t *testing.T) (*gin.Engine, *API, *testSigner, *gorm.DB) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db := testutil.NewPostgresDB(t)

	api := NewAPI("", "", "", http.DefaultTransport.(*http.Transport).Clone(), db, "", nil)
	t.Cleanup(api.Close)

	signer := newTestSigner(t)
	signer.attachTo(t, api)

	router := gin.New()
	Register(router, api)

	return router, api, signer, db
}

// countUsers counts user rows matching query.
func countUsers(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()

	var count int64
	require.NoError(t, db.Model(&domain.User{}).Where(query, args...).Count(&count).Error)
	return count
}

// TestUserInitConcurrentCalls is the §6.3 acceptance: 20 concurrent first
// logins for one sub create exactly one row, and every call answers with that
// row's user_id.
func TestUserInitConcurrentCalls(t *testing.T) {
	router, _, signer, db := newRegisterIntegrationAPI(t)

	sub := uuid.New()
	const email = "concurrent@example.com"
	token := "Bearer " + signer.userToken(t, sub, email)

	const calls = 20
	type outcome struct {
		status int
		userID string
		body   string
	}
	results := make([]outcome, calls)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
			req.Header.Set("Authorization", token)
			rec := httptest.NewRecorder()

			<-start
			router.ServeHTTP(rec, req)

			var body domain.User
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err == nil {
				results[i] = outcome{status: rec.Code, userID: body.ID.String(), body: rec.Body.String()}
				return
			}
			results[i] = outcome{status: rec.Code, body: rec.Body.String()}
		}(i)
	}

	close(start)
	wg.Wait()

	require.Equal(t, int64(1), countUsers(t, db, "supabase_user_id = ?", sub),
		"20 concurrent init calls must leave exactly one row")

	first := results[0]
	require.Equal(t, http.StatusOK, first.status, first.body)
	require.NotEmpty(t, first.userID)
	require.NotEqual(t, uuid.Nil.String(), first.userID)

	for i, result := range results {
		assert.Equal(t, http.StatusOK, result.status, "call %d: %s", i, result.body)
		assert.Equal(t, first.userID, result.userID, "call %d resolved a different row", i)
	}

	// The stored row carries the JWT identity and nothing from a request body.
	stored, err := repository.NewGormUserRepository(db).GetBySupabaseUserID(t.Context(), sub)
	require.NoError(t, err)
	assert.Equal(t, first.userID, stored.ID.String())
	assert.Equal(t, email, stored.Email)
	assert.Equal(t, "concurrent", stored.Name, "a new account is named after its email prefix")
	assert.Equal(t, domain.UserRoleUser, stored.Role, "a new account gets the model default role")
	assert.Equal(t, domain.UserStatusActive, stored.Status)
	assert.Zero(t, stored.Points)
}

// TestUserInitEmailCollision pins the engineering default of #73: an email that
// already belongs to another row answers 409, creates no row and links nothing.
func TestUserInitEmailCollision(t *testing.T) {
	router, api, signer, db := newRegisterIntegrationAPI(t)

	const email = "owner@example.com"
	olderSub := uuid.New()
	existing := domain.User{ID: uuid.New(), Email: email, Name: "owner", SupabaseUserID: &olderSub}
	require.NoError(t, api.userRepo.Create(t.Context(), &existing))

	newSub := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
	req.Header.Set("Authorization", "Bearer "+signer.userToken(t, newSub, email))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	env := decodeEnvelope(t, rec)
	assert.Equal(t, "CONFLICT", env.Error.Code)
	assert.Equal(t, "email is already in use by another account", env.Error.Message)

	assert.Equal(t, int64(1), countUsers(t, db, "email = ?", email), "no second row may own the email")
	assert.Equal(t, int64(0), countUsers(t, db, "supabase_user_id = ?", newSub), "no row may be created for the new sub")

	// The existing row is untouched: no automatic account linking.
	stored, err := api.userRepo.GetByID(t.Context(), existing.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.SupabaseUserID)
	assert.Equal(t, olderSub, *stored.SupabaseUserID)
}

// TestRegisterUserMeCannotSelfPromote proves over PostgreSQL that PUT /me and
// users/init cannot touch server-owned columns, and that a custom role claim in
// the token changes nothing.
func TestRegisterUserMeCannotSelfPromote(t *testing.T) {
	router, api, signer, db := newRegisterIntegrationAPI(t)

	sub := uuid.New()
	stored := domain.User{ID: uuid.New(), Email: "self@example.com", Name: "before", SupabaseUserID: &sub}
	require.NoError(t, api.userRepo.Create(t.Context(), &stored))

	// The token claims a role; authorization must still use the stored row.
	token := signer.rawToken(t, jwt.MapClaims{
		"sub":   sub.String(),
		"email": "self@example.com",
		"role":  "admin",
	})

	body := `{"name":"after","role":"admin","status":"banned","points":9999,"email":"other@example.com"}
	`
	req := httptest.NewRequest(http.MethodPut, "/v1/users/me", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	updated, err := repository.NewGormUserRepository(db).GetByID(t.Context(), stored.ID)
	require.NoError(t, err)
	assert.Equal(t, "after", updated.Name)
	assert.Equal(t, domain.UserRoleUser, updated.Role, "a user cannot promote itself")
	assert.Equal(t, domain.UserStatusActive, updated.Status)
	assert.Zero(t, updated.Points)
	assert.Equal(t, "self@example.com", updated.Email)
	require.NotNil(t, updated.SupabaseUserID)
	assert.Equal(t, sub, *updated.SupabaseUserID)

	assert.Equal(t, int64(0), countUsers(t, db, "email = ?", "other@example.com"),
		"the body cannot introduce another email")
}

// TestRegisterUserPublicProfileIntegration checks the public profile shape
// against a real row: the caller sees their own profile, but /users/:id and
// /users/name/:name never leak the private columns.
func TestRegisterUserPublicProfileIntegration(t *testing.T) {
	router, api, signer, _ := newRegisterIntegrationAPI(t)

	callerSub := uuid.New()
	caller := domain.User{ID: uuid.New(), Email: "caller@example.com", Name: "caller", SupabaseUserID: &callerSub}
	require.NoError(t, api.userRepo.Create(t.Context(), &caller))

	targetSub := uuid.New()
	target := domain.User{
		ID:              uuid.New(),
		Email:           "target@example.com",
		Name:            "target",
		IsEmailVerified: true,
		Role:            domain.UserRoleAdmin,
		SupabaseUserID:  &targetSub,
		Points:          100,
		Level:           7,
	}
	require.NoError(t, api.userRepo.Create(t.Context(), &target))

	token := "Bearer " + signer.userToken(t, callerSub, caller.Email)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// The caller's own profile keeps the private fields.
	me := get("/v1/users/me")
	require.Equal(t, http.StatusOK, me.Code, me.Body.String())
	var meBody domain.User
	require.NoError(t, json.Unmarshal(me.Body.Bytes(), &meBody))
	assert.Equal(t, caller.Email, meBody.Email)

	// Another user's profile does not.
	for _, path := range []string{
		"/v1/users/" + target.ID.String(),
		"/v1/users/name/" + target.Name,
	} {
		rec := get(path)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", path, rec.Body.String())

		var profiles []map[string]any
		if strings.HasPrefix(path, "/v1/users/name/") {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profiles))
			require.Len(t, profiles, 1)
		} else {
			var single map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &single))
			profiles = []map[string]any{single}
		}

		assert.Equal(t, target.ID.String(), profiles[0]["user_id"], path)
		assertNoExcludedProfileKeys(t, profiles[0])
	}
}

// TestUserInitRejectsUnusableSubject is the real-PostgreSQL half of spec §2.1:
// users/init answers 400 INVALID_REQUEST for a verified `sub` that is empty,
// malformed or the nil UUID, and writes no row. Every user-resolving route keeps
// answering 401 for the same tokens.
func TestUserInitRejectsUnusableSubject(t *testing.T) {
	router, _, signer, db := newRegisterIntegrationAPI(t)

	cases := []struct{ name, subject string }{
		{"missing subject", ""},
		{"subject that is not a UUID", "not-a-uuid"},
		{"nil UUID subject", uuid.Nil.String()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := "Bearer " + signer.token(t, supabaseClaims{
				Email:            "ghost@example.com",
				RegisteredClaims: jwt.RegisteredClaims{Subject: tc.subject},
			})

			initReq := httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
			initReq.Header.Set("Authorization", token)
			initRec := httptest.NewRecorder()
			router.ServeHTTP(initRec, initReq)

			require.Equal(t, http.StatusBadRequest, initRec.Code, initRec.Body.String())
			assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, initRec).Error.Code)

			meReq := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
			meReq.Header.Set("Authorization", token)
			meRec := httptest.NewRecorder()
			router.ServeHTTP(meRec, meReq)

			require.Equal(t, http.StatusUnauthorized, meRec.Code, meRec.Body.String())
			assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, meRec).Error.Code)

			assert.Equal(t, int64(0), countUsers(t, db, "email = ?", "ghost@example.com"),
				"an unusable subject must not create a row")
		})
	}
}
