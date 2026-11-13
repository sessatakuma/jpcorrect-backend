package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// RecordingSessionStatus is the recording lifecycle state of a practice
// (spec §2.2, §7.3).
type RecordingSessionStatus string

const (
	RecordingSessionStatusStarting   RecordingSessionStatus = "starting"
	RecordingSessionStatusRecording  RecordingSessionStatus = "recording"
	RecordingSessionStatusFinalizing RecordingSessionStatus = "finalizing"
	RecordingSessionStatusReady      RecordingSessionStatus = "ready"
	RecordingSessionStatusPartial    RecordingSessionStatus = "partial"
	RecordingSessionStatusFailed     RecordingSessionStatus = "failed"
)

// RecordingAssetKind is the type of a stored recording artifact or reconnect
// segment (spec §2.2).
type RecordingAssetKind string

const (
	RecordingAssetKindAudioTrack  RecordingAssetKind = "audio_track"
	RecordingAssetKindScreenShare RecordingAssetKind = "screen_share"
	RecordingAssetKindComposite   RecordingAssetKind = "composite"
	RecordingAssetKindManifest    RecordingAssetKind = "manifest"
)

// RecordingAssetStatus is the upload state of a recording asset (spec §2.2).
type RecordingAssetStatus string

const (
	RecordingAssetStatusUploading RecordingAssetStatus = "uploading"
	RecordingAssetStatusReady     RecordingAssetStatus = "ready"
	RecordingAssetStatusFailed    RecordingAssetStatus = "failed"
	RecordingAssetStatusDeleted   RecordingAssetStatus = "deleted"
)

// RecordingSession is the recording lifecycle of one practice (spec §2.2, §7.3).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none. partial_approved_by_user_id is nullable and
// only written when an admin approves a partial recording (§7.5).
type RecordingSession struct {
	ID                      uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	ActivityID              uuid.UUID              `gorm:"type:uuid;not null;index" json:"activity_id"`
	EventID                 uuid.UUID              `gorm:"type:uuid;not null;index" json:"event_id"`
	Provider                string                 `gorm:"not null" json:"provider"`
	ProviderRoomID          string                 `gorm:"not null" json:"provider_room_id"`
	ProviderEgressID        string                 `gorm:"not null" json:"provider_egress_id"`
	Status                  RecordingSessionStatus `gorm:"not null" json:"status"`
	StartedAt               *time.Time             `json:"started_at"`
	StoppedAt               *time.Time             `json:"stopped_at"`
	ReadyAt                 *time.Time             `json:"ready_at"`
	PartialApprovedAt       *time.Time             `json:"partial_approved_at"`
	PartialApprovedByUserID *uuid.UUID             `gorm:"type:uuid" json:"partial_approved_by_user_id"`
	FailureReason           *string                `gorm:"type:text" json:"failure_reason"`
	CreatedAt               time.Time              `json:"created_at"`
	UpdatedAt               time.Time              `json:"updated_at"`

	Activity              *Activity `gorm:"foreignKey:ActivityID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	Event                 *Event    `gorm:"foreignKey:EventID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	PartialApprovedByUser *User     `gorm:"foreignKey:PartialApprovedByUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// RecordingAsset is one stored artifact or reconnect segment of a recording
// session (spec §2.2). Disconnect-and-reconnect products are separate assets;
// the shared timeline is a manifest asset that marks missing segments.
//
// object_key is the key in a private bucket: no public URL is stored.
//
// DeletedAt is deliberately a plain nullable timestamp, not gorm.DeletedAt: the
// spec keeps the metadata of a deleted object and only marks Status deleted, so
// a soft-delete scope would hide rows that must stay visible.
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none. participant_user_id is nullable because
// mixed / composite artifacts have no single owner.
type RecordingAsset struct {
	ID                 uuid.UUID            `gorm:"type:uuid;primaryKey" json:"id"`
	RecordingSessionID uuid.UUID            `gorm:"type:uuid;not null;index" json:"recording_session_id"`
	ParticipantUserID  *uuid.UUID           `gorm:"type:uuid" json:"participant_user_id"`
	Kind               RecordingAssetKind   `gorm:"not null" json:"kind"`
	ObjectKey          string               `gorm:"not null" json:"object_key"`
	ProviderTrackID    *string              `json:"provider_track_id"`
	TimelineMetadata   datatypes.JSON       `gorm:"type:jsonb" json:"timeline_metadata"`
	DurationMS         *int                 `json:"duration_ms"`
	ContentType        *string              `json:"content_type"`
	SizeBytes          *int64               `json:"size_bytes"`
	Checksum           *string              `json:"checksum"`
	Status             RecordingAssetStatus `gorm:"not null" json:"status"`
	DeleteAfter        *time.Time           `json:"delete_after"`
	DeletedAt          *time.Time           `json:"deleted_at"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`

	RecordingSession *RecordingSession `gorm:"foreignKey:RecordingSessionID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	ParticipantUser  *User             `gorm:"foreignKey:ParticipantUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
