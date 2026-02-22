package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"jpcorrect-backend/internal/api"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// 由 main() 設定，決定使用 http/https 及 ws/wss
var proxyWebSocketUseHTTPS bool

// WebRTC Demo specific validation and configuration

// Connection rate limiter configuration (can be adjusted via environment variables)
var (
	connWindow = 10 * time.Second // Time window for rate limiting
	connMax    = 15               // Max connections per IP per window
)

// // Username validation pattern and function
// var usernamePattern = regexp.MustCompile(`^[\p{L}0-9_-]+$`)

// func validateUserName(name string) (bool, string) {
// 	if name == "" {
// 		return false, "名稱不可為空"
// 	}
// 	if len([]rune(name)) > 20 {
// 		return false, "名稱長度不可超過 20 個字元"
// 	}
// 	if !usernamePattern.MatchString(name) {
// 		return false, "名稱只能包含字母、數字、連字號或底線"
// 	}
// 	return true, name
// }

// WebSocket upgrader 用於升級 HTTP 連接為 WebSocket
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允許所有來源（生產環境應該更嚴格）
	},
}

// WebSocket dialer 用於連接到後端 API
var dialer = &websocket.Dialer{
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, // 開發環境跳過證書驗證
	},
	HandshakeTimeout: 10 * time.Second,
}

func main() {
	_ = godotenv.Load()
	_ = api.NewHub()

	// Get the directory where this source file is located
	// When running with "go run", we need to use the source directory
	baseDir := os.Getenv("WEBRTC_DEMO_BASE_DIR")
	if baseDir == "" {
		// Default to cmd/webrtc-demo directory
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			log.Fatal("無法取得當前目錄:", err)
		}
		// If we're in the project root, append cmd/webrtc-demo
		if _, err := os.Stat(filepath.Join(baseDir, "cmd", "webrtc-demo")); err == nil {
			baseDir = filepath.Join(baseDir, "cmd", "webrtc-demo")
		}
		log.Printf("使用工作目錄: %s", baseDir)
	}

	// Load configuration from environment
	if v := os.Getenv("WEBRTC_CONN_SEC"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			connWindow = time.Duration(secs) * time.Second
			log.Printf("連線速率限制時間窗口: %v", connWindow)
		}
	}
	if v := os.Getenv("WEBRTC_CONN_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			connMax = n
			log.Printf("每個 IP 最大連線數: %d", connMax)
		}
	}
	certPath := os.Getenv("WEBRTC_DEMO_CERT_PATH")
	if certPath == "" {
		certPath = "./certs/cert.pem"
	}
	log.Printf("憑證路徑: %s", certPath)
	keyPath := os.Getenv("WEBRTC_DEMO_KEY_PATH")
	if keyPath == "" {
		keyPath = "./certs/key.pem"
	}
	log.Printf("金鑰路徑: %s", keyPath)

	publicDir := filepath.Join(baseDir, "public")

	// Verify public directory exists
	if _, err := os.Stat(publicDir); os.IsNotExist(err) {
		log.Fatalf("❌ 靜態檔案目錄不存在: %s", publicDir)
	}

	fs := http.FileServer(http.Dir(publicDir))

	// 顯式提供 /test 對應到 public/test.html，方便診斷使用者直接訪問 /test
	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(publicDir, "test.html"))
	})

	http.Handle("/", fs)

	apiPort := os.Getenv("PORT")
	if apiPort == "" {
		apiPort = "8080"
	}

	// WebSocket 代理：將 /ws 請求轉發到主 API 服務器
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		proxyWebSocket(w, r, apiPort)
	})

	webPort := os.Getenv("WEBRTC_DEMO_PORT")
	if webPort == "" {
		webPort = "3000"
	}

	addr := ":" + webPort

	proxyWebSocketUseHTTPS = fileExists(certPath) && fileExists(keyPath)
	if proxyWebSocketUseHTTPS {
		log.Println("🔒 使用 HTTPS 模式")
		srv := &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 5 * time.Second,
		}
		// 將 HTTPS 狀態傳給 proxyWebSocket
		proxyWebSocketUseHTTPS = true
		log.Fatal(srv.ListenAndServeTLS(certPath, keyPath))
	} else {
		log.Println("⚠️ 使用 HTTP 模式（開發用）")
		srv := &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 5 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())
	}
}

// proxyWebSocket 使用 gorilla/websocket 將連接代理到主 API 服務器
func proxyWebSocket(w http.ResponseWriter, r *http.Request, apiPort string) {
	// 升級客戶端連接為 WebSocket
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ 升級客戶端連接失敗: %v", err)
		return
	}
	defer func() {
		if err := clientConn.Close(); err != nil {
			log.Printf("關閉 clientConn 失敗: %v", err)
		}
	}()

	// 構建後端 WebSocket URL，根據主機模式決定 ws/wss
	scheme := "ws"
	if proxyWebSocketUseHTTPS {
		scheme = "wss"
	}

	backendURL := url.URL{
		Scheme: scheme,
		Host:   "localhost:" + apiPort,
		Path:   "/ws",
	}

	// 連接到後端 WebSocket
	log.Printf("🔄 代理 WebSocket 到: %s", backendURL.String())
	backendConn, _, err := dialer.Dial(backendURL.String(), nil)
	if err != nil {
		log.Printf("❌ 連接後端 WebSocket 失敗: %v", err)
		if err := clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Backend connection failed")); err != nil {
			log.Printf("傳送 close message 失敗: %v", err)
		}
		return
	}
	defer func() {
		if err := backendConn.Close(); err != nil {
			log.Printf("關閉 backendConn 失敗: %v", err)
		}
	}()

	log.Println("✅ WebSocket 代理連接已建立")

	// 雙向轉發消息
	errChan := make(chan error, 2)

	// 客戶端 -> 後端
	go func() {
		errChan <- proxyMessages(clientConn, backendConn, "客戶端->後端")
	}()

	// 後端 -> 客戶端
	go func() {
		errChan <- proxyMessages(backendConn, clientConn, "後端->客戶端")
	}()

	// 等待任一方向發生錯誤或關閉
	err = <-errChan
	if err != nil {
		log.Printf("⚠️ WebSocket 代理結束: %v", err)
	} else {
		log.Println("✅ WebSocket 代理正常關閉")
	}
}

// proxyMessages 在兩個 WebSocket 連接之間轉發消息
func proxyMessages(src, dst *websocket.Conn, direction string) error {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("❌ %s 讀取錯誤: %v", direction, err)
			}
			return err
		}

		err = dst.WriteMessage(messageType, message)
		if err != nil {
			log.Printf("❌ %s 寫入錯誤: %v", direction, err)
			return err
		}
	}
}

func fileExists(p string) bool {
	if _, err := os.Stat(p); err != nil {
		return false
	}
	return true
}
