package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUserTestAPI builds an API whose only dependency is the user repository.
func newUserTestAPI(repo *mockUserRepo) *API {
	return &API{userRepo: repo}
}

// invokeUserHandler mounts handler on a route pattern with the context values
// AuthMiddleware would have set. Passing a nil user leaves the request
// unauthenticated (no "user" value), which handlers must reject with 401.
func invokeUserHandler(
	t *testing.T,
	api *API,
	method, pattern, path string,
	user *domain.User,
	auth *supabaseAuth,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	handler := map[string]gin.HandlerFunc{
		http.MethodGet + " " + "/v1/users/me":         api.UserMeHandler,
		http.MethodPut + " " + "/v1/users/me":         api.UserMeUpdateHandler,
		http.MethodPost + " " + "/v1/users/init":      api.UserInitHandler,
		http.MethodGet + " " + "/v1/users/:id":        api.UserGetHandler,
		http.MethodGet + " " + "/v1/users/name/:name": api.UserGetByNameHandler,
	}[method+" "+pattern]
	require.NotNil(t, handler, "no handler for %s %s", method, pattern)

	r := gin.New()
	r.Handle(method, pattern, func(c *gin.Context) {
		if user != nil {
			c.Set(userContextKey, user)
		}
		if auth != nil {
			c.Set(supabaseAuthContextKey, auth)
		}
		handler(c)
	})

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUserMeHandler(t *testing.T) {
	t.Run("no resolved user → 401", func(t *testing.T) {
		rec := invokeUserHandler(t, newUserTestAPI(&mockUserRepo{}),
			http.MethodGet, "/v1/users/me", "/v1/users/me", nil, nil, nil)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("returns the caller's own profile", func(t *testing.T) {
		user := storedUser(t, uuid.New(), "me@example.com")
		rec := invokeUserHandler(t, newUserTestAPI(&mockUserRepo{}),
			http.MethodGet, "/v1/users/me", "/v1/users/me", user, nil, nil)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var body domain.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, user.ID, body.ID)
		assert.Equal(t, user.Email, body.Email)
		assert.Equal(t, user.Role, body.Role)
	})
}

func TestUserMeUpdateHandler(t *testing.T) {
	t.Run("no resolved user → 401", func(t *testing.T) {
		repo := &mockUserRepo{}
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", nil, nil, []byte(`{"name":"new"}`))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.updateCalls)
		assert.Empty(t, repo.profileUpdates)
	})

	t.Run("updates name and avatar only", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{getByIDResult: caller}

		body := []byte(`{"name":"renamed","avatar_url":"https://example.com/new.png"}`)
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, body)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.profileUpdates, 1)

		written := repo.profileUpdates[0]
		assert.Equal(t, caller.ID, written.userID)
		require.NotNil(t, written.name)
		assert.Equal(t, "renamed", *written.name)
		require.NotNil(t, written.avatarURL)
		assert.Equal(t, "https://example.com/new.png", *written.avatarURL)

		// Saving the whole row would write this request's copy of every
		// server-owned column back over whatever is stored now.
		assert.Empty(t, repo.updateCalls, "the profile write must not save the whole row")

		// Server-owned fields survive untouched in the stored row, and the
		// response carries the committed row.
		assert.Equal(t, "stored name", caller.Name, "the handler must not mutate the row it was handed")
		assert.Equal(t, "me@example.com", caller.Email)
		assert.Equal(t, domain.UserRoleUser, caller.Role)
		assert.Equal(t, domain.UserStatusActive, caller.Status)
		assert.Equal(t, 42, caller.Points)
		require.NotNil(t, caller.SupabaseUserID)

		var response domain.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		assert.Equal(t, "renamed", response.Name)
		assert.Equal(t, "me@example.com", response.Email)
		assert.Equal(t, domain.UserRoleUser, response.Role)
		assert.Equal(t, domain.UserStatusActive, response.Status)
		assert.Equal(t, 42, response.Points)
		require.NotNil(t, response.SupabaseUserID)
		assert.Equal(t, *caller.SupabaseUserID, *response.SupabaseUserID)
	})

	t.Run("server-owned fields in the body are ignored", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{getByIDResult: caller}

		body := []byte(`{"name":"renamed","email":"attacker@example.com","role":"admin",` +
			`"status":"banned","points":9999,"supabase_user_id":"9f3e8a52-4b1c-4d6f-8e07-2a5b9c1d3e4f","is_email_verified":false}`)
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, body)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.profileUpdates, 1)

		written := repo.profileUpdates[0]
		require.NotNil(t, written.name)
		assert.Equal(t, "renamed", *written.name)
		assert.Nil(t, written.avatarURL, "an omitted avatar_url is not rewritten")
		assert.Empty(t, repo.updateCalls)

		var response domain.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		assert.Equal(t, "me@example.com", response.Email, "email is server-owned")
		assert.Equal(t, domain.UserRoleUser, response.Role, "a user cannot promote itself")
		assert.Equal(t, domain.UserStatusActive, response.Status)
		assert.Equal(t, 42, response.Points)
		assert.Equal(t, caller.SupabaseUserID, response.SupabaseUserID)
		assert.True(t, response.IsEmailVerified)
	})

	t.Run("empty avatar_url clears the column", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{getByIDResult: caller}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, []byte(`{"avatar_url":""}`))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.profileUpdates, 1)
		require.NotNil(t, repo.profileUpdates[0].avatarURL, "an empty avatar_url is a write, not an omission")
		assert.Empty(t, *repo.profileUpdates[0].avatarURL)
		assert.Nil(t, repo.profileUpdates[0].name)
	})

	t.Run("empty body writes no column", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{getByIDResult: caller}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, []byte(`{}`))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.profileUpdates, 1)
		assert.Nil(t, repo.profileUpdates[0].name)
		assert.Nil(t, repo.profileUpdates[0].avatarURL)
		assert.Empty(t, repo.updateCalls)
	})

	t.Run("row that no longer exists → 404", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{profileUpdateErr: domain.ErrNotFound}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, []byte(`{"name":"renamed"}`))

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "NOT_FOUND", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("repository failure → 500", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{profileUpdateErr: errors.New("connection reset")}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, []byte(`{"name":"renamed"}`))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Equal(t, "INTERNAL_ERROR", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		caller := storedUser(t, uuid.New(), "me@example.com")
		repo := &mockUserRepo{getByIDResult: caller}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPut, "/v1/users/me", "/v1/users/me", caller, nil, []byte(`{"name":""}`))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.updateCalls)
		assert.Empty(t, repo.profileUpdates)
	})
}

func TestUserGetHandlerPublicProfile(t *testing.T) {
	t.Run("invalid id → 400", func(t *testing.T) {
		rec := invokeUserHandler(t, newUserTestAPI(&mockUserRepo{}),
			http.MethodGet, "/v1/users/:id", "/v1/users/not-a-uuid", &domain.User{}, nil, nil)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("unknown id → 404", func(t *testing.T) {
		rec := invokeUserHandler(t, newUserTestAPI(&mockUserRepo{}),
			http.MethodGet, "/v1/users/:id", "/v1/users/"+uuid.NewString(), &domain.User{}, nil, nil)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "NOT_FOUND", decodeEnvelope(t, rec).Error.Code)
	})

	t.Run("returns only allowlisted fields", func(t *testing.T) {
		target := storedUser(t, uuid.New(), "target@example.com")
		repo := &mockUserRepo{getByIDResult: target}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodGet, "/v1/users/:id", "/v1/users/"+target.ID.String(), &domain.User{}, nil, nil)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assertPublicProfileShape(t, rec.Body.Bytes(), target)
	})
}

func TestUserGetByNameHandlerPublicProfile(t *testing.T) {
	first := storedUser(t, uuid.New(), "one@example.com")
	second := storedUser(t, uuid.New(), "two@example.com")
	repo := &mockUserRepo{getByNameResult: []*domain.User{first, second}}

	rec := invokeUserHandler(t, newUserTestAPI(repo),
		http.MethodGet, "/v1/users/name/:name", "/v1/users/name/stored%20name", &domain.User{}, nil, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var profiles []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profiles))
	require.Len(t, profiles, 2)
	assert.Equal(t, first.ID.String(), profiles[0]["user_id"])
	assert.Equal(t, second.ID.String(), profiles[1]["user_id"])
	for _, profile := range profiles {
		assertNoExcludedProfileKeys(t, profile)
	}
}

// assertPublicProfileShape checks the GET /users/:id body: the allowlisted keys
// are present and none of the private ones leak.
func assertPublicProfileShape(t *testing.T, body []byte, user *domain.User) {
	t.Helper()

	var profile map[string]any
	require.NoError(t, json.Unmarshal(body, &profile))

	assert.Equal(t, user.ID.String(), profile["user_id"])
	assert.Equal(t, user.Name, profile["name"])
	assert.Equal(t, float64(user.Level), profile["level"])
	assert.NotContains(t, profile, "deleted_at")
	assertNoExcludedProfileKeys(t, profile)
}

// assertNoExcludedProfileKeys pins the allowlist from spec §3.1.2: none of
// these keys may ever appear in a public profile.
func assertNoExcludedProfileKeys(t *testing.T, profile map[string]any) {
	t.Helper()

	for _, key := range []string{
		"email",
		"status",
		"role",
		"is_email_verified",
		"supabase_user_id",
		"password_hash",
		"deleted_at",
		"points",
		"late_streak",
		"timezone",
	} {
		assert.NotContains(t, profile, key, "public profile must not expose %q", key)
	}
}

func TestUserInitHandler(t *testing.T) {
	t.Run("no verified token in context → 401", func(t *testing.T) {
		repo := &mockUserRepo{}
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil, nil, []byte(`{}`))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "UNAUTHORIZED", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.createIfAbsentCalls)
	})

	t.Run("unusable subject → 400 without a repo call", func(t *testing.T) {
		for _, tc := range []struct{ name, subject string }{
			{"missing subject", ""},
			{"subject that is not a UUID", "not-a-uuid"},
			{"nil UUID subject", uuid.Nil.String()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repo := &mockUserRepo{}
				rec := invokeUserHandler(t, newUserTestAPI(repo),
					http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
					&supabaseAuth{Subject: tc.subject, Email: "first@example.com"}, []byte(`{}`))

				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
				assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code)
				assert.Empty(t, repo.getBySupabaseUserIDCalls, "an unusable subject must not read a row")
				assert.Empty(t, repo.createIfAbsentCalls, "an unusable subject must not write a row")
			})
		}
	})

	t.Run("token without email → 400", func(t *testing.T) {
		repo := &mockUserRepo{}
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
			&supabaseAuth{Subject: uuid.NewString()}, []byte(`{}`))

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "INVALID_REQUEST", decodeEnvelope(t, rec).Error.Code)
		assert.Empty(t, repo.createIfAbsentCalls)
	})

	t.Run("creates the row from the token and returns it", func(t *testing.T) {
		sub := uuid.New()
		created := storedUser(t, sub, "first@example.com")
		repo := &mockUserRepo{getBySupabaseUserIDResult: created}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
			&supabaseAuth{Subject: sub.String(), Email: "First@Example.com"}, []byte(`{}`))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.createIfAbsentCalls, 1)

		candidate := repo.createIfAbsentCalls[0]
		require.NotNil(t, candidate.SupabaseUserID)
		assert.Equal(t, sub, *candidate.SupabaseUserID)
		assert.Equal(t, "First@Example.com", candidate.Email)
		assert.Equal(t, "First", candidate.Name, "a new account is named after its email prefix")
		assert.NotEqual(t, uuid.Nil, candidate.ID)
		// New accounts start on the model defaults: the request never sets them.
		assert.Empty(t, candidate.Role)
		assert.Empty(t, candidate.Status)
		assert.Zero(t, candidate.Points)

		var body domain.User
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, created.ID, body.ID)
	})

	t.Run("a long Unicode email prefix is capped at 100 runes", func(t *testing.T) {
		sub := uuid.New()
		email := strings.Repeat("あ", 150) + "@example.com"
		created := storedUser(t, sub, email)
		repo := &mockUserRepo{getBySupabaseUserIDResult: created}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
			&supabaseAuth{Subject: sub.String(), Email: email}, []byte(`{}`))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.createIfAbsentCalls, 1)

		name := repo.createIfAbsentCalls[0].Name
		assert.Equal(t, strings.Repeat("あ", 100), name)
		assert.Equal(t, 100, utf8.RuneCountInString(name), "the cap counts runes")
		assert.Equal(t, 300, len(name), "a byte cap would cut inside a rune")
		assert.True(t, utf8.ValidString(name))
	})

	t.Run("role sent in the body is ignored", func(t *testing.T) {
		created := storedUser(t, uuid.New(), "first@example.com")
		repo := &mockUserRepo{getBySupabaseUserIDResult: created}

		body := []byte(`{"role":"admin","name":"attacker","email":"attacker@example.com","points":9999}`)
		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
			&supabaseAuth{Subject: uuid.NewString(), Email: "first@example.com"}, body)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Len(t, repo.createIfAbsentCalls, 1)
		assert.Empty(t, repo.createIfAbsentCalls[0].Role)
		assert.Equal(t, "first@example.com", repo.createIfAbsentCalls[0].Email,
			"the email comes from the token, never from the body")
		assert.Equal(t, "first", repo.createIfAbsentCalls[0].Name,
			"a display name in the body is ignored; the email prefix is the default")
		assert.Zero(t, repo.createIfAbsentCalls[0].Points)
	})

	t.Run("email owned by another row → 409", func(t *testing.T) {
		repo := &mockUserRepo{createIfAbsentErr: domain.ErrDuplicateEntry}

		rec := invokeUserHandler(t, newUserTestAPI(repo),
			http.MethodPost, "/v1/users/init", "/v1/users/init", nil,
			&supabaseAuth{Subject: uuid.NewString(), Email: "taken@example.com"}, []byte(`{}`))

		require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		env := decodeEnvelope(t, rec)
		assert.Equal(t, "CONFLICT", env.Error.Code)
		assert.Equal(t, "email is already in use by another account", env.Error.Message)
		assert.Empty(t, repo.getBySupabaseUserIDCalls, "a conflict must not resolve or link another row")
	})
}

func TestDefaultDisplayName(t *testing.T) {
	long := strings.Repeat("あ", 150)

	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"prefix before the at sign", "taro.yamada@example.com", "taro.yamada"},
		{"plus tag is kept", "taro+jp@example.com", "taro+jp"},
		{"last at sign splits a quoted local part", `"a@b"@example.com`, `"a@b"`},
		{"no local part keeps the address", "@example.com", "@example.com"},
		{"no at sign keeps the address", "not-an-email", "not-an-email"},
		{"exactly 100 runes is unchanged", strings.Repeat("あ", 100) + "@example.com", strings.Repeat("あ", 100)},
		{"over 100 ASCII runes is cut", strings.Repeat("a", 101) + "@example.com", strings.Repeat("a", 100)},
		{"cut to the PUT /me limit in runes", long + "@example.com", strings.Repeat("あ", 100)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultDisplayName(tc.email)
			assert.Equal(t, tc.want, got)
			assert.NotEmpty(t, got)
			assert.LessOrEqual(t, len([]rune(got)), 100)
		})
	}
}
