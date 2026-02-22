package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type API struct {
	apiToolsURL      string
	proxyTransport   *http.Transport
	jwksURL          string
	jwksCache        keyfunc.Keyfunc
	jwksCtx          context.Context
	jwksCancel       context.CancelFunc
	jwksMutex        sync.Mutex
	jwksErr          error
	aiCorrectionRepo domain.AICorrectionRepository
	mistakeRepo      domain.MistakeRepository
	noteRepo         domain.NoteRepository
	practiceRepo     domain.PracticeRepository
	transcriptRepo   domain.TranscriptRepository
	userRepo         domain.UserRepository
	webrtcRepo       domain.WebRTCRepository
	rateLimiter      *RateLimiter
	upgrader         websocket.Upgrader
}

func NewAPI(url string, transport *http.Transport, conn repository.Connection, jwksURL string, allowedOrigins []string) *API {
	aiCorrectionRepo := repository.NewPostgresAICorrection(conn)
	mistakeRepo := repository.NewPostgresMistake(conn)
	noteRepo := repository.NewPostgresNote(conn)
	practiceRepo := repository.NewPostgresPractice(conn)
	transcriptRepo := repository.NewPostgresTranscript(conn)
	userRepo := repository.NewPostgresUser(conn)
	webrtcRepo := NewHub()
	rateLimiter := NewRateLimiter(10*time.Second, 15) // 10秒窗口，最多15次連線

	// 配置 WebSocket upgrader 的來源驗證
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if len(allowedOrigins) == 0 {
				// 開發模式：允許所有來源
				if gin.IsDebugging() {
					return true
				}
				// 生產模式：必須設定 ALLOWED_ORIGINS
				return false
			}
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if allowed == "*" || allowed == origin {
					return true
				}
			}
			return false
		},
	}

	return &API{
		apiToolsURL:      url,
		proxyTransport:   transport,
		jwksURL:          jwksURL,
		aiCorrectionRepo: aiCorrectionRepo,
		mistakeRepo:      mistakeRepo,
		noteRepo:         noteRepo,
		practiceRepo:     practiceRepo,
		transcriptRepo:   transcriptRepo,
		userRepo:         userRepo,
		webrtcRepo:       webrtcRepo,
		rateLimiter:      rateLimiter,
		upgrader:         upgrader,
	}
}

// Close stops the RateLimiter's cleanup goroutine
func (api *API) Close() {
	if api.rateLimiter != nil {
		api.rateLimiter.Close()
	}
}

func Register(r *gin.Engine, api *API) {
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })
	// WebRTC WebSocket endpoint
	r.GET("/ws", api.ServeWebSocket)

	v1 := r.Group("/v1")
	v1.Use(api.AuthMiddleware())
	{
		// API Tools Handlers
		v1.POST("/mark-accent", api.MarkAccentHandler)
		v1.POST("/mark-furigana", api.MarkFuriganaHandler)
		v1.POST("/usage-query/headwords", api.UsageQueryHeadWordsHandler)
		v1.POST("/usage-query/url", api.UsageQueryURLHandler)
		v1.POST("/usage-query/id-details", api.UsageQueryIDDetailsHandler)
		v1.POST("/dict-query", api.DictQueryHandler)
		v1.POST("/sentence-query", api.SentenceQueryHandler)

		// AI Corrections
		aiCorrections := v1.Group("/ai-corrections")
		{
			aiCorrections.POST("", api.AICorrectionCreateHandler)
			aiCorrections.GET("/:id", api.AICorrectionGetHandler)
			aiCorrections.PUT("/:id", api.AICorrectionUpdateHandler)
			aiCorrections.DELETE("/:id", api.AICorrectionDeleteHandler)
			aiCorrections.GET("/mistake/:mistake_id", api.AICorrectionGetByMistakeHandler)
		}

		// Mistakes
		mistakes := v1.Group("/mistakes")
		{
			mistakes.POST("", api.MistakeCreateHandler)
			mistakes.GET("/:id", api.MistakeGetHandler)
			mistakes.PUT("/:id", api.MistakeUpdateHandler)
			mistakes.DELETE("/:id", api.MistakeDeleteHandler)
			mistakes.GET("/practice/:practice_id", api.MistakeGetByPracticeHandler)
			mistakes.GET("/user/:user_id", api.MistakeGetByUserHandler)
		}

		// Notes
		notes := v1.Group("/notes")
		{
			notes.POST("", api.NoteCreateHandler)
			notes.GET("/:id", api.NoteGetHandler)
			notes.PUT("/:id", api.NoteUpdateHandler)
			notes.DELETE("/:id", api.NoteDeleteHandler)
			notes.GET("/practice/:practice_id", api.NoteGetByPracticeHandler)
		}

		// Practices
		practices := v1.Group("/practices")
		{
			practices.POST("", api.PracticeCreateHandler)
			practices.GET("/:id", api.PracticeGetHandler)
			practices.PUT("/:id", api.PracticeUpdateHandler)
			practices.DELETE("/:id", api.PracticeDeleteHandler)
			practices.GET("/user/:user_id", api.PracticeGetByUserHandler)
		}

		// Transcripts
		transcripts := v1.Group("/transcripts")
		{
			transcripts.POST("", api.TranscriptCreateHandler)
			transcripts.GET("/:id", api.TranscriptGetHandler)
			transcripts.PUT("/:id", api.TranscriptUpdateHandler)
			transcripts.DELETE("/:id", api.TranscriptDeleteHandler)
			transcripts.GET("/mistake/:mistake_id", api.TranscriptGetByMistakeHandler)
		}

		// Users
		users := v1.Group("/users")
		{
			users.POST("", api.UserCreateHandler)
			users.GET("/:id", api.UserGetHandler)
			users.PUT("/:id", api.UserUpdateHandler)
			users.DELETE("/:id", api.UserDeleteHandler)
			users.GET("/name/:name", api.UserGetByNameHandler)
		}
	}
}
