# WebRTC DEMO

使用前請先啟動主API服務

```bash
go run cmd/jpcorrect/main.go
```

啟動完成後，再啟動測試網頁 (注意路徑，正確會顯示HTTPS)

```bash
cd cmd/webrtc-demo/
go run main.go
```

### 環境變數
```bash
PORT=8000              # 伺服器端口（預設 8000）
CONN_WINDOW_SEC=10     # 速率限制時間窗口（秒）
CONN_MAX=15            # 每個 IP 在時間窗口內的最大連線數
CERT_PATH=certs/cert.pem  # TLS 憑證路徑
KEY_PATH=certs/key.pem     # TLS 金鑰路徑
```
