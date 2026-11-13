package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ActivityStatus is the lifecycle state of an Activity (spec §2.2, §5.1).
type ActivityStatus string

const (
	ActivityStatusPendingPractice ActivityStatus = "pending_practice"
	ActivityStatusInPractice      ActivityStatus = "in_practice"
	ActivityStatusAnalyzing       ActivityStatus = "analyzing"
	ActivityStatusInFeedback      ActivityStatus = "in_feedback"
	ActivityStatusInReview        ActivityStatus = "in_review"
	ActivityStatusDone            ActivityStatus = "done"
	ActivityStatusAborted         ActivityStatus = "aborted"
)

// ActivityMode is the practice type of an Activity (spec §2.2). It is the
// source of truth; the paired practice Event's mode is a mirror.
type ActivityMode string

const (
	ActivityModeReport       ActivityMode = "report"
	ActivityModeConversation ActivityMode = "conversation"
)

// AIStatus is the state of the initial AI analysis of an Activity (spec §2.2).
type AIStatus string

const (
	AIStatusPending    AIStatus = "pending"
	AIStatusProcessing AIStatus = "processing"
	AIStatusCompleted  AIStatus = "completed"
	AIStatusFailed     AIStatus = "failed"
)

// YouTubeStatus is the state of the YouTube upload of the practice recording
// (spec §2.2).
type YouTubeStatus string

const (
	YouTubeStatusPending   YouTubeStatus = "pending"
	YouTubeStatusUploading YouTubeStatus = "uploading"
	YouTubeStatusCompleted YouTubeStatus = "completed"
	YouTubeStatusFailed    YouTubeStatus = "failed"
)

// Activity is one cycle pairing a practice Event and its review Event, shown as
// "Activity #N" (spec §2.2).
//
// Foreign keys: the spec gives no ON UPDATE / ON DELETE actions, so every
// foreign key in this schema uses the project default established with
// GuildAttendee, ON UPDATE CASCADE / ON DELETE RESTRICT. The constraint has to
// live on the belongs-to association field below; a `constraint` tag on a
// scalar column is silently ignored by GORM. announce_topic_id and
// aborted_by_user_id are nullable as #72 specifies, the rest are NOT NULL.
//
// StatusEnteredAt is an internal field, not part of v1.2; see its field comment
// and ActivityStatusRepository.
type Activity struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"activity_id"`
	GuildID         uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_activity_guild_sequence,priority:1" json:"guild_id"`
	SequenceNumber  int            `gorm:"not null;uniqueIndex:idx_activity_guild_sequence,priority:2" json:"sequence_number"`
	Status          ActivityStatus `gorm:"not null;default:pending_practice;index" json:"status"`
	Mode            ActivityMode   `gorm:"not null" json:"mode"`
	Theme           *string        `gorm:"type:text" json:"theme"`
	AnnounceTopicID *uuid.UUID     `gorm:"type:uuid" json:"announce_topic_id"`
	YouTubeURL      *string        `gorm:"column:youtube_url" json:"youtube_url"`
	AIStatus        AIStatus       `gorm:"not null;default:pending;index" json:"ai_status"`
	YouTubeStatus   YouTubeStatus  `gorm:"column:youtube_status;not null;default:pending;index:idx_activity_youtube_status" json:"youtube_status"`
	AbortedByUserID *uuid.UUID     `gorm:"type:uuid" json:"aborted_by_user_id"`
	AbortedAt       *time.Time     `json:"aborted_at"`
	AbortedReason   *string        `gorm:"type:text" json:"aborted_reason"`

	// StatusEnteredAt records when the current Status was entered, so the stuck
	// activity cron (§5.3) can measure how long an Activity has been in
	// in_practice / analyzing / in_review. updated_at cannot do that: it moves
	// on every write. BeforeCreate stamps it on insert and
	// ActivityStatusRepository.TransitionStatus re-stamps it in the same UPDATE
	// that changes Status. Nothing else moves it, so every status change must go
	// through TransitionStatus: a raw write to the status column would leave a
	// stale timestamp.
	StatusEnteredAt time.Time `gorm:"not null" json:"status_entered_at"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	// Associations exist only to emit the foreign keys; they are never
	// serialised. See the struct doc comment.
	Guild         *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	AnnounceTopic *Topic `gorm:"foreignKey:AnnounceTopicID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	AbortedByUser *User  `gorm:"foreignKey:AbortedByUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// BeforeCreate stamps StatusEnteredAt on the first write when the caller did not
// set it explicitly.
func (a *Activity) BeforeCreate(_ *gorm.DB) error {
	if a.StatusEnteredAt.IsZero() {
		a.StatusEnteredAt = time.Now().UTC()
	}
	return nil
}

// ActivityStatusRepository changes the status of an Activity. It is the only
// supported way to do so, because it keeps StatusEnteredAt in step with Status
// and gives the state machine (§5.1) its compare-and-swap primitive.
type ActivityStatusRepository interface {
	// TransitionStatus moves the Activity from any status in from to to in one
	// guarded UPDATE that also records when the new status was entered. It
	// reports whether a row changed. false means the Activity does not exist,
	// is soft-deleted, is not in one of the from statuses (the loser of a race,
	// or a wrong state) or is already in to; the caller turns that into
	// ErrActivityNotInState or ErrNotFound as its endpoint requires.
	TransitionStatus(ctx context.Context, activityID uuid.UUID, from []ActivityStatus, to ActivityStatus) (bool, error)
}

// ActivityReporter is a reporter (speaker) of an Activity (spec §2.2). The
// activity_id / user_id pair is the primary key, so a user reports an Activity
// at most once.
type ActivityReporter struct {
	ActivityID uuid.UUID `gorm:"type:uuid;not null;primaryKey" json:"activity_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;primaryKey" json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`

	Activity *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	User     *User     `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
