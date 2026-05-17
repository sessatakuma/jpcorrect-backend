package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"

	_ "jpcorrect-backend/docs/swagger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	activityRepo        domain.ActivityRepository
	inviteLinkRepo      domain.InviteLinkRepository
	guildDefaultSlotRepo       domain.GuildDefaultSlotRepository
	topicRepo                  domain.TopicRepository
	reportThemeSuggestionRepo  domain.ReportThemeSuggestionRepository
	joinRequestRepo            domain.JoinRequestRepository
	webrtcHub                  domain.WebRTCHub
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
	activityRepo := repository.NewGormActivityRepository(db)
	inviteLinkRepo := repository.NewGormInviteLinkRepository(db)
	guildDefaultSlotRepo := repository.NewGormGuildDefaultSlotRepository(db)
	topicRepo := repository.NewGormTopicRepository(db)
	reportThemeSuggestionRepo := repository.NewGormReportThemeSuggestionRepository(db)
	joinRequestRepo := repository.NewGormJoinRequestRepository(db)
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
		activityRepo:        activityRepo,
		inviteLinkRepo:      inviteLinkRepo,
		guildDefaultSlotRepo:       guildDefaultSlotRepo,
		topicRepo:                  topicRepo,
		reportThemeSuggestionRepo:  reportThemeSuggestionRepo,
		joinRequestRepo:            joinRequestRepo,
		webrtcHub:                  webrtcHub,
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

	// Only register Swagger endpoint in development mode
	if gin.IsDebugging() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// WebRTC WebSocket endpoint
	r.GET("/ws", api.ServeWebSocket)

	v1 := r.Group("/v1")

	// Public routes (no auth required)
	v1.GET("/invites/:token", api.InviteInfoHandler)

	// Auth-required routes
	v1Auth := v1.Group("")
	v1Auth.Use(api.AuthMiddleware())
	{
		// API Tools Handlers
		v1Auth.POST("/mark-accent", api.MarkAccentHandler)
		v1Auth.POST("/mark-furigana", api.MarkFuriganaHandler)
		v1Auth.POST("/usage-query/headwords", api.UsageQueryHeadWordsHandler)
		v1Auth.POST("/usage-query/url", api.UsageQueryURLHandler)
		v1Auth.POST("/usage-query/id-details", api.UsageQueryIDDetailsHandler)
		v1Auth.POST("/dict-query", api.DictQueryHandler)
		v1Auth.POST("/sentence-query", api.SentenceQueryHandler)

		// Invites (auth-required)
		v1Auth.POST("/invites/:token/accept", api.InviteAcceptHandler)

		// Mistakes
		mistakes := v1Auth.Group("/mistakes")
		{
			mistakes.POST("", api.MistakeCreateHandler)
			mistakes.GET("/:id", api.MistakeGetHandler)
			mistakes.PUT("/:id", api.MistakeUpdateHandler)
			mistakes.DELETE("/:id", api.MistakeDeleteHandler)
			mistakes.GET("/event/:event_id", api.MistakeGetByEventHandler)
			mistakes.GET("/user/:user_id", api.MistakeGetByUserHandler)
		}

		// Practices (keep old route for backward compatibility)
		practices := v1Auth.Group("/practices")
		{
			practices.POST("", api.PracticeCreateHandler)
			practices.GET("/:id", api.PracticeGetHandler)
			practices.PUT("/:id", api.PracticeUpdateHandler)
			practices.DELETE("/:id", api.PracticeDeleteHandler)
			practices.GET("/user/:user_id", api.PracticeGetByUserHandler)
		}

		// Guilds
		guilds := v1Auth.Group("/guilds")
		{
			guilds.POST("", api.GuildCreateHandler)
			guilds.GET("/discover", api.GuildDiscoverHandler)
			guilds.GET("/:id", api.GuildGetHandler)
			guilds.PUT("/:id", api.GuildUpdateHandler)
			guilds.DELETE("/:id", api.GuildDeleteHandler)
			guilds.GET("/:id/members", api.GuildMembersHandler)
			guilds.DELETE("/:id/members/:user_id", api.GuildMemberRemoveHandler)
			guilds.POST("/:id/leave", api.GuildLeaveHandler)
			guilds.POST("/:id/applications", api.GuildApplicationCreateHandler)
			guilds.GET("/:id/applications", api.GuildApplicationsHandler)
			guilds.POST("/:id/applications/:app_id/approve", api.GuildApplicationApproveHandler)
			guilds.POST("/:id/applications/:app_id/reject", api.GuildApplicationRejectHandler)
			guilds.POST("/:id/transfer-leader", api.GuildTransferLeaderHandler)
			guilds.GET("/:id/invite-link", api.GuildInviteLinkGetHandler)
			guilds.POST("/:id/invite-link", api.GuildInviteLinkCreateHandler)
			guilds.GET("/:id/default-slot", api.GuildDefaultSlotGetHandler)
			guilds.PUT("/:id/default-slot", api.GuildDefaultSlotUpsertHandler)
			guilds.DELETE("/:id/default-slot", api.GuildDefaultSlotDeleteHandler)
			guilds.GET("/:id/report-rotation-suggestion", api.GuildReportRotationHandler)
			guilds.POST("/:id/activities", api.ActivityCreateHandler)
			guilds.GET("/:id/activities", api.GuildActivitiesHandler)
		}

		// Activities
		v1Auth.GET("/activities/:id", api.ActivityGetHandler)
		v1Auth.PUT("/activities/:id", api.ActivityUpdateHandler)
		v1Auth.POST("/activities/:id/abort", api.ActivityAbortHandler)

		// Topics
		topics := v1Auth.Group("/topics")
		{
			topics.GET("/announce/search", api.TopicAnnounceSearchHandler)
			topics.GET("/random", api.TopicRandomHandler)
			topics.GET("/report-suggestions", api.ReportThemeSuggestionsHandler)
			topics.GET("/:id", api.TopicGetHandler)
		}

		// Guild Attendees
		guildAttendees := v1Auth.Group("/guild-attendees")
		{
			guildAttendees.POST("", api.GuildAttendeeCreateHandler)
			guildAttendees.GET("/:id", api.GuildAttendeeGetHandler)
			guildAttendees.PUT("/:id", api.GuildAttendeeUpdateHandler)
			guildAttendees.DELETE("/:id", api.GuildAttendeeDeleteHandler)
			guildAttendees.GET("/guild/:guild_id", api.GuildAttendeeGetByGuildHandler)
			guildAttendees.GET("/user/:user_id", api.GuildAttendeeGetByUserHandler)
		}

		// Transcripts
		transcripts := v1Auth.Group("/transcripts")
		{
			transcripts.POST("", api.TranscriptCreateHandler)
			transcripts.GET("/:id", api.TranscriptGetHandler)
			transcripts.PUT("/:id", api.TranscriptUpdateHandler)
			transcripts.DELETE("/:id", api.TranscriptDeleteHandler)
			transcripts.GET("/event/:event_id", api.TranscriptGetByEventHandler)
			transcripts.GET("/user/:user_id", api.TranscriptGetByUserHandler)
		}

		// Event Attendees
		eventAttendees := v1Auth.Group("/event-attendees")
		{
			eventAttendees.POST("", api.EventAttendeeCreateHandler)
			eventAttendees.GET("/:id", api.EventAttendeeGetHandler)
			eventAttendees.PUT("/:id", api.EventAttendeeUpdateHandler)
			eventAttendees.DELETE("/:id", api.EventAttendeeDeleteHandler)
			eventAttendees.GET("/event/:event_id", api.EventAttendeeGetByEventHandler)
			eventAttendees.GET("/user/:user_id", api.EventAttendeeGetByUserHandler)
		}

		// Users
		users := v1Auth.Group("/users")
		{
			users.POST("", api.UserCreateHandler)
			users.GET("/:id", api.UserGetHandler)
			users.PUT("/:id", api.UserUpdateHandler)
			users.DELETE("/:id", api.UserDeleteHandler)
			users.POST("/init", api.UserInitHandler)
			users.GET("/me", api.UserMeHandler)
			users.PUT("/me", api.UserMeUpdateHandler)
			users.GET("/name/:name", api.UserGetByNameHandler)
			users.GET("/email/:email", api.UserGetByEmailHandler)
		}
	}
}
