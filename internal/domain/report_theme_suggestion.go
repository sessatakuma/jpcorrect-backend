package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ReportThemeSuggestion struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"suggestion_id"`
	Title       string    `json:"title"`
	Description *string   `gorm:"type:text" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReportThemeSuggestionRepository interface {
	List(ctx context.Context) ([]*ReportThemeSuggestion, error)
	GetByID(ctx context.Context, id uuid.UUID) (*ReportThemeSuggestion, error)
}
