package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormEventAttendeeRepository struct {
	db *gorm.DB
}

func NewGormEventAttendeeRepository(db *gorm.DB) domain.EventAttendeeRepository {
	return &gormEventAttendeeRepository{db: db}
}

func (r *gormEventAttendeeRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.EventAttendee, error) {
	var attendee domain.EventAttendee
	err := r.db.WithContext(ctx).First(&attendee, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &attendee, nil
}

func (r *gormEventAttendeeRepository) GetByEventID(ctx context.Context, eventID uuid.UUID) ([]*domain.EventAttendee, error) {
	var attendees []*domain.EventAttendee
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Find(&attendees).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return attendees, nil
}

func (r *gormEventAttendeeRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.EventAttendee, error) {
	var attendees []*domain.EventAttendee
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&attendees).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return attendees, nil
}

func (r *gormEventAttendeeRepository) Create(ctx context.Context, attendee *domain.EventAttendee) error {
	err := r.db.WithContext(ctx).Create(attendee).Error
	return MapGormError(err)
}

func (r *gormEventAttendeeRepository) Update(ctx context.Context, attendee *domain.EventAttendee) error {
	err := r.db.WithContext(ctx).Save(attendee).Error
	return MapGormError(err)
}

func (r *gormEventAttendeeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).Delete(&domain.EventAttendee{}, "id = ?", id).Error
	return MapGormError(err)
}
