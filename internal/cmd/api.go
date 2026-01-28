package cmd

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jpcorrect-backend/internal/api"
	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

func Execute() {
	db, err := database.NewGormDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get database instance: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		log.Printf("failed to close database connection: %v", err)
	}

	if err := db.AutoMigrate(
		&domain.User{},
		&domain.Event{},
		&domain.EventAttendee{},
		&domain.Transcript{},
		&domain.Mistake{},
		&domain.Guild{},
		&domain.GuildAttendee{},
	); err != nil {
		log.Fatalf("failed to run auto migrate: %v", err)
		os.Exit(1)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" {
		log.Fatalf("JWKS_URL environment variable is required")
	}

	a := api.NewAPI(os.Getenv("API_TOOLS_URL"), transport, db, jwksURL)
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
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	// kill (no params) by default sends syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Clean up JWKS resources
	a.ShutdownJWKS()

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}
