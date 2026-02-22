package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

func (r *gormUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
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
