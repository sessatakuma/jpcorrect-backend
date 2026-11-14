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

	createCalls []*domain.User
	createErr   error

	deleteCalls []uuid.UUID
	deleteErr   error

	profileUpdates      []profileUpdateCall
	profileUpdateErr    error
	profileUpdateResult *domain.User
}

// profileUpdateCall records one UpdateProfile call: the row it targeted and the
// two optional columns it asked the repository to write.
type profileUpdateCall struct {
	userID    uuid.UUID
	name      *string
	avatarURL *string
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

func (m *mockUserRepo) Create(_ context.Context, user *domain.User) error {
	m.createCalls = append(m.createCalls, user)
	if m.createErr != nil {
		return m.createErr
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return nil
}

func (m *mockUserRepo) Delete(_ context.Context, userID uuid.UUID) error {
	m.deleteCalls = append(m.deleteCalls, userID)
	return m.deleteErr
}

// UpdateProfile records the requested columns and answers the way the GORM
// repository does: the stored row with exactly those columns applied, or
// ErrNotFound when the mock holds no row with this id.
func (m *mockUserRepo) UpdateProfile(_ context.Context, userID uuid.UUID, name *string, avatarURL *string) (*domain.User, error) {
	m.profileUpdates = append(m.profileUpdates, profileUpdateCall{userID: userID, name: name, avatarURL: avatarURL})

	if m.profileUpdateErr != nil {
		return nil, m.profileUpdateErr
	}
	if m.profileUpdateResult != nil {
		copied := *m.profileUpdateResult
		return &copied, nil
	}
	if m.getByIDResult == nil || m.getByIDResult.ID != userID {
		return nil, domain.ErrNotFound
	}

	updated := *m.getByIDResult
	if name != nil {
		updated.Name = *name
	}
	if avatarURL != nil {
		updated.AvatarURL = avatarURL
	}
	return &updated, nil
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
