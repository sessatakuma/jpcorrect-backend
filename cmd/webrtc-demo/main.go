package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"jpcorrect-backend/internal/api"
)

// WebRTC Demo specific validation and configuration

// Connection rate limiter configuration (can be adjusted via environment variables)
var (
	connWindow = 10 * time.Second // Time window for rate limiting
	connMax    = 15               // Max connections per IP per window
)

// Username validation pattern and function
var usernamePattern = regexp.MustCompile(`^[\p{L}0-9_-]+$`)

func validateUserName(name string) (bool, string) {
	if name == "" {
		return false, "名稱不可為空"
	}
	if len([]rune(name)) > 20 {
		return false, "名稱長度不可超過 20 個字元"
	}
	if !usernamePattern.MatchString(name) {
		return false, "名稱只能包含字母、數字、連字號或底線"
	}
	return true, name
}

func main() {
	_ = api.NewHub()

	// Get the directory where this source file is located
	// When running with "go run", we need to use the source directory
	baseDir := os.Getenv("WEBRTC_BASE_DIR")
	if baseDir == "" {
		// Default to current working directory
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			log.Fatal("無法取得當前目錄:", err)
		}
		log.Printf("使用當前工作目錄: %s", baseDir)
	}

	// Load configuration from environment
	if v := os.Getenv("CONN_WINDOW_SEC"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			connWindow = time.Duration(secs) * time.Second
			log.Printf("✓ 連線速率限制時間窗口: %v", connWindow)
		}
	}
	if v := os.Getenv("CONN_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			connMax = n
			log.Printf("✓ 每個 IP 最大連線數: %d", connMax)
		}
	}
	certPath := os.Getenv("CERT_PATH")
	if certPath == "" {
		certPath = filepath.Join(baseDir, "certs", "cert.pem")
	}
	keyPath := os.Getenv("KEY_PATH")
	if keyPath == "" {
		keyPath = filepath.Join(baseDir, "certs", "key.pem")
	}

	publicDir := filepath.Join(baseDir, "public")
	log.Printf("靜態檔案目錄: %s", publicDir)
	fs := http.FileServer(http.Dir(publicDir))

	// 顯式提供 /test 對應到 public/test.html，方便診斷使用者直接訪問 /test
	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(publicDir, "test.html"))
	})

	http.Handle("/", fs)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// Inform clients to connect to main API WebSocket endpoint
		target := "ws://localhost:8080/ws"
		// If request is a websocket upgrade, respond with a temporary redirect (note: many WS clients connect directly)
		if r.Header.Get("Upgrade") != "" {
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		// For regular HTTP requests, return JSON with the correct websocket endpoint
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("{\"websocket\": %q}", target)))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	addr := ":" + port

	log.Printf("config: PORT=%s, CERT=%s, KEY=%s", port, certPath, keyPath)

	if fileExists(certPath) && fileExists(keyPath) {
		log.Println("🔒 使用 HTTPS 模式")
		// Use TLS server
		srv := &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 5 * time.Second,
		}
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

func fileExists(p string) bool {
	if _, err := os.Stat(p); err != nil {
		return false
	}
	return true
}
