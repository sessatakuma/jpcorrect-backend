package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TopicKind is how a topic bank entry is used (spec §2.2).
type TopicKind string

const (
	TopicKindAnnounce TopicKind = "announce"
	TopicKindRandom   TopicKind = "random"
)

// TopicDifficulty is the difficulty of a topic bank entry (spec §2.2).
type TopicDifficulty string

const (
	TopicDifficultyEasy   TopicDifficulty = "easy"
	TopicDifficultyMedium TopicDifficulty = "medium"
	TopicDifficultyHard   TopicDifficulty = "hard"
)

// Topic is one entry of the topic bank (spec §2.2). Content is maintained
// through the platform back office; there is no topic CRUD API in the MVP.
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type Topic struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	Kind        TopicKind       `gorm:"not null" json:"kind"`
	TitleJP     string          `gorm:"type:text;not null" json:"title_jp"`
	Difficulty  TopicDifficulty `gorm:"default:medium" json:"difficulty"`
	HintVocab   datatypes.JSON  `gorm:"type:jsonb" json:"hint_vocab"`
	HintGrammar datatypes.JSON  `gorm:"type:jsonb" json:"hint_grammar"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   gorm.DeletedAt  `gorm:"index" json:"deleted_at"`
}

// ReportThemeSuggestion is a reference theme the guild leader can pick when
// scheduling report practice (spec §2.2). It is maintained through the platform
// back office in the MVP.
type ReportThemeSuggestion struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string    `gorm:"not null" json:"title"`
	Description *string   `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
