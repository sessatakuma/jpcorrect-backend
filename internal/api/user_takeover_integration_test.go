package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// This file is the real-PostgreSQL, real-JWT half of the raw user write policy:
// the three legacy raw writes bind a whole domain.User, so without a gate any
// signed-in caller could move another account's supabase_user_id onto a row it
// controls and take that identity over. The cases below run in gin.ReleaseMode,
// the mode a deployment runs in, against the same Register() wiring the server
// starts with.

// newReleaseIntegrationAPI mirrors newRegisterIntegrationAPI in gin.ReleaseMode,
// where no middleware is skipped and the api-tools API key is enforced.
func newReleaseIntegrationAPI(t *testing.T) (*gin.Engine, *API, *testSigner, *gorm.DB) {
	t.Helper()

	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })
	gin.SetMode(gin.ReleaseMode)

	db := testutil.NewPostgresDB(t)

	api := NewAPI("", "", http.DefaultTransport.(*http.Transport).Clone(), db, "", nil)
	t.Cleanup(api.Close)

	signer := newTestSigner(t)
	signer.attachTo(t, api)

	router := gin.New()
	Register(router, api)

	return router, api, signer, db
}

// takeoverFixture is a release-mode router plus one stored account per role.
type takeoverFixture struct {
	router *gin.Engine
	api    *API
	signer *testSigner
	users  domain.UserRepository

	attacker, victim, staff, admin domain.User

	attackerToken, staffToken, adminToken string
}

func newTakeoverFixture(t *testing.T) *takeoverFixture {
	t.Helper()

	router, api, signer, _ := newReleaseIntegrationAPI(t)
	f := &takeoverFixture{router: router, api: api, signer: signer, users: api.userRepo}

	create := func(role domain.UserRole) (domain.User, string) {
		sub := uuid.New()
		user := domain.User{
			ID:             uuid.New(),
			Email:          uuid.NewString() + "@example.com",
			Name:           string(role) + " user",
			Role:           role,
			SupabaseUserID: &sub,
		}
		require.NoError(t, api.userRepo.Create(t.Context(), &user))
		return user, signer.userToken(t, sub, user.Email)
	}

	f.attacker, f.attackerToken = create(domain.UserRoleUser)
	f.victim, _ = create(domain.UserRoleUser)
	f.staff, f.staffToken = create(domain.UserRoleStaff)
	f.admin, f.adminToken = create(domain.UserRoleAdmin)

	return f
}

// do sends one request with the given bearer token.
func (f *takeoverFixture) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// reload reads a row through a fresh repository, bypassing the handler.
func reload(t *testing.T, f *takeoverFixture, id uuid.UUID) *domain.User {
	t.Helper()

	row, err := f.users.GetByID(t.Context(), id)
	require.NoError(t, err)
	return row
}

// assertVictimIntact pins every column a raw write binds: no caller without the
// admin role may change any of them.
func assertVictimIntact(t *testing.T, row, victim *domain.User) {
	t.Helper()

	assert.Equal(t, victim.Name, row.Name)
	assert.Equal(t, victim.Email, row.Email)
	assert.Equal(t, victim.Role, row.Role)
	assert.Equal(t, victim.Status, row.Status)
	assert.Equal(t, victim.Points, row.Points)
	assert.Equal(t, victim.IsEmailVerified, row.IsEmailVerified)
	require.NotNil(t, row.SupabaseUserID)
	require.NotNil(t, victim.SupabaseUserID)
	assert.Equal(t, *victim.SupabaseUserID, *row.SupabaseUserID, "the identity mapping must not move")
}

// TestRegisterRawUserWriteTakeoverSequence attempts the identity takeover with
// real signed tokens: a plain user and a staff user try to write another
// account's row, a plain user tries to create a row carrying their own identity
// mapping, and a token with a custom admin claim changes nothing. Every attempt
// is refused before the body is bound, and the target row is byte-for-byte the
// row it was.
func TestRegisterRawUserWriteTakeoverSequence(t *testing.T) {
	f := newTakeoverFixture(t)
	attackerSub := *f.attacker.SupabaseUserID

	// The body a takeover needs: move the attacker's own identity mapping onto
	// the row, promote it, and rename it.
	takeoverBody := `{"name":"owned","email":"owned-` + uuid.NewString() + `@example.com",` +
		`"role":"admin","status":"banned","points":9999,` +
		`"supabase_user_id":"` + attackerSub.String() + `"}`

	t.Run("plain user cannot rewrite another row", func(t *testing.T) {
		rec := f.do(t, http.MethodPut, "/v1/users/"+f.victim.ID.String(), takeoverBody, f.attackerToken)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
		assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
	})

	t.Run("staff cannot rewrite another row", func(t *testing.T) {
		rec := f.do(t, http.MethodPut, "/v1/users/"+f.victim.ID.String(), takeoverBody, f.staffToken)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
	})

	t.Run("admin claim in the token is not the stored role", func(t *testing.T) {
		claimed := f.signer.rawToken(t, jwt.MapClaims{
			"sub":   attackerSub.String(),
			"email": f.attacker.Email,
			"role":  "admin",
		})
		rec := f.do(t, http.MethodPut, "/v1/users/"+f.victim.ID.String(), takeoverBody, claimed)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
	})

	t.Run("plain user cannot create a row", func(t *testing.T) {
		row := `{"email":"fresh-` + uuid.NewString() + `@example.com","name":"fresh",` +
			`"role":"admin","supabase_user_id":"` + uuid.NewString() + `"}`
		rec := f.do(t, http.MethodPost, "/v1/users", row, f.attackerToken)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assert.Equal(t, "FORBIDDEN", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("plain user cannot delete another row", func(t *testing.T) {
		rec := f.do(t, http.MethodDelete, "/v1/users/"+f.victim.ID.String(), "", f.attackerToken)
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
	})

	// With a JWT but no stored row the gate still answers 401: a token for an
	// account that never called users/init is an unknown caller, not a 403.
	t.Run("token without a stored row is 401", func(t *testing.T) {
		uninitialized := f.signer.userToken(t, uuid.New(), uuid.NewString()+"@example.com")
		rec := f.do(t, http.MethodPut, "/v1/users/"+f.victim.ID.String(), takeoverBody, uninitialized)
		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
		assert.Equal(t, "USER_NOT_INITIALIZED", decodeEnvelope(t, rec).Error.Code)
		assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
	})

	// The victim row survived every attempt above.
	assertVictimIntact(t, reload(t, f, f.victim.ID), &f.victim)
}

// TestRegisterRawUserWritesForAdmin pins the other half: the gate changes access
// only. An admin still binds a whole domain.User on create and update, and can
// still delete, exactly as the raw handlers did before the gate.
func TestRegisterRawUserWritesForAdmin(t *testing.T) {
	f := newTakeoverFixture(t)

	createdSub := uuid.New()
	createdEmail := uuid.NewString() + "@example.com"
	createBody := `{"email":"` + createdEmail + `","name":"created","role":"staff","points":5,` +
		`"supabase_user_id":"` + createdSub.String() + `"}`

	rec := f.do(t, http.MethodPost, "/v1/users", createBody, f.adminToken)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotEqual(t, uuid.Nil, created.ID)

	stored, err := f.users.GetBySupabaseUserID(t.Context(), createdSub)
	require.NoError(t, err, "the raw create still binds supabase_user_id")
	assert.Equal(t, domain.UserRoleStaff, stored.Role)
	assert.Equal(t, 5, stored.Points)

	// The raw update still writes every column it binds.
	updateBody := `{"name":"renamed","email":"` + createdEmail + `","role":"admin","status":"suspended","points":11,` +
		`"supabase_user_id":"` + createdSub.String() + `"}`
	rec = f.do(t, http.MethodPut, "/v1/users/"+created.ID.String(), updateBody, f.adminToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	updated := reload(t, f, created.ID)
	assert.Equal(t, "renamed", updated.Name)
	assert.Equal(t, domain.UserRoleAdmin, updated.Role)
	assert.Equal(t, domain.UserStatusSuspended, updated.Status)
	assert.Equal(t, 11, updated.Points)

	// The raw delete still deletes.
	rec = f.do(t, http.MethodDelete, "/v1/users/"+f.victim.ID.String(), "", f.adminToken)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	_, err = repository.NewGormUserRepository(f.api.db).GetByID(t.Context(), f.victim.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound, "the deleted row is gone from the read path")
}

// TestRegisterUserSelfServiceSurvivesTheGate pins the product paths a plain user
// keeps: own profile read and write, another user's public profile, first login.
// The self-service write also stays unable to touch a server-owned column.
func TestRegisterUserSelfServiceSurvivesTheGate(t *testing.T) {
	f := newTakeoverFixture(t)

	rec := f.do(t, http.MethodGet, "/v1/users/me", "", f.attackerToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = f.do(t, http.MethodPut, "/v1/users/me",
		`{"name":"self service","role":"admin","status":"banned","points":9999,"email":"stolen@example.com"}`,
		f.attackerToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var response domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "self service", response.Name)
	assert.Equal(t, domain.UserRoleUser, response.Role, "self service cannot promote itself")
	assert.Equal(t, domain.UserStatusActive, response.Status)
	assert.Zero(t, response.Points)
	assert.Equal(t, f.attacker.Email, response.Email)

	row := reload(t, f, f.attacker.ID)
	assert.Equal(t, "self service", row.Name)
	assert.Equal(t, domain.UserRoleUser, row.Role)
	assert.Equal(t, f.attacker.Email, row.Email)
	require.NotNil(t, row.SupabaseUserID)
	assert.Equal(t, *f.attacker.SupabaseUserID, *row.SupabaseUserID)

	// Another user's public profile, and first login, stay open to everyone.
	rec = f.do(t, http.MethodGet, "/v1/users/"+f.victim.ID.String(), "", f.attackerToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertNotServerOwnedProfileKeys(t, rec)

	newSub := uuid.New()
	initToken := f.signer.userToken(t, newSub, uuid.NewString()+"@example.com")
	rec = f.do(t, http.MethodPost, "/v1/users/init", "", initToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var initialized domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initialized))
	assert.NotEqual(t, uuid.Nil, initialized.ID)
}

// assertNotServerOwnedProfileKeys checks the public profile body for the columns
// the raw writes bind but a public read never shows.
func assertNotServerOwnedProfileKeys(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	var profile map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))
	for _, key := range []string{
		"email", "role", "status", "points", "supabase_user_id",
		"password_hash", "is_email_verified", "deleted_at",
	} {
		assert.NotContains(t, profile, key, "the public profile must not leak %s", key)
	}
}
