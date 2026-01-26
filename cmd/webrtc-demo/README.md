# WebRTC DEMO

使用前請先啟動主API服務

```bash
go run cmd/jpcorrect/main.go
```

啟動完成後，再啟動測試網頁 (正確配置會顯示使用HTTPS)

```bash
go run cmd/webrtc-demo/main.go
```

### 環境變數
```bash
PORT=8080

# API HTTPS 憑證設定
API_CERT_PATH=./cmd/webrtc-demo/certs/cert.pem
API_KEY_PATH=./cmd/webrtc-demo/certs/key.pem

# WebRTC config
WEBRTC_CONN_SEC=10 # 速率限制時間窗口（秒）
WEBRTC_CONN_MAX=15 # 每個 IP 在時間窗口內的最大連線數

# WebRTC Demo 網頁設定
WEBRTC_DEMO_PORT=3000 # WebRTC Demo 網頁 PORT
WEBRTC_DEMO_BASE_DIR=./cmd/webrtc-demo # WebRTC Demo 目錄
WEBRTC_DEMO_CERT_PATH=./cmd/webrtc-demo/certs/cert.pem
WEBRTC_DEMO_KEY_PATH=./cmd/webrtc-demo/certs/key.pem
```
