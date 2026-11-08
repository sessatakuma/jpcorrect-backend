package cmd

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"jpcorrect-backend/internal/api"
	"jpcorrect-backend/internal/database"
	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
)

// migrationModels returns the single AutoMigrate list of spec §2.3. The order is
// the foreign-key dependency order, so one call can build an empty database:
// a table is always migrated after everything it references. There is no
// production data and no down migration; a failed migration means the database
// is rebuilt.
//
// testutil.BaselineModels mirrors this list for the integration tests, and
// TestMigrationModelsMatchTestBaseline fails if the two drift apart.
func migrationModels() []any {
	return []any{
		// Existing models, with the spec §2.1 changes.
		&domain.User{}, // + supabase_user_id unique
		&domain.Guild{},
		&domain.GuildAttendee{},

		// Topic bank: the foreign key target of Activity.announce_topic_id.
		&domain.Topic{},
		&domain.ReportThemeSuggestion{},

		// Activity depends on Guild / User / Topic.
		&domain.Activity{},

		// Successful AI submissions depend on Activity.
		&domain.AIAnalysisSubmission{},

		// Event depends on Activity / User.
		&domain.Event{}, // + activity_id / kind / current_step / step_durations / recording_*
		&domain.EventAttendee{},
		&domain.Transcript{},
		&domain.Mistake{}, // + confidence / latest_correction_* / interacted_at

		// ActivityReporter depends on Activity / User.
		&domain.ActivityReporter{},

		// Guild support tables.
		&domain.GuildDefaultSlot{},
		&domain.InviteLink{},
		&domain.JoinRequest{},

		// Feedback and review.
		&domain.FeedbackProgress{},
		&domain.SelectedSentence{},
		&domain.ReviewRecord{},
		&domain.PracticeNote{},

		// Feed.
		&domain.FeedPost{},
		&domain.FeedComment{},

		// Notifications.
		&domain.NotificationOutbox{},
		&domain.Notification{}, // + outbox_id unique
		&domain.PushSubscription{},

		// Chatbot usage (spec §2.3 "billing").
		&domain.ChatbotUsage{},

		// Recording.
		&domain.RecordingSession{},
		&domain.RecordingAsset{},
	}
}

func Execute() {
	db, err := database.NewGormDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(migrationModels()...); err != nil {
		log.Fatalf("failed to run auto migrate: %v", err)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		// Bound how long the api-tools proxy waits on a stalled upstream. A dead
		// or hung api-tools (e.g. its external OJAD dependency timing out) would
		// otherwise pin the inbound request open indefinitely. ResponseHeaderTimeout
		// only limits time-to-first-byte, so it is safe for the NDJSON streaming
		// handler: the long-lived body stream is not affected once headers arrive.
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" && !gin.IsDebugging() {
		log.Fatalf("JWKS_URL environment variable is required in release mode")
	}

	allowedOrigins := []string{}
	if originsEnv := os.Getenv("ALLOWED_ORIGINS"); originsEnv != "" {
		allowedOrigins = strings.Split(originsEnv, ",")
		for i, origin := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(origin)
		}
	}

	a := api.NewAPI(os.Getenv("API_TOOLS_URL"), os.Getenv("CLIENT_API_KEY"), transport, db, jwksURL, allowedOrigins)
	defer a.Close()

	// Skip JWKS fetch in debug mode: AuthMiddleware is not registered so it
	// would only waste a startup HTTP call. Also tolerates an empty
	// JWKS_URL in dev deploys where there's no real auth provider configured.
	// Debug callers are selected via X-Debug-User-ID instead.
	if !gin.IsDebugging() {
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer initCancel()
		if err := a.InitializeJWKS(initCtx); err != nil {
			log.Fatalf("failed to initialize JWKS: %v", err)
		}
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

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
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
