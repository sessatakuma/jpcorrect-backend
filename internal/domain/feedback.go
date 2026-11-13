package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// FeedbackProgressStatus is a member's feedback progress on an Activity
// (spec §2.2).
type FeedbackProgressStatus string

const (
	FeedbackProgressStatusNotStarted FeedbackProgressStatus = "not_started"
	FeedbackProgressStatusInProgress FeedbackProgressStatus = "in_progress"
	FeedbackProgressStatusCompleted  FeedbackProgressStatus = "completed"
)

// FeedbackProgress tracks how far one member got through the feedback stage of
// an Activity (spec §2.2). The activity_id / user_id pair is the primary key.
// Progress only: correction content stays in Mistake so the leader can see
// progress without seeing corrections.
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type FeedbackProgress struct {
	ActivityID  uuid.UUID              `gorm:"type:uuid;not null;primaryKey;index:idx_feedback_progress_activity_status,priority:1" json:"activity_id"`
	UserID      uuid.UUID              `gorm:"type:uuid;not null;primaryKey" json:"user_id"`
	Status      FeedbackProgressStatus `gorm:"not null;default:not_started;index:idx_feedback_progress_activity_status,priority:2" json:"status"`
	StartedAt   *time.Time             `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at"`
	UpdatedAt   time.Time              `json:"updated_at"`

	Activity *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	User     *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// SelectedSentence is a sentence one member picked from their own mistakes for
// the review room (spec §2.2). A mistake can only be selected once.
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type SelectedSentence struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ActivityID       uuid.UUID  `gorm:"type:uuid;not null;index;index:idx_selected_sentence_activity_mistake,priority:1;index:idx_selected_sentence_user_activity,priority:2" json:"activity_id"`
	MistakeID        uuid.UUID  `gorm:"type:uuid;not null;index;index:idx_selected_sentence_activity_mistake,priority:2;uniqueIndex:idx_selected_sentence_mistake_user,priority:1" json:"mistake_id"`
	SelectedByUserID uuid.UUID  `gorm:"type:uuid;not null;index;index:idx_selected_sentence_user_activity,priority:1;uniqueIndex:idx_selected_sentence_mistake_user,priority:2" json:"selected_by_user_id"`
	IsDegraded       bool       `gorm:"not null;default:false" json:"is_degraded"`
	RevealedAt       *time.Time `json:"revealed_at"`
	RevealedByUserID *uuid.UUID `gorm:"type:uuid" json:"revealed_by_user_id"`
	CreatedAt        time.Time  `json:"created_at"`

	Activity       *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	Mistake        *Mistake  `gorm:"foreignKey:MistakeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	SelectedByUser *User     `gorm:"foreignKey:SelectedByUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	RevealedByUser *User     `gorm:"foreignKey:RevealedByUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// ReviewRecord is the stage-2 summary of one Activity's review (spec §2.2).
// activity_id is the primary key, so the relation is 1:1.
type ReviewRecord struct {
	ActivityID    uuid.UUID      `gorm:"type:uuid;not null;primaryKey" json:"activity_id"`
	Stage2Summary datatypes.JSON `gorm:"type:jsonb" json:"stage2_summary"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`

	Activity *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// PracticeNote is a non-reporter's personal note during report practice
// (spec §2.2). The activity_id / user_id pair is the primary key.
type PracticeNote struct {
	ActivityID uuid.UUID `gorm:"type:uuid;not null;primaryKey" json:"activity_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;primaryKey" json:"user_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	UpdatedAt  time.Time `json:"updated_at"`

	Activity *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	User     *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
