package api

import (
	"context"
	"log"
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
	db                        *gorm.DB
	apiToolsURL               string
	clientAPIKey              string
	proxyTransport            *http.Transport
	jwksURL                   string
	jwksCache                 keyfunc.Keyfunc
	jwksCtx                   context.Context
	jwksCancel                context.CancelFunc
	jwksMutex                 sync.Mutex
	jwksErr                   error
	userRepo                  domain.UserRepository
	guildRepo                 domain.GuildRepository
	guildAttendeeRepo         domain.GuildAttendeeRepository
	eventRepo                 domain.EventRepository
	eventAttendeeRepo         domain.EventAttendeeRepository
	transcriptRepo            domain.TranscriptRepository
	mistakeRepo               domain.MistakeRepository
	activityRepo              domain.ActivityRepository
	guildDefaultSlotRepo      domain.GuildDefaultSlotRepository
	topicRepo                 domain.TopicRepository
	reportThemeSuggestionRepo domain.ReportThemeSuggestionRepository
	joinRequestRepo           domain.JoinRequestRepository
	webrtcHub                 domain.WebRTCHub
	rateLimiter               *RateLimiter
	upgrader                  websocket.Upgrader
}

func NewAPI(url string, clientAPIKey string, transport *http.Transport, db *gorm.DB, jwksURL string, allowedOrigins []string) *API {
	userRepo := repository.NewGormUserRepository(db)
	guildRepo := repository.NewGormGuildRepository(db)
	guildAttendeeRepo := repository.NewGormGuildAttendeeRepository(db)
	eventRepo := repository.NewGormEventRepository(db)
	eventAttendeeRepo := repository.NewGormEventAttendeeRepository(db)
	transcriptRepo := repository.NewGormTranscriptRepository(db)
	mistakeRepo := repository.NewGormMistakeRepository(db)
	activityRepo := repository.NewGormActivityRepository(db)
	guildDefaultSlotRepo := repository.NewGormGuildDefaultSlotRepository(db)
	topicRepo := repository.NewGormTopicRepository(db)
	reportThemeSuggestionRepo := repository.NewGormReportThemeSuggestionRepository(db)
	joinRequestRepo := repository.NewGormJoinRequestRepository(db)
	webrtcHub := NewHub()
	rateLimiter := NewRateLimiter(10*time.Second, 15)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if len(allowedOrigins) == 0 {
				return gin.IsDebugging()
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
		db:                        db,
		apiToolsURL:               url,
		clientAPIKey:              clientAPIKey,
		proxyTransport:            transport,
		jwksURL:                   jwksURL,
		userRepo:                  userRepo,
		guildRepo:                 guildRepo,
		guildAttendeeRepo:         guildAttendeeRepo,
		eventRepo:                 eventRepo,
		eventAttendeeRepo:         eventAttendeeRepo,
		transcriptRepo:            transcriptRepo,
		mistakeRepo:               mistakeRepo,
		activityRepo:              activityRepo,
		guildDefaultSlotRepo:      guildDefaultSlotRepo,
		topicRepo:                 topicRepo,
		reportThemeSuggestionRepo: reportThemeSuggestionRepo,
		joinRequestRepo:           joinRequestRepo,
		webrtcHub:                 webrtcHub,
		rateLimiter:               rateLimiter,
		upgrader:                  upgrader,
	}
}

func (api *API) Close() {
	api.rateLimiter.Close()
}

func Register(r *gin.Engine, api *API) {
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	// Only register Swagger endpoint in development mode
	if gin.IsDebugging() {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	// WebRTC WebSocket endpoint
	r.GET("/ws", api.ServeWebSocket)

	// Invite info is public: the landing page must render before login.
	r.GET("/v1/invites/:token", api.InviteInfoHandler)

	// In debug mode (GIN_MODE=debug), APIKeyMiddleware + AuthMiddleware are
	// skipped so the dev / internal deployment can be gated solely by an edge
	// gateway like Cloudflare Access. Production (GIN_MODE=release) enforces
	// both middlewares as usual.
	debug := gin.IsDebugging()
	if debug {
		log.Println("⚠️  GIN_MODE=debug — APIKeyMiddleware and AuthMiddleware are DISABLED on /v1 routes. An edge gateway (e.g. Cloudflare Access) must protect any non-localhost exposure of this build.")
		log.Println("GIN_MODE=debug — use X-Debug-User-ID to select a caller for user-specific operations.")
	}

	// API Tools — non-user-specific service access, X-API-Key only (release only)
	apiTools := r.Group("/v1")
	if !debug {
		apiTools.Use(api.APIKeyMiddleware())
	}
	{
		apiTools.POST("/mark-accent", api.MarkAccentHandler)
		apiTools.POST("/mark-accent/stream", api.MarkAccentStreamHandler)
		apiTools.POST("/usage-query/headwords", api.UsageQueryHeadWordsHandler)
		apiTools.POST("/usage-query/url", api.UsageQueryURLHandler)
		apiTools.POST("/usage-query/id-details", api.UsageQueryIDDetailsHandler)
		apiTools.POST("/dict-query", api.DictQueryHandler)
		apiTools.POST("/sentence-query", api.SentenceQueryHandler)
	}

	// v1 requires a valid JWT in release mode. It does not require a local
	// User row: /v1/users/init provisions one on first login.
	v1 := r.Group("/v1")
	if debug {
		v1.Use(debugCallerMiddleware())
	} else {
		v1.Use(api.AuthMiddleware())
	}

	// First-login provisioning deliberately sits before RequireLocalUser.
	v1.POST("/users/init", api.UserInitHandler)

	// Everything else needs a local account.
	authed := v1.Group("")
	if !debug {
		authed.Use(api.RequireLocalUser())
	}
	{
		// Invites (auth-required)
		authed.POST("/invites/:token/accept", api.InviteAcceptHandler)

		// Mistakes
		mistakes := authed.Group("/mistakes")
		{
			mistakes.POST("", api.MistakeCreateHandler)
			mistakes.GET("/:id", api.MistakeGetHandler)
			mistakes.PUT("/:id", api.MistakeUpdateHandler)
			mistakes.DELETE("/:id", api.MistakeDeleteHandler)
			mistakes.GET("/event/:event_id", api.MistakeGetByEventHandler)
			mistakes.GET("/user/:user_id", api.MistakeGetByUserHandler)
		}

		// Practices (keep old route for backward compatibility)
		practices := authed.Group("/practices")
		{
			practices.POST("", api.PracticeCreateHandler)
			practices.GET("/:id", api.PracticeGetHandler)
			practices.PUT("/:id", api.PracticeUpdateHandler)
			practices.DELETE("/:id", api.PracticeDeleteHandler)
			practices.GET("/user/:user_id", api.PracticeGetByUserHandler)
		}

		// Guilds
		guilds := authed.Group("/guilds")
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
		authed.GET("/activities/:id", api.ActivityGetHandler)
		authed.PUT("/activities/:id", api.ActivityUpdateHandler)
		authed.POST("/activities/:id/abort", api.ActivityAbortHandler)

		// Topics
		topics := authed.Group("/topics")
		{
			topics.GET("/announce/search", api.TopicAnnounceSearchHandler)
			topics.GET("/random", api.TopicRandomHandler)
			topics.GET("/report-suggestions", api.ReportThemeSuggestionsHandler)
			topics.GET("/:id", api.TopicGetHandler)
		}

		// Guild Attendees
		guildAttendees := authed.Group("/guild-attendees")
		{
			guildAttendees.POST("", api.GuildAttendeeCreateHandler)
			guildAttendees.GET("/:id", api.GuildAttendeeGetHandler)
			guildAttendees.PUT("/:id", api.GuildAttendeeUpdateHandler)
			guildAttendees.DELETE("/:id", api.GuildAttendeeDeleteHandler)
			guildAttendees.GET("/guild/:guild_id", api.GuildAttendeeGetByGuildHandler)
			guildAttendees.GET("/user/:user_id", api.GuildAttendeeGetByUserHandler)
		}

		// Transcripts
		transcripts := authed.Group("/transcripts")
		{
			transcripts.POST("", api.TranscriptCreateHandler)
			transcripts.GET("/:id", api.TranscriptGetHandler)
			transcripts.PUT("/:id", api.TranscriptUpdateHandler)
			transcripts.DELETE("/:id", api.TranscriptDeleteHandler)
			transcripts.GET("/event/:event_id", api.TranscriptGetByEventHandler)
			transcripts.GET("/user/:user_id", api.TranscriptGetByUserHandler)
		}

		// Event Attendees
		eventAttendees := authed.Group("/event-attendees")
		{
			eventAttendees.POST("", api.EventAttendeeCreateHandler)
			eventAttendees.GET("/:id", api.EventAttendeeGetHandler)
			eventAttendees.PUT("/:id", api.EventAttendeeUpdateHandler)
			eventAttendees.DELETE("/:id", api.EventAttendeeDeleteHandler)
			eventAttendees.GET("/event/:event_id", api.EventAttendeeGetByEventHandler)
			eventAttendees.GET("/user/:user_id", api.EventAttendeeGetByUserHandler)
		}

		// Users (excluding /users/init, mounted above)
		users := authed.Group("/users")
		{
			// Raw user CRUD is a legacy management route. Issue #78 allows only a
			// system admin, because these handlers bind the whole domain.User and
			// would otherwise let any caller rewrite the server-owned identity
			// mapping (supabase_user_id), email, role or status. Account
			// provisioning and self-service edits go through /users/init and
			// /users/me, which bind allowlisted DTOs.
			users.POST("", api.RequireSystemAdmin(), api.UserCreateHandler)
			users.GET("/me", api.UserMeHandler)
			users.PUT("/me", api.UserMeUpdateHandler)
			users.GET("/:id", api.UserGetHandler)
			users.PUT("/:id", api.RequireSystemAdmin(), api.UserUpdateHandler)
			users.DELETE("/:id", api.RequireSystemAdmin(), api.UserDeleteHandler)
			users.GET("/name/:name", api.UserGetByNameHandler)
			users.GET("/email/:email", api.UserGetByEmailHandler)
		}
	}
}
