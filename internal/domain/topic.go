package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type TopicKind string

const (
	TopicKindAnnounce TopicKind = "announce"
	TopicKindRandom   TopicKind = "random"
)

type Topic struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"topic_id"`
	Kind        TopicKind      `gorm:"default:announce" json:"kind"`
	TitleJP     string         `gorm:"type:text" json:"title_jp"`
	Difficulty  string         `gorm:"default:medium" json:"difficulty"`
	HintVocab   datatypes.JSON `gorm:"type:jsonb" json:"hint_vocab"`
	HintGrammar datatypes.JSON `gorm:"type:jsonb" json:"hint_grammar"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

type TopicRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Topic, error)
	SearchAnnounce(ctx context.Context, keyword string) ([]*Topic, error)
	GetRandom(ctx context.Context) (*Topic, error)
	List(ctx context.Context) ([]*Topic, error)
}
