package domain

import (
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
// StatusEnteredAt is an internal field, not part of v1.2; see BeforeUpdate.
type Activity struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"activity_id"`
	GuildID         uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_activity_guild_sequence,priority:1" json:"guild_id"`
	SequenceNumber  int            `gorm:"not null;uniqueIndex:idx_activity_guild_sequence,priority:2" json:"sequence_number"`
	Status          ActivityStatus `gorm:"default:pending_practice;index" json:"status"`
	Mode            ActivityMode   `gorm:"not null" json:"mode"`
	Theme           *string        `gorm:"type:text" json:"theme"`
	AnnounceTopicID *uuid.UUID     `gorm:"type:uuid" json:"announce_topic_id"`
	YouTubeURL      *string        `gorm:"column:youtube_url" json:"youtube_url"`
	AIStatus        AIStatus       `gorm:"default:pending;index" json:"ai_status"`
	YouTubeStatus   YouTubeStatus  `gorm:"column:youtube_status;default:pending;index:idx_activity_youtube_status" json:"youtube_status"`
	AbortedByUserID *uuid.UUID     `gorm:"type:uuid" json:"aborted_by_user_id"`
	AbortedAt       *time.Time     `json:"aborted_at"`
	AbortedReason   *string        `gorm:"type:text" json:"aborted_reason"`

	// StatusEnteredAt records when the current Status was entered, so the stuck
	// activity cron (§5.3) can measure how long an Activity has been in
	// in_practice / analyzing / in_review. updated_at cannot do that: it moves
	// on every write. It is maintained by the BeforeCreate / BeforeUpdate hooks
	// below, so every writer gets it without remembering to set it.
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

// BeforeUpdate re-stamps StatusEnteredAt whenever the write changes the status,
// and leaves it alone on every other update. SetColumn adds the assignment to
// the statement, so the stamp lands for map updates, struct updates and Save
// alike.
func (a *Activity) BeforeUpdate(tx *gorm.DB) error {
	if tx.Statement.Changed("Status") {
		tx.Statement.SetColumn("StatusEnteredAt", time.Now().UTC())
		return nil
	}
	if _, isMap := tx.Statement.Dest.(map[string]any); isMap {
		// An Update/Updates map statement: Changed above already answered it.
		return nil
	}
	if tx.Statement.Model != tx.Statement.Dest {
		// An Update/Updates struct statement that does not touch the status
		// column: Changed above already answered it.
		return nil
	}

	// Save writes every column, and gorm cannot tell whether the status really
	// changed in that case, so ask the stored row instead of guessing.
	var stored []ActivityStatus
	if err := tx.Session(&gorm.Session{NewDB: true}).Model(&Activity{}).
		Where("id = ?", a.ID).Limit(1).Pluck("status", &stored).Error; err != nil {
		return err
	}
	if len(stored) > 0 && stored[0] == a.Status {
		return nil
	}

	tx.Statement.SetColumn("StatusEnteredAt", time.Now().UTC())
	return nil
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
