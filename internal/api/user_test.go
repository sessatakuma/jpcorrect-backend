package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callerContext is the gin context AuthMiddleware produces for an authenticated
// caller, minus the resolved user: the init route never has one.
func callerContext(sub uuid.UUID, email string) map[string]any {
	return map[string]any{
		ContextKeySupabaseID: sub,
		ContextKeyEmail:      email,
	}
}

func TestUserInitHandlerCreatesAccount(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, "new@example.com"), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var created domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.NotEqual(t, uuid.Nil, created.ID, "the account gets a backend user ID")
	assert.NotEqual(t, sub, created.ID, "the backend user ID is not the Supabase subject")
	assert.Equal(t, "new@example.com", created.Email)
	require.NotNil(t, created.SupabaseUserID)
	assert.Equal(t, sub, *created.SupabaseUserID)

	assert.Equal(t, 1, users.createCalls)
	assert.Equal(t, 1, users.count())

	// The stored row and the response agree, so later requests resolve it.
	stored, err := users.GetBySupabaseUserID(t.Context(), sub)
	require.NoError(t, err)
	assert.Equal(t, created.ID, stored.ID)
}

func TestUserInitHandlerIsIdempotent(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()

	first := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, "repeat@example.com"), nil)
	require.Equal(t, http.StatusOK, first.Code)

	second := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, "repeat@example.com"), nil)
	require.Equal(t, http.StatusOK, second.Code)

	var firstUser, secondUser domain.User
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstUser))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondUser))

	assert.Equal(t, firstUser.ID, secondUser.ID, "a retried init returns the same account")
	assert.Equal(t, 1, users.count(), "a retried init creates no second row")
}

func TestUserInitHandlerExistingAccountIsNotRelinked(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()
	existing := storedAccount(t, sub)
	users.seed(*existing)

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, existing.Email), nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var body domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, existing.ID, body.ID, "init returns the account this subject already owns")
	assert.Equal(t, 1, users.count())
}

func TestUserInitHandlerRefusesEmailOwnedByAnotherAccount(t *testing.T) {
	api, users, _ := newAccountAPI()

	// A legacy account: it holds the email but has no Supabase mapping yet.
	legacy := domain.User{ID: uuid.New(), Email: "legacy@example.com", Name: "legacy"}
	users.seed(legacy)

	sub := uuid.New()
	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, legacy.Email), nil)
	require.Equal(t, http.StatusConflict, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "email is already in use by another account", body["error"])

	assert.Equal(t, 1, users.count(), "no account is created for the colliding email")

	// The existing row is untouched: nothing was linked to the new subject.
	stored, err := users.GetByID(t.Context(), legacy.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.SupabaseUserID, "a matching email never links an account")
	assert.Equal(t, legacy.Email, stored.Email)

	_, err = users.GetBySupabaseUserID(t.Context(), sub)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUserInitHandlerRejectsTokenWithoutEmail(t *testing.T) {
	api, users, _ := newAccountAPI()

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(uuid.New(), "   "), nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, users.createCalls)
}

func TestUserInitHandlerRequiresVerifiedIdentity(t *testing.T) {
	api, users, _ := newAccountAPI()

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init", nil, nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 0, users.createCalls)
}

func TestUserInitHandlerRejectsSoftDeletedAccount(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()
	gone := storedAccount(t, sub)
	users.seed(*gone)
	users.hide(gone.ID)

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(sub, gone.Email), nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 1, users.count(), "the deleted row still owns the subject")
}

func TestUserInitHandlerReportsCreateFailure(t *testing.T) {
	api, users, _ := newAccountAPI()
	users.createErr = assert.AnError

	rec := invokeUserHandler(t, api.UserInitHandler, http.MethodPost, "/v1/users/init",
		callerContext(uuid.New(), "boom@example.com"), nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestUserMeHandlerReturnsResolvedAccount(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()
	account := storedAccount(t, sub)
	users.seed(*account)

	rec := invokeUserHandler(t, api.UserMeHandler, http.MethodGet, "/v1/users/me",
		map[string]any{ContextKeyUser: account}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var body domain.User
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, account.ID, body.ID)
	assert.Equal(t, account.Name, body.Name)
	assert.Equal(t, 0, users.lookupCalls, "the middleware already resolved the account")
}

func TestUserMeHandlerRequiresIdentity(t *testing.T) {
	api, _, _ := newAccountAPI()

	rec := invokeUserHandler(t, api.UserMeHandler, http.MethodGet, "/v1/users/me", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserMeUpdateHandlerUpdatesAllowedFields(t *testing.T) {
	api, users, _ := newAccountAPI()
	sub := uuid.New()
	account := storedAccount(t, sub)
	users.seed(*account)

	avatar := "https://example.com/new.png"
	body, err := json.Marshal(map[string]any{"nickname": "renamed", "avatar_url": avatar})
	require.NoError(t, err)

	rec := invokeUserHandler(t, api.UserMeUpdateHandler, http.MethodPut, "/v1/users/me",
		map[string]any{ContextKeyUser: account}, body)
	require.Equal(t, http.StatusOK, rec.Code)

	stored, err := users.GetByID(t.Context(), account.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", stored.Name)
	require.NotNil(t, stored.AvatarURL)
	assert.Equal(t, avatar, *stored.AvatarURL)
	assert.Equal(t, account.Email, stored.Email, "the email is not client-owned")
	require.NotNil(t, stored.SupabaseUserID)
	assert.Equal(t, sub, *stored.SupabaseUserID, "the mapping is not client-owned")
}

func TestUserMeUpdateHandlerRejectsInvalidBody(t *testing.T) {
	api, users, _ := newAccountAPI()
	account := storedAccount(t, uuid.New())
	users.seed(*account)

	rec := invokeUserHandler(t, api.UserMeUpdateHandler, http.MethodPut, "/v1/users/me",
		map[string]any{ContextKeyUser: account}, []byte(`{"avatar_url":"https://example.com/a.png"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserMeUpdateHandlerRequiresIdentity(t *testing.T) {
	api, _, _ := newAccountAPI()

	body, err := json.Marshal(map[string]string{"nickname": "renamed"})
	require.NoError(t, err)

	rec := invokeUserHandler(t, api.UserMeUpdateHandler, http.MethodPut, "/v1/users/me", nil, body)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestUserMeUpdateHandlerReportsMissingAccount(t *testing.T) {
	api, users, _ := newAccountAPI()
	account := storedAccount(t, uuid.New())
	users.seed(*account)
	users.hide(account.ID)

	body, err := json.Marshal(map[string]string{"nickname": "renamed"})
	require.NoError(t, err)

	rec := invokeUserHandler(t, api.UserMeUpdateHandler, http.MethodPut, "/v1/users/me",
		map[string]any{ContextKeyUser: account}, body)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUserMeUpdateHandlerReportsConflict(t *testing.T) {
	api, users, _ := newAccountAPI()
	account := storedAccount(t, uuid.New())
	users.seed(*account)
	users.updateErr = domain.ErrDuplicateEntry

	body, err := json.Marshal(map[string]string{"nickname": "renamed"})
	require.NoError(t, err)

	rec := invokeUserHandler(t, api.UserMeUpdateHandler, http.MethodPut, "/v1/users/me",
		map[string]any{ContextKeyUser: account}, body)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// TestUserUpdateHandlerKeepsServerOwnedFields covers the raw update route: it
// binds the whole model, so the server-owned fields must come from the stored
// row. Otherwise a caller could link itself to a Supabase subject, promote
// itself, or take over another user's email.
func TestUserUpdateHandlerKeepsServerOwnedFields(t *testing.T) {
	api, users, _ := newAccountAPI()

	ownerSub := uuid.New()
	owner := storedAccount(t, ownerSub)
	owner.Role = domain.UserRoleUser
	users.seed(*owner)

	attackerSub := uuid.New()
	body, err := json.Marshal(map[string]any{
		"name":             "renamed by update",
		"email":            "attacker@example.com",
		"role":             "admin",
		"supabase_user_id": attackerSub.String(),
		"points":           9999,
	})
	require.NoError(t, err)

	rec := invokeUserHandler(t, api.UserUpdateHandler, http.MethodPut, "/v1/users/"+owner.ID.String(), nil, body)
	require.Equal(t, http.StatusOK, rec.Code)

	stored, err := users.GetByID(t.Context(), owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed by update", stored.Name, "the name stays client-owned")
	assert.Equal(t, owner.Email, stored.Email, "the email is server-owned")
	assert.Equal(t, domain.UserRoleUser, stored.Role, "the role is server-owned")
	assert.Equal(t, 0, stored.Points, "points are server-owned")
	require.NotNil(t, stored.SupabaseUserID)
	assert.Equal(t, ownerSub, *stored.SupabaseUserID, "the Supabase mapping is never set from a request body")
}
