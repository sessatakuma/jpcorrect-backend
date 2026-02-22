package cmd

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"jpcorrect-backend/internal/api"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Execute() {
	dbpool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
		os.Exit(1)
	}
	defer dbpool.Close()

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" {
		log.Fatalf("JWKS_URL environment variable is required")
	}

	allowedOrigins := []string{}
	if originsEnv := os.Getenv("ALLOWED_ORIGINS"); originsEnv != "" {
		allowedOrigins = strings.Split(originsEnv, ",")
		for i, origin := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(origin)
		}
	}

	a := api.NewAPI(os.Getenv("API_TOOLS_URL"), transport, dbpool, jwksURL, allowedOrigins)
	defer a.Close()

	initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer initCancel()
	if err := a.InitializeJWKS(initCtx); err != nil {
		log.Fatalf("failed to initialize JWKS: %v", err)
	}

	r := gin.Default()
	api.Register(r, a)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	certPath := os.Getenv("API_CERT_PATH")
	if certPath == "" {
		certPath = "./certs/cert.pem"
	}
	keyPath := os.Getenv("API_KEY_PATH")
	if keyPath == "" {
		keyPath = "./certs/key.pem"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	fileExists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}

	go func() {
		if fileExists(certPath) && fileExists(keyPath) {
			log.Println("🔒 使用 HTTPS 模式")
			log.Printf("📱 API 監聽: https://localhost:%s", port)
			log.Printf("   憑證: %s", certPath)
			log.Printf("   金鑰: %s", keyPath)
			if err := srv.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		} else {
			log.Println("⚠️ 使用 HTTP 模式（開發用）")
			log.Printf("📱 API 監聽: http://localhost:%s", port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	a.ShutdownJWKS()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
