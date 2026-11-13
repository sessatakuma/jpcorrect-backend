package api

import (
	"context"
	"testing"
	"time"

	"jpcorrect-backend/internal/domain"

	"github.com/google/uuid"
)

// mockUserRepo implements domain.UserRepository by embedding the interface and
// overriding only the methods the handlers under test call. Any non-overridden
// method panics on the nil embedded interface — acceptable for focused handler
// tests.
type mockUserRepo struct {
	domain.UserRepository

	getByIDResult *domain.User
	getByIDErr    error

	getByNameResult []*domain.User
	getByNameErr    error

	getBySupabaseUserIDCalls  []uuid.UUID
	getBySupabaseUserIDResult *domain.User
	getBySupabaseUserIDErr    error

	createIfAbsentCalls []*domain.User
	createIfAbsentErr   error

	updateCalls []*domain.User
	updateErr   error
}

func (m *mockUserRepo) GetByID(_ context.Context, userID uuid.UUID) (*domain.User, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if m.getByIDResult == nil {
		return nil, domain.ErrNotFound
	}
	if m.getByIDResult.ID != userID {
		return nil, domain.ErrNotFound
	}
	copied := *m.getByIDResult
	return &copied, nil
}

func (m *mockUserRepo) GetByName(_ context.Context, _ string) ([]*domain.User, error) {
	return m.getByNameResult, m.getByNameErr
}

func (m *mockUserRepo) GetBySupabaseUserID(_ context.Context, supabaseUserID uuid.UUID) (*domain.User, error) {
	m.getBySupabaseUserIDCalls = append(m.getBySupabaseUserIDCalls, supabaseUserID)
	if m.getBySupabaseUserIDErr != nil {
		return nil, m.getBySupabaseUserIDErr
	}
	if m.getBySupabaseUserIDResult == nil {
		return nil, domain.ErrNotFound
	}
	copied := *m.getBySupabaseUserIDResult
	return &copied, nil
}

func (m *mockUserRepo) CreateIfAbsentBySupabaseUserID(_ context.Context, user *domain.User) error {
	m.createIfAbsentCalls = append(m.createIfAbsentCalls, user)
	return m.createIfAbsentErr
}

func (m *mockUserRepo) Update(_ context.Context, user *domain.User) error {
	m.updateCalls = append(m.updateCalls, user)
	return m.updateErr
}

// storedUser is the row every "already initialized" test resolves.
func storedUser(t *testing.T, sub uuid.UUID, email string) *domain.User {
	t.Helper()

	avatar := "https://example.com/avatar.png"
	return &domain.User{
		ID:              uuid.New(),
		Email:           email,
		Name:            "stored name",
		AvatarURL:       &avatar,
		IsEmailVerified: true,
		Role:            domain.UserRoleUser,
		Status:          domain.UserStatusActive,
		Timezone:        "Asia/Taipei",
		SupabaseUserID:  &sub,
		Points:          42,
		Level:           3,
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now(),
	}
}
