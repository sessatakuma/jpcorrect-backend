package repository

import (
	"context"

	"jpcorrect-backend/internal/domain"
)

type postgresTranscriptRepository struct {
	conn Connection
}

func NewPostgresTranscript(conn Connection) domain.TranscriptRepository {
	return &postgresTranscriptRepository{conn: conn}
}

func (p *postgresTranscriptRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.Transcript, error) {
	rows, err := p.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transcripts []*domain.Transcript
	for rows.Next() {
		var transcript domain.Transcript
		if err := rows.Scan(
			&transcript.TranscriptID,
			&transcript.MistakeID,
			&transcript.Content,
			&transcript.Furigana,
			&transcript.Accent,
		); err != nil {
			return nil, err
		}
		transcripts = append(transcripts, &transcript)
	}
	return transcripts, nil
}

func (p *postgresTranscriptRepository) GetByID(ctx context.Context, transcriptID int) (*domain.Transcript, error) {
	query := `
		SELECT transcript_id, mistake_id, content, furigana, accent
		FROM jpcorrect.transcript
		WHERE transcript_id = $1`

	transcripts, err := p.fetch(ctx, query, transcriptID)
	if err != nil {
		return nil, err
	}
	if len(transcripts) == 0 {
		return nil, domain.ErrNotFound
	}
	return transcripts[0], nil
}

func (p *postgresTranscriptRepository) GetByMistakeID(ctx context.Context, mistakeID int) (*domain.Transcript, error) {
	query := `
		SELECT transcript_id, mistake_id, content, furigana, accent
		FROM jpcorrect.transcript
		WHERE mistake_id = $1`

	transcripts, err := p.fetch(ctx, query, mistakeID)
	if err != nil {
		return nil, err
	}
	if len(transcripts) == 0 {
		return nil, domain.ErrNotFound
	}
	return transcripts[0], nil
}

func (p *postgresTranscriptRepository) Create(ctx context.Context, transcript *domain.Transcript) error {
	query := `
		INSERT INTO jpcorrect.transcript (mistake_id, content, furigana, accent)
		VALUES ($1, $2, $3, $4)
		RETURNING transcript_id`

	if err := p.conn.QueryRow(ctx, query, transcript.MistakeID, transcript.Content, transcript.Furigana, transcript.Accent).Scan(&transcript.TranscriptID); err != nil {
		return err
	}
	return nil
}

func (p *postgresTranscriptRepository) Update(ctx context.Context, transcript *domain.Transcript) error {
	query := `
		UPDATE jpcorrect.transcript
		SET mistake_id = $1, content = $2, furigana = $3, accent = $4
		WHERE transcript_id = $5`

	if _, err := p.conn.Exec(ctx, query, transcript.MistakeID, transcript.Content, transcript.Furigana, transcript.Accent, transcript.TranscriptID); err != nil {
		return err
	}
	return nil
}

func (p *postgresTranscriptRepository) Delete(ctx context.Context, transcriptID int) error {
	query := `
		DELETE FROM jpcorrect.transcript
		WHERE transcript_id = $1`

	if _, err := p.conn.Exec(ctx, query, transcriptID); err != nil {
		return err
	}
	return nil
}
