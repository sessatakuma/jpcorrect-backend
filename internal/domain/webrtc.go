package domain

import (
	"github.com/gorilla/websocket"
)

// Client represents a connected websocket client
type Client struct {
	ID   string
	Conn *websocket.Conn
	Send chan []byte
	Done chan struct{}
	Name string
}

type OnlineUser struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
}

type WebRTCHub interface {
	AddClient(c *Client)
	RemoveClient(id string)
	GetClient(id string) (*Client, bool)
	ListUsers() []OnlineUser
	BroadcastExcept(senderID string, msgType string, payload interface{})
}
