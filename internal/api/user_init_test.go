package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jpcorrect-backend/internal/domain"
)

type mockUserRepo struct {
	domain.UserRepository

	bySupabase map[string]*domain.User
	byEmail    map[string]*domain.User
	byID       map[uuid.UUID]*domain.User
	created    []*domain.User
	updated    []*domain.User
	createErr  error
}

func newMockUserRepo(users ...*domain.User) *mockUserRepo {
	m := &mockUserRepo{bySupabase: map[string]*domain.User{}, byEmail: map[string]*domain.User{}, byID: map[uuid.UUID]*domain.User{}}
	for _, user := range users {
		m.byID[user.ID] = user
		if user.SupabaseUserID != "" {
			m.bySupabase[user.SupabaseUserID] = user
		}
		m.byEmail[user.Email] = user
	}
	return m
}

func (m *mockUserRepo) GetBySupabaseUserID(_ context.Context, supabaseUserID string) (*domain.User, error) {
	if user, ok := m.bySupabase[supabaseUserID]; ok {
		return user, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if user, ok := m.byID[id]; ok {
		return user, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if user, ok := m.byEmail[email]; ok {
		return user, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) Create(_ context.Context, user *domain.User) error {
	if m.createErr != nil {
		return m.createErr
	}
	user.ID = uuid.New()
	m.created = append(m.created, user)
	return nil
}

func (m *mockUserRepo) Update(_ context.Context, user *domain.User) error {
	m.updated = append(m.updated, user)
	return nil
}

func TestRequireLocalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &domain.User{ID: uuid.New(), Email: "member@example.com", SupabaseUserID: "supabase-1"}
	api := &API{userRepo: newMockUserRepo(user)}

	t.Run("resolves the local user", func(t *testing.T) {
		var gotID any
		var gotUser *domain.User
		r := gin.New()
		r.GET("/protected", func(c *gin.Context) { c.Set("supabaseUserID", "supabase-1") }, api.RequireLocalUser(), func(c *gin.Context) {
			gotID, _ = c.Get("userID")
			gotUser, _ = c.MustGet("user").(*domain.User)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, user.ID, gotID)
		assert.Equal(t, user, gotUser)
	})

	t.Run("rejects an unregistered subject", func(t *testing.T) {
		r := gin.New()
		r.GET("/protected", func(c *gin.Context) { c.Set("supabaseUserID", "unknown") }, api.RequireLocalUser(), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("rejects a missing subject", func(t *testing.T) {
		r := gin.New()
		r.GET("/protected", api.RequireLocalUser(), func(c *gin.Context) { c.Status(http.StatusOK) })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestUserInitHandlerProvisionsOrLinksUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	call := func(api *API, subject, email string, emailVerified *bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/users/init", nil)
		if subject != "" {
			c.Set("supabaseUserID", subject)
		}
		if email != "" {
			c.Set("supabaseEmail", email)
		}
		if emailVerified != nil {
			c.Set("supabaseEmailVerified", *emailVerified)
		}
		api.UserInitHandler(c)
		return w
	}

	t.Run("creates a local user on first login", func(t *testing.T) {
		repo := newMockUserRepo()
		api := &API{userRepo: repo}

		w := call(api, "supabase-new", "new@example.com", nil)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Len(t, repo.created, 1)
		assert.Equal(t, "supabase-new", repo.created[0].SupabaseUserID)
		assert.Equal(t, "new@example.com", repo.created[0].Email)
		assert.Equal(t, "new", repo.created[0].Name)
	})

	t.Run("returns an already linked user", func(t *testing.T) {
		existing := &domain.User{ID: uuid.New(), Email: "member@example.com", SupabaseUserID: "supabase-1"}
		repo := newMockUserRepo(existing)
		api := &API{userRepo: repo}

		w := call(api, "supabase-1", "member@example.com", nil)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, repo.created)
		assert.Empty(t, repo.updated)
	})

	t.Run("refuses a legacy row with an unverified email claim", func(t *testing.T) {
		legacy := &domain.User{ID: uuid.New(), Email: "legacy@example.com", Name: "unchanged"}
		repo := newMockUserRepo(legacy)
		api := &API{userRepo: repo}
		verified := false

		w := call(api, "supabase-legacy", "legacy@example.com", &verified)

		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "email is linked to another account")
		assert.Empty(t, repo.created, "a conflict must not create a second row")
		assert.Empty(t, repo.updated, "a conflict must not touch the legacy row")
		assert.Empty(t, legacy.SupabaseUserID, "the legacy row must stay unlinked")
		assert.Equal(t, "unchanged", legacy.Name)
	})

	t.Run("does not trust a verified email claim either", func(t *testing.T) {
		legacy := &domain.User{ID: uuid.New(), Email: "verified@example.com"}
		repo := newMockUserRepo(legacy)
		api := &API{userRepo: repo}
		verified := true

		w := call(api, "supabase-verified", "verified@example.com", &verified)

		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Empty(t, repo.created)
		assert.Empty(t, repo.updated)
		assert.Empty(t, legacy.SupabaseUserID)
	})

	t.Run("does not steal an email already linked to another subject", func(t *testing.T) {
		other := &domain.User{ID: uuid.New(), Email: "taken@example.com", SupabaseUserID: "supabase-other"}
		repo := newMockUserRepo(other)
		api := &API{userRepo: repo}

		w := call(api, "supabase-new", "taken@example.com", nil)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Empty(t, repo.updated)
		assert.Empty(t, repo.created)
		assert.Equal(t, "supabase-other", other.SupabaseUserID, "the owning row must keep its subject")
	})

	t.Run("rejects a token without subject", func(t *testing.T) {
		api := &API{userRepo: newMockUserRepo()}

		w := call(api, "", "", nil)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("rejects a token without email", func(t *testing.T) {
		api := &API{userRepo: newMockUserRepo()}

		w := call(api, "supabase-new", "", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("propagates create errors", func(t *testing.T) {
		repo := newMockUserRepo()
		repo.createErr = errors.New("db down")
		api := &API{userRepo: repo}

		w := call(api, "supabase-new", "new@example.com", nil)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestUserMeHandlerFallsBackToCallerID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &domain.User{ID: uuid.New(), Email: "member@example.com", SupabaseUserID: "supabase-1"}
	api := &API{userRepo: newMockUserRepo(user)}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	c.Set("userID", user.ID)

	api.UserMeHandler(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "member@example.com")
}
