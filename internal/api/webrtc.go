package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"jpcorrect-backend/internal/domain"

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

// RateLimiter struct to track connection attempts per IP
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	window   time.Duration
	max      int
	ctx      context.Context
	cancel   context.CancelFunc
}

// Hub maintains set of clients
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*domain.Client
}

// Builds a new RateLimiter
func NewRateLimiter(window time.Duration, max int) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())
	rl := &RateLimiter{
		attempts: make(map[string][]time.Time),
		window:   window,
		max:      max,
		ctx:      ctx,
		cancel:   cancel,
	}
	go rl.cleanup()
	return rl
}

// IsAllowed checks if a new connection is allowed for the given IP
func (rl *RateLimiter) IsAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	times := rl.attempts[ip]

	// Filter out expired timestamps
	newTimes := make([]time.Time, 0, len(times))
	for _, t := range times {
		if now.Sub(t) <= rl.window {
			newTimes = append(newTimes, t)
		}
	}

	// Add the current timestamp
	newTimes = append(newTimes, now)
	rl.attempts[ip] = newTimes

	return len(newTimes) <= rl.max
}

// cleanup periodically cleans up expired IP records to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window * 2) // 每隔兩個窗口期清理一次
	defer ticker.Stop()

	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, times := range rl.attempts {
				// 過濾出有效的時間戳
				validTimes := make([]time.Time, 0, len(times))
				for _, t := range times {
					if now.Sub(t) <= rl.window {
						validTimes = append(validTimes, t)
					}
				}
				// 如果沒有有效時間戳，刪除該 IP
				if len(validTimes) == 0 {
					delete(rl.attempts, ip)
				} else {
					rl.attempts[ip] = validTimes
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Close RateLimiter and stop cleanup goroutine
func (rl *RateLimiter) Close() {
	rl.cancel()
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]*domain.Client)}
}

func (h *Hub) AddClient(c *domain.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.ID] = c
}

func (h *Hub) RemoveClient(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, id)
}

func (h *Hub) GetClient(id string) (*domain.Client, bool) {
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
		if c.Name == "" {
			continue
		}
		out = append(out, map[string]string{"userId": id, "userName": c.Name})
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
		case c.Send <- b:
		default:
			// drop
		}
	}
}

func sendToClient(c *domain.Client, msgType string, payload interface{}) error {
	m := map[string]interface{}{"type": msgType, "payload": payload}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	select {
	case c.Send <- b:
		return nil
	default:
		return fmt.Errorf("client send channel full")
	}
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

	if !api.rateLimiter.IsAllowed(ip) {
		log.Printf("拒絕來自 %s 的連線：短時間內連線數過多", ip)
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "too many connections",
		})
		return
	}

	conn, err := api.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("websocket upgrade error:", err)
		return
	}

	id := uuid.New().String()
	client := &domain.Client{
		ID:   id,
		Conn: conn,
		Send: make(chan []byte, 16),
		Done: make(chan struct{}),
	}

	api.webrtcRepo.AddClient(client)
	log.Println("新使用者連線:", id)

	// send connected message with assigned id
	if err := sendToClient(client, "connected", map[string]string{"id": id}); err != nil {
		log.Printf("傳送連線確認訊息失敗 (user: %s): %v", id, err)
	}

	// start writer
	go writer(client)

	// read loop
	for {
		var msg Message
		if err := client.Conn.ReadJSON(&msg); err != nil {
			log.Println("read error:", err)
			break
		}

		api.handleWebRTCMessage(client, msg)
	}

	// cleanup
	api.webrtcRepo.RemoveClient(client.ID)
	if client.Name != "" {
		api.webrtcRepo.BroadcastExcept(client.ID, "user-left", client.ID)
	}

	// cleanup 時
	close(client.Done) // 先關閉 done
	close(client.Send) // 再關閉 send
	if err := client.Conn.Close(); err != nil {
		log.Printf("關閉連線失敗 (user: %s): %v", client.ID, err)
	}
	log.Println("使用者離線:", client.ID)
}

func writer(c *domain.Client) {
	for {
		select {
		case <-c.Done:
			return
		case b, ok := <-c.Send:
			if !ok {
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, b); err != nil {
				log.Println("write error:", err)
				if err := c.Conn.Close(); err != nil {
					log.Println("關閉連線失敗:", err)
				}
				return
			}
		}
	}
}

func (api *API) handleWebRTCMessage(c *domain.Client, m Message) {
	switch m.Type {
	case "get-online-users":
		users := api.webrtcRepo.ListUsers()
		if err := sendToClient(c, "online-users-list", users); err != nil {
			log.Printf("傳送線上使用者列表失敗 (user: %s): %v", c.ID, err)
		}

	case "join-room":
		var p JoinPayload
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			if err := sendToClient(c, "error", map[string]string{"message": "invalid payload"}); err != nil {
				log.Printf("傳送錯誤訊息失敗 (user: %s): %v", c.ID, err)
			}
			return
		}
		valid, name := defaultValidateUserName(p.UserName)
		if !valid {
			if err := sendToClient(c, "error", map[string]string{"message": name}); err != nil {
				log.Printf("傳送使用者名稱驗證錯誤失敗 (user: %s): %v", c.ID, err)
			}
			return
		}
		c.Name = name
		// notify others
		api.webrtcRepo.BroadcastExcept(c.ID, "user-joined", map[string]string{"userId": c.ID, "userName": c.Name})
		// send current users (excluding self)
		current := api.webrtcRepo.ListUsers()
		filtered := make([]map[string]string, 0)
		for _, u := range current {
			if u["userId"] != c.ID {
				filtered = append(filtered, u)
			}
		}
		if err := sendToClient(c, "current-users", filtered); err != nil {
			log.Printf("傳送當前使用者列表失敗 (user: %s): %v", c.ID, err)
		}

	case "offer", "answer", "ice-candidate":
		// forward to target
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(m.Payload, &payload); err != nil {
			if err := sendToClient(c, "error", map[string]string{"message": "invalid payload"}); err != nil {
				log.Printf("傳送錯誤訊息失敗 (user: %s): %v", c.ID, err)
			}
			return
		}
		var targetId string
		if t, ok := payload["target"]; ok {
			if err := json.Unmarshal(t, &targetId); err != nil {
				if err := sendToClient(c, "error", map[string]string{"message": "invalid target"}); err != nil {
					log.Printf("傳送錯誤訊息失敗 (user: %s): %v", c.ID, err)
				}
				return
			}
		} else {
			if err := sendToClient(c, "error", map[string]string{"message": "missing target"}); err != nil {
				log.Printf("傳送錯誤訊息失敗 (user: %s): %v", c.ID, err)
			}
			return
		}
		target, ok := api.webrtcRepo.GetClient(targetId)
		if !ok {
			if err := sendToClient(c, "error", map[string]string{"message": "target not online"}); err != nil {
				log.Printf("傳送目標離線錯誤失敗 (user: %s, target: %s): %v", c.ID, targetId, err)
			}
			return
		}
		// Build forward payload: preserve original JSON format for WebRTC compatibility
		// Critical for iOS - any data format changes can break WebRTC connections
		forward := make(map[string]json.RawMessage)
		senderJSON, _ := json.Marshal(c.ID)
		forward["sender"] = senderJSON
		// Copy all fields except "target", preserving original JSON format
		for k, v := range payload {
			if k != "target" {
				forward[k] = v
			}
		}
		// Convert to interface{} for sendToClient
		var forwardData interface{}
		forwardBytes, _ := json.Marshal(forward)
		if err := json.Unmarshal(forwardBytes, &forwardData); err != nil {
			log.Printf("unmarshal forward data 失敗: %v", err)
			return
		}

		if err := sendToClient(target, m.Type, forwardData); err != nil {
			log.Printf("轉發 %s 訊息失敗 (from: %s, to: %s): %v", m.Type, c.ID, target.ID, err)
		}

	case "leave-room":
		if c.Name != "" {
			name := c.Name
			c.Name = ""
			api.webrtcRepo.BroadcastExcept(c.ID, "user-left", c.ID)
			log.Println("使用者離開聊天室:", c.ID, name)
		}

	default:
		if err := sendToClient(c, "error", map[string]string{"message": "unknown type"}); err != nil {
			log.Printf("傳送未知訊息類型錯誤失敗 (user: %s, type: %s): %v", c.ID, m.Type, err)
		}
	}
}
