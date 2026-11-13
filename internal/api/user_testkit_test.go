package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// fakeUserRepo is an in-memory domain.UserRepository with the database
// behaviour the account handlers depend on: the Supabase subject maps to one
// row, the email index is unique, and the first-login insert keeps the first
// row when it conflicts on the subject. It embeds the interface so that any
// unexpected call panics instead of silently succeeding.
type fakeUserRepo struct {
	domain.UserRepository

	mu      sync.Mutex
	byID    map[uuid.UUID]*domain.User
	bySub   map[uuid.UUID]uuid.UUID
	byEmail map[string]uuid.UUID
	// hidden marks soft-deleted rows: the lookup cannot see them, but the
	// subject and the email stay taken.
	hidden map[uuid.UUID]bool

	lookupErr error
	createErr error
	updateErr error

	lookupCalls int
	createCalls int
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    map[uuid.UUID]*domain.User{},
		bySub:   map[uuid.UUID]uuid.UUID{},
		byEmail: map[string]uuid.UUID{},
		hidden:  map[uuid.UUID]bool{},
	}
}

// seed stores user as if it already existed, bypassing the uniqueness rules
// only where the test says so (for example a NULL Supabase mapping).
func (f *fakeUserRepo) seed(user domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored := user
	if stored.ID == uuid.Nil {
		stored.ID = uuid.New()
	}
	f.byID[stored.ID] = &stored
	if stored.SupabaseUserID != nil {
		f.bySub[*stored.SupabaseUserID] = stored.ID
	}
	if stored.Email != "" {
		f.byEmail[stored.Email] = stored.ID
	}
}

// hide makes a stored row behave like a soft-deleted one.
func (f *fakeUserRepo) hide(id uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hidden[id] = true
}

func (f *fakeUserRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byID)
}

func (f *fakeUserRepo) GetByID(_ context.Context, userID uuid.UUID) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	user, exists := f.byID[userID]
	if !exists || f.hidden[userID] {
		return nil, domain.ErrNotFound
	}
	copied := *user
	return &copied, nil
}

func (f *fakeUserRepo) GetBySupabaseUserID(_ context.Context, supabaseUserID uuid.UUID) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lookupCalls++
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}

	id, exists := f.bySub[supabaseUserID]
	if !exists || f.hidden[id] {
		return nil, domain.ErrNotFound
	}
	copied := *f.byID[id]
	return &copied, nil
}

func (f *fakeUserRepo) CreateIfAbsentBySupabaseUserID(_ context.Context, user *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}

	if user.SupabaseUserID == nil {
		return domain.ErrDuplicateEntry
	}
	if _, exists := f.bySub[*user.SupabaseUserID]; exists {
		// ON CONFLICT (supabase_user_id) DO NOTHING: the first row wins.
		return nil
	}
	if _, exists := f.byEmail[user.Email]; exists {
		return domain.ErrDuplicateEntry
	}

	stored := *user
	f.byID[stored.ID] = &stored
	f.bySub[*stored.SupabaseUserID] = stored.ID
	f.byEmail[stored.Email] = stored.ID
	return nil
}

func (f *fakeUserRepo) Update(_ context.Context, user *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.updateErr != nil {
		return f.updateErr
	}
	if _, exists := f.byID[user.ID]; !exists {
		return domain.ErrNotFound
	}

	stored := *user
	f.byID[stored.ID] = &stored
	return nil
}

// newAccountAPI builds an API around a fake user repository and a mock guild
// repository, which is everything the identity and account tests touch.
func newAccountAPI() (*API, *fakeUserRepo, *mockGuildRepo) {
	users := newFakeUserRepo()
	guilds := &mockGuildRepo{}
	return &API{userRepo: users, guildRepo: guilds}, users, guilds
}

// invokeUserHandler mounts handler on a fresh gin engine with the given context
// values, exactly as AuthMiddleware would have set them. UUID segments of path
// become the :id route param, the way the router registers them.
func invokeUserHandler(
	t *testing.T,
	handler gin.HandlerFunc,
	method, path string,
	values map[string]any,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if _, err := uuid.Parse(part); err == nil {
			parts[i] = ":id"
		}
	}

	r := gin.New()
	r.Handle(method, strings.Join(parts, "/"), func(c *gin.Context) {
		for key, value := range values {
			c.Set(key, value)
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
