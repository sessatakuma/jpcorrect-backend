package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FeedPostType is the kind of a guild board post (spec §2.2). Regular user
// posts are out of scope for the MVP.
type FeedPostType string

const (
	FeedPostTypeSystem       FeedPostType = "system"
	FeedPostTypeAnnouncement FeedPostType = "announcement"
)

// FeedPost is one post on a guild's board (spec §2.2).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none. author_user_id is nullable because system
// posts have no author.
type FeedPost struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	GuildID      uuid.UUID      `gorm:"type:uuid;not null;index;index:idx_feed_post_guild_created,priority:1" json:"guild_id"`
	Type         FeedPostType   `gorm:"not null" json:"type"`
	AuthorUserID *uuid.UUID     `gorm:"type:uuid" json:"author_user_id"`
	Content      string         `gorm:"type:text;not null" json:"content"`
	Metadata     datatypes.JSON `gorm:"type:jsonb" json:"metadata"`
	CreatedAt    time.Time      `gorm:"index;index:idx_feed_post_guild_created,priority:2,sort:desc" json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Guild      *Guild `gorm:"foreignKey:GuildID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	AuthorUser *User  `gorm:"foreignKey:AuthorUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}

// FeedComment is a comment on a FeedPost (spec §2.2).
//
// Foreign-key actions are the project default (ON UPDATE CASCADE / ON DELETE
// RESTRICT); the spec gives none.
type FeedComment struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	PostID       uuid.UUID      `gorm:"type:uuid;not null;index;index:idx_feed_comment_post_created,priority:1" json:"post_id"`
	AuthorUserID uuid.UUID      `gorm:"type:uuid;not null" json:"author_user_id"`
	Content      string         `gorm:"type:text;not null" json:"content"`
	CreatedAt    time.Time      `gorm:"index;index:idx_feed_comment_post_created,priority:2" json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at"`

	Post       *FeedPost `gorm:"foreignKey:PostID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
	AuthorUser *User     `gorm:"foreignKey:AuthorUserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-" swaggerignore:"true"`
}
