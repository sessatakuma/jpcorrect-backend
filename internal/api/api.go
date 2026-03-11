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
	"gorm.io/gorm"
)

type API struct {
	db                *gorm.DB
	apiToolsURL       string
	proxyTransport    *http.Transport
	jwksURL           string
	jwksCache         keyfunc.Keyfunc
	jwksCtx           context.Context
	jwksCancel        context.CancelFunc
	jwksMutex         sync.Mutex
	jwksErr           error
	userRepo          domain.UserRepository
	guildRepo         domain.GuildRepository
	guildAttendeeRepo domain.GuildAttendeeRepository
	eventRepo         domain.EventRepository
	eventAttendeeRepo domain.EventAttendeeRepository
	transcriptRepo    domain.TranscriptRepository
	mistakeRepo       domain.MistakeRepository
	webrtcHub         domain.WebRTCHub
	rateLimiter       *RateLimiter
	upgrader          websocket.Upgrader
}

func NewAPI(url string, transport *http.Transport, db *gorm.DB, jwksURL string, allowedOrigins []string) *API {
	userRepo := repository.NewGormUserRepository(db)
	guildRepo := repository.NewGormGuildRepository(db)
	guildAttendeeRepo := repository.NewGormGuildAttendeeRepository(db)
	eventRepo := repository.NewGormEventRepository(db)
	eventAttendeeRepo := repository.NewGormEventAttendeeRepository(db)
	transcriptRepo := repository.NewGormTranscriptRepository(db)
	mistakeRepo := repository.NewGormMistakeRepository(db)
	webrtcHub := NewHub()
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
		db:                db,
		apiToolsURL:       url,
		proxyTransport:    transport,
		jwksURL:           jwksURL,
		userRepo:          userRepo,
		guildRepo:         guildRepo,
		guildAttendeeRepo: guildAttendeeRepo,
		eventRepo:         eventRepo,
		eventAttendeeRepo: eventAttendeeRepo,
		transcriptRepo:    transcriptRepo,
		mistakeRepo:       mistakeRepo,
		webrtcHub:         webrtcHub,
		rateLimiter:       rateLimiter,
		upgrader:          upgrader,
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

		// Mistakes
		mistakes := v1.Group("/mistakes")
		{
			mistakes.POST("", api.MistakeCreateHandler)
			mistakes.GET("/:id", api.MistakeGetHandler)
			mistakes.PUT("/:id", api.MistakeUpdateHandler)
			mistakes.DELETE("/:id", api.MistakeDeleteHandler)
			mistakes.GET("/event/:event_id", api.MistakeGetByEventHandler)
			mistakes.GET("/user/:user_id", api.MistakeGetByUserHandler)
		}

		// Practices (keep old route for backward compatibility)
		practices := v1.Group("/practices")
		{
			practices.POST("", api.PracticeCreateHandler)
			practices.GET("/:id", api.PracticeGetHandler)
			practices.PUT("/:id", api.PracticeUpdateHandler)
			practices.DELETE("/:id", api.PracticeDeleteHandler)
			practices.GET("/user/:user_id", api.PracticeGetByUserHandler)
		}

		// Guilds
		guilds := v1.Group("/guilds")
		{
			guilds.POST("", api.GuildCreateHandler)
			guilds.GET("/:id", api.GuildGetHandler)
			guilds.PUT("/:id", api.GuildUpdateHandler)
			guilds.DELETE("/:id", api.GuildDeleteHandler)
		}

		// Guild Attendees
		guildAttendees := v1.Group("/guild-attendees")
		{
			guildAttendees.POST("", api.GuildAttendeeCreateHandler)
			guildAttendees.GET("/:id", api.GuildAttendeeGetHandler)
			guildAttendees.PUT("/:id", api.GuildAttendeeUpdateHandler)
			guildAttendees.DELETE("/:id", api.GuildAttendeeDeleteHandler)
			guildAttendees.GET("/guild/:guild_id", api.GuildAttendeeGetByGuildHandler)
			guildAttendees.GET("/user/:user_id", api.GuildAttendeeGetByUserHandler)
		}

		// Transcripts
		transcripts := v1.Group("/transcripts")
		{
			transcripts.POST("", api.TranscriptCreateHandler)
			transcripts.GET("/:id", api.TranscriptGetHandler)
			transcripts.PUT("/:id", api.TranscriptUpdateHandler)
			transcripts.DELETE("/:id", api.TranscriptDeleteHandler)
			transcripts.GET("/event/:event_id", api.TranscriptGetByEventHandler)
			transcripts.GET("/user/:user_id", api.TranscriptGetByUserHandler)
		}

		// Event Attendees
		eventAttendees := v1.Group("/event-attendees")
		{
			eventAttendees.POST("", api.EventAttendeeCreateHandler)
			eventAttendees.GET("/:id", api.EventAttendeeGetHandler)
			eventAttendees.PUT("/:id", api.EventAttendeeUpdateHandler)
			eventAttendees.DELETE("/:id", api.EventAttendeeDeleteHandler)
			eventAttendees.GET("/event/:event_id", api.EventAttendeeGetByEventHandler)
			eventAttendees.GET("/user/:user_id", api.EventAttendeeGetByUserHandler)
		}

		// Users
		users := v1.Group("/users")
		{
			users.POST("", api.UserCreateHandler)
			users.GET("/:id", api.UserGetHandler)
			users.PUT("/:id", api.UserUpdateHandler)
			users.DELETE("/:id", api.UserDeleteHandler)
			users.GET("/name/:name", api.UserGetByNameHandler)
			users.GET("/email/:email", api.UserGetByEmailHandler)
		}
	}
}
