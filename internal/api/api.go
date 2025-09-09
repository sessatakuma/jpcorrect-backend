package api

import (
	"jpcorrect-backend/internal/domain"
	"jpcorrect-backend/internal/repository"
)

type API struct {
	aiCorrectionRepo domain.AICorrectionRepository
	mistakeRepo      domain.MistakeRepository
	noteRepo         domain.NoteRepository
	practiceRepo     domain.PracticeRepository
	transcriptRepo   domain.TranscriptRepository
	userRepo         domain.UserRepository
}

func NewAPI(conn repository.Connection) *API {
	aiCorrectionRepo := repository.NewPostgresAICorrection(conn)
	mistakeRepo := repository.NewPostgresMistake(conn)
	noteRepo := repository.NewPostgresNote(conn)
	practiceRepo := repository.NewPostgresPractice(conn)
	transcriptRepo := repository.NewPostgresTranscript(conn)
	userRepo := repository.NewPostgresUser(conn)

	return &API{
		aiCorrectionRepo: aiCorrectionRepo,
		mistakeRepo:      mistakeRepo,
		noteRepo:         noteRepo,
		practiceRepo:     practiceRepo,
		transcriptRepo:   transcriptRepo,
		userRepo:         userRepo,
	}
}
