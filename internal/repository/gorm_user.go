package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"jpcorrect-backend/internal/domain"
)

type gormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository creates a new GORM-based user repository.
func NewGormUserRepository(db *gorm.DB) domain.UserRepository {
	return &gormUserRepository{db: db}
}

func (r *gormUserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &user, nil
}

func (r *gormUserRepository) GetByName(ctx context.Context, name string) ([]*domain.User, error) {
	var users []*domain.User
	err := r.db.WithContext(ctx).Where("name = ?", name).Find(&users).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return users, nil
}

func (r *gormUserRepository) GetBySupabaseUserID(ctx context.Context, supabaseUserID uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("supabase_user_id = ?", supabaseUserID).First(&user).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &user, nil
}

// CreateIfAbsentBySupabaseUserID is the §6.3 idempotent insert: one
// `INSERT … ON CONFLICT (supabase_user_id) DO NOTHING`. Only a conflict on
// supabase_user_id is ignored; a concurrent request for the same sub blocks on
// the unique index and then writes nothing, so every caller reads the same row
// afterwards. Any other unique violation (e.g. the email index) still fails and
// reaches the caller as domain.ErrDuplicateEntry.
func (r *gormUserRepository) CreateIfAbsentBySupabaseUserID(ctx context.Context, user *domain.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "supabase_user_id"}},
			DoNothing: true,
		}).
		Create(user).Error)
}

func (r *gormUserRepository) Create(ctx context.Context, user *domain.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(user).Error)
}

func (r *gormUserRepository) Update(ctx context.Context, user *domain.User) error {
	return MapGormError(r.db.WithContext(ctx).Save(user).Error)
}

func (r *gormUserRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	// GORM soft delete
	return MapGormError(r.db.WithContext(ctx).Delete(&domain.User{}, "id = ?", userID).Error)
}
