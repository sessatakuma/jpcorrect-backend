package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"jpcorrect-backend/internal/domain"
)

type gormTopicRepository struct {
	db *gorm.DB
}

func NewGormTopicRepository(db *gorm.DB) domain.TopicRepository {
	return &gormTopicRepository{db: db}
}

func (r *gormTopicRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Topic, error) {
	var topic domain.Topic
	err := r.db.WithContext(ctx).First(&topic, "id = ?", id).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &topic, nil
}

func (r *gormTopicRepository) SearchAnnounce(ctx context.Context, keyword string) ([]*domain.Topic, error) {
	var topics []*domain.Topic
	err := r.db.WithContext(ctx).
		Where("kind = ? AND title_jp LIKE ?", domain.TopicKindAnnounce, "%"+keyword+"%").
		Find(&topics).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return topics, nil
}

func (r *gormTopicRepository) GetRandom(ctx context.Context) (*domain.Topic, error) {
	var topic domain.Topic
	err := r.db.WithContext(ctx).Order("RANDOM()").First(&topic).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return &topic, nil
}

func (r *gormTopicRepository) List(ctx context.Context) ([]*domain.Topic, error) {
	var topics []*domain.Topic
	err := r.db.WithContext(ctx).Find(&topics).Error
	if err != nil {
		return nil, MapGormError(err)
	}
	return topics, nil
}
