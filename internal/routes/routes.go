package routes

import (
	"jpcorrect-backend/internal/api"

	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, api *api.API) {
	r.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	// AI Corrections
	ai_corrections := r.Group("/ai_corrections")
	{
		ai_corrections.POST("", api.AICorrectionCreateHandler)
		ai_corrections.GET("/:id", api.AICorrectionGetHandler)
		ai_corrections.PUT("/:id", api.AICorrectionUpdateHandler)
		ai_corrections.DELETE("/:id", api.AICorrectionDeleteHandler)
		ai_corrections.GET("/mistake/:mistake_id", api.AICorrectionGetByMistakeHandler)
	}

	// Mistakes
	mistakes := r.Group("/mistakes")
	{
		mistakes.POST("", api.MistakeCreateHandler)
		mistakes.GET("/:id", api.MistakeGetHandler)
		mistakes.PUT("/:id", api.MistakeUpdateHandler)
		mistakes.DELETE("/:id", api.MistakeDeleteHandler)
		mistakes.GET("/practice/:practice_id", api.MistakeGetByPracticeHandler)
		mistakes.GET("/user/:user_id", api.MistakeGetByUserHandler)
	}

	// Notes
	notes := r.Group("/notes")
	{
		notes.POST("", api.NoteCreateHandler)
		notes.GET("/:id", api.NoteGetHandler)
		notes.PUT("/:id", api.NoteUpdateHandler)
		notes.DELETE("/:id", api.NoteDeleteHandler)
		notes.GET("/practice/:practice_id", api.NoteGetByPracticeHandler)
	}

	// Practices
	practices := r.Group("/practices")
	{
		practices.POST("", api.PracticeCreateHandler)
		practices.GET("/:id", api.PracticeGetHandler)
		practices.PUT("/:id", api.PracticeUpdateHandler)
		practices.DELETE("/:id", api.PracticeDeleteHandler)
		practices.GET("/user/:user_id", api.PracticeGetByUserHandler)
	}

	// Transcripts
	transcripts := r.Group("/transcripts")
	{
		transcripts.POST("", api.TranscriptCreateHandler)
		transcripts.GET("/:id", api.TranscriptGetHandler)
		transcripts.PUT("/:id", api.TranscriptUpdateHandler)
		transcripts.DELETE("/:id", api.TranscriptDeleteHandler)
		transcripts.GET("/mistake/:mistake_id", api.TranscriptGetByMistakeHandler)
	}

	// Users
	users := r.Group("/users")
	{
		users.POST("", api.UserCreateHandler)
		users.GET("/:id", api.UserGetHandler)
		users.PUT("/:id", api.UserUpdateHandler)
		users.DELETE("/:id", api.UserDeleteHandler)
		users.GET("/name/:name", api.UserGetByNameHandler)
	}
}
