package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/MicahParks/keyfunc/v3"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"

	"github.com/gin-gonic/gin"
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
	eventRepo         domain.EventRepository
	eventAttendeeRepo domain.EventAttendeeRepository
	transcriptRepo    domain.TranscriptRepository
	mistakeRepo       domain.MistakeRepository
}

func NewAPI(url string, transport *http.Transport, db *gorm.DB, jwksURL string) *API {
	return &API{
		db:                db,
		apiToolsURL:       url,
		proxyTransport:    transport,
		jwksURL:           jwksURL,
		userRepo:          repository.NewGormUserRepository(db),
		eventRepo:         repository.NewGormEventRepository(db),
		eventAttendeeRepo: repository.NewGormEventAttendeeRepository(db),
		transcriptRepo:    repository.NewGormTranscriptRepository(db),
		mistakeRepo:       repository.NewGormMistakeRepository(db),
	}
}

func Register(r *gin.Engine, api *API) {
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

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
			mistakes.GET("/event/:event_id", api.MistakeGetByPracticeHandler)
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
