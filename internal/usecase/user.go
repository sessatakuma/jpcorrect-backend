package usecase

import (
	"context"
	"jpcorrect-backend/internal/domain"
)

type userUsecase struct {
	userRepo domain.UserRepository
}

func NewUserUsecase(repo domain.UserRepository) domain.UserUsecase {
	return &userUsecase{userRepo: repo}
}

func (u *userUsecase) InitUser(ctx context.Context, supabaseID string, email string) (*domain.User, error) {
	user, err := u.userRepo.GetBySupabaseID(ctx, supabaseID)

	if err != nil {
		newUser := &domain.User{
			SupabaseID: supabaseID,
			Email:      email,
		}
		if err := u.userRepo.Create(ctx, newUser); err != nil {
			return nil, err
		}
		return newUser, nil
	}
	// If user already exists, return it
	return user, nil
}
