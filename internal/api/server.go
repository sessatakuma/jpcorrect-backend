package api

import (
	"net/http"

	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

type API struct {
	apiToolsURL      string
	httpClient       *http.Client
	aiCorrectionRepo domain.AICorrectionRepository
	mistakeRepo      domain.MistakeRepository
	noteRepo         domain.NoteRepository
	practiceRepo     domain.PracticeRepository
	transcriptRepo   domain.TranscriptRepository
	userRepo         domain.UserRepository
}

func NewAPI(url string, client *http.Client, conn repository.Connection) *API {
	aiCorrectionRepo := repository.NewPostgresAICorrection(conn)
	mistakeRepo := repository.NewPostgresMistake(conn)
	noteRepo := repository.NewPostgresNote(conn)
	practiceRepo := repository.NewPostgresPractice(conn)
	transcriptRepo := repository.NewPostgresTranscript(conn)
	userRepo := repository.NewPostgresUser(conn)

	return &API{
		apiToolsURL:      url,
		httpClient:       client,
		aiCorrectionRepo: aiCorrectionRepo,
		mistakeRepo:      mistakeRepo,
		noteRepo:         noteRepo,
		practiceRepo:     practiceRepo,
		transcriptRepo:   transcriptRepo,
		userRepo:         userRepo,
	}
}

func Register(r *gin.Engine, api *API) {
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	v1 := r.Group("/v1")
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
