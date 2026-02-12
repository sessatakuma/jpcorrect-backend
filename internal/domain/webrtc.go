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

type WebRTCRepository interface {
	AddClient(c *Client)
	RemoveClient(id string)
	GetClient(id string) (*Client, bool)
	ListUsers() []map[string]string
	BroadcastExcept(senderId string, msgType string, payload interface{})
}
