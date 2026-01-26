package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Message is the generic JSON wrapper for messages between client and server
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Various payload structures
type JoinPayload struct {
	UserName string `json:"userName"`
}

type TargetPayload struct {
	Target string          `json:"target"`
	Data   json.RawMessage `json:"-"`
}

// Client represents a connected websocket client
type Client struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	name string
}

// Hub maintains set of clients
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// Connection rate limiter per IP (simple sliding window)
var connLimitMu sync.Mutex
var connAttempts = make(map[string][]time.Time)
var connWindow = 10 * time.Second
var connMax = 15 // max new connections per IP per window

func NewHub() *Hub {
	return &Hub{clients: make(map[string]*Client)}
}

func (h *Hub) AddClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.id] = c
}

func (h *Hub) RemoveClient(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, id)
}

func (h *Hub) GetClient(id string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[id]
	return c, ok
}

func (h *Hub) ListUsers() []map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]map[string]string, 0, len(h.clients))
	for id, c := range h.clients {
		if c.name == "" {
			continue
		}
		out = append(out, map[string]string{"userId": id, "userName": c.name})
	}
	return out
}

func (h *Hub) BroadcastExcept(senderId string, msgType string, payload interface{}) {
	m := map[string]interface{}{"type": msgType, "payload": payload}
	b, _ := json.Marshal(m)

	h.mu.RLock()
	defer h.mu.RUnlock()
	for id, c := range h.clients {
		if id == senderId {
			continue
		}
		select {
		case c.send <- b:
		default:
			// drop
		}
	}
}

func sendToClient(c *Client, msgType string, payload interface{}) error {
	m := map[string]interface{}{"type": msgType, "payload": payload}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	select {
	case c.send <- b:
		return nil
	default:
		return fmt.Errorf("client send channel full")
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Simple validation for WebRTC username (can be overridden)
func defaultValidateUserName(name string) (bool, string) {
	if name == "" {
		return false, "名稱不可為空"
	}
	if len([]rune(name)) > 20 {
		return false, "名稱長度不可超過 20 個字元"
	}
	return true, name
}

func (api *API) ServeWebSocket(c *gin.Context) {
	// Rate limit new connections per IP
	ip, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	if ip == "" {
		ip = c.Request.RemoteAddr
	}
	now := time.Now()
	connLimitMu.Lock()
	times := connAttempts[ip]
	// drop old
	newTimes := make([]time.Time, 0, len(times))
	for _, t := range times {
		if now.Sub(t) <= connWindow {
			newTimes = append(newTimes, t)
		}
	}
	newTimes = append(newTimes, now)
	connAttempts[ip] = newTimes
	if len(newTimes) > connMax {
		connLimitMu.Unlock()
		log.Printf("拒絕來自 %s 的連線：短時間內連線數過多 (%d)", ip, len(newTimes))
		// respond with 429-like behavior: upgrade and immediately close
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err == nil {
			conn.Close()
		}
		return
	}
	connLimitMu.Unlock()

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("websocket upgrade error:", err)
		return
	}

	id := uuid.New().String()
	client := &Client{
		id:   id,
		conn: conn,
		send: make(chan []byte, 16),
	}

	api.webrtcHub.AddClient(client)
	log.Println("新使用者連線:", id)

	// send connected message with assigned id
	_ = sendToClient(client, "connected", map[string]string{"id": id})

	// start writer
	go writer(client)

	// read loop
	for {
		var msg Message
		if err := client.conn.ReadJSON(&msg); err != nil {
			log.Println("read error:", err)
			break
		}

		api.handleWebRTCMessage(client, msg)
	}

	// cleanup
	api.webrtcHub.RemoveClient(client.id)
	if client.name != "" {
		api.webrtcHub.BroadcastExcept(client.id, "user-left", client.id)
	}
	client.conn.Close()
	log.Println("使用者離線:", client.id)
}

func writer(c *Client) {
	for {
		b, ok := <-c.send
		if !ok {
			return
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, b); err != nil {
			log.Println("write error:", err)
			return
		}
	}
}

func (api *API) handleWebRTCMessage(c *Client, m Message) {
	switch m.Type {
	case "get-online-users":
		users := api.webrtcHub.ListUsers()
		_ = sendToClient(c, "online-users-list", users)

	case "join-room":
		var p JoinPayload
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			_ = sendToClient(c, "error", map[string]string{"message": "invalid payload"})
			return
		}
		valid, name := defaultValidateUserName(p.UserName)
		if !valid {
			_ = sendToClient(c, "error", map[string]string{"message": name})
			return
		}
		c.name = name
		// notify others
		api.webrtcHub.BroadcastExcept(c.id, "user-joined", map[string]string{"userId": c.id, "userName": c.name})
		// send current users (excluding self)
		current := api.webrtcHub.ListUsers()
		filtered := make([]map[string]string, 0)
		for _, u := range current {
			if u["userId"] != c.id {
				filtered = append(filtered, u)
			}
		}
		_ = sendToClient(c, "current-users", filtered)

	case "offer", "answer", "ice-candidate":
		// forward to target
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(m.Payload, &payload); err != nil {
			_ = sendToClient(c, "error", map[string]string{"message": "invalid payload"})
			return
		}
		var targetId string
		if t, ok := payload["target"]; ok {
			json.Unmarshal(t, &targetId)
		} else {
			_ = sendToClient(c, "error", map[string]string{"message": "missing target"})
			return
		}
		target, ok := api.webrtcHub.GetClient(targetId)
		if !ok {
			_ = sendToClient(c, "error", map[string]string{"message": "target not online"})
			return
		}
		// Build forward payload: include sender and the rest
		forward := map[string]interface{}{"sender": c.id}
		// attach the actual data (offer/answer/candidate)
		for k, v := range payload {
			if k == "target" {
				continue
			}
			var raw interface{}
			if err := json.Unmarshal(v, &raw); err == nil {
				forward[k] = raw
			} else {
				forward[k] = nil
			}
		}
		_ = sendToClient(target, m.Type, forward)

	case "leave-room":
		if c.name != "" {
			name := c.name
			c.name = ""
			api.webrtcHub.BroadcastExcept(c.id, "user-left", c.id)
			log.Println("使用者離開聊天室:", c.id, name)
		}

	default:
		_ = sendToClient(c, "error", map[string]string{"message": "unknown type"})
	}
}
