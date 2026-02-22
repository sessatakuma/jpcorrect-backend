package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormEventRepository struct {
	db *gorm.DB
}

func NewGormEventRepository(db *gorm.DB) domain.EventRepository {
	return &gormEventRepository{db: db}
}

func (r *gormEventRepository) GetByID(ctx context.Context, eventID uuid.UUID) (*domain.Event, error) {
	var event domain.Event
	err := r.db.WithContext(ctx).First(&event, "id = ?", eventID).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &event, nil
}

func (r *gormEventRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*domain.Event, error) {
	var events []*domain.Event
	err := r.db.WithContext(ctx).
		Joins("JOIN event_attendee ON event_attendee.event_id = event.id").
		Where("event_attendee.user_id = ?", userID).
		Find(&events).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return events, nil
}

func (r *gormEventRepository) Create(ctx context.Context, event *domain.Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	return MapGormError(r.db.WithContext(ctx).Create(event).Error)
}

func (r *gormEventRepository) Update(ctx context.Context, event *domain.Event) error {
	return MapGormError(r.db.WithContext(ctx).Save(event).Error)
}

func (r *gormEventRepository) Delete(ctx context.Context, eventID uuid.UUID) error {
	var attendeeCount int64
	err := r.db.WithContext(ctx).Model(&domain.EventAttendee{}).Where("event_id = ?", eventID).Count(&attendeeCount).Error
	if err != nil {
		return MapGormError(err)
	}
	if attendeeCount > 0 {
		return errors.New("cannot delete event: has attendees")
	}

	var transcriptCount int64
	err = r.db.WithContext(ctx).Model(&domain.Transcript{}).Where("event_id = ?", eventID).Count(&transcriptCount).Error
	if err != nil {
		return MapGormError(err)
	}
	if transcriptCount > 0 {
		return errors.New("cannot delete event: has transcripts")
	}

	var mistakeCount int64
	err = r.db.WithContext(ctx).Model(&domain.Mistake{}).Where("event_id = ?", eventID).Count(&mistakeCount).Error
	if err != nil {
		return MapGormError(err)
	}
	if mistakeCount > 0 {
		return errors.New("cannot delete event: has mistakes")
	}

	return MapGormError(r.db.WithContext(ctx).Delete(&domain.Event{}, "id = ?", eventID).Error)
}
