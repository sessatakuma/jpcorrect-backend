package repository

import (
	"context"

	"jpcorrect-backend/internal/domain"
)

type postgresAICorrectionRepository struct {
	conn Connection
}

func NewPostgresAICorrection(conn Connection) domain.AICorrectionRepository {
	return &postgresAICorrectionRepository{conn: conn}
}

func (p *postgresAICorrectionRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.AICorrection, error) {
	rows, err := p.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aiCorrections []*domain.AICorrection
	for rows.Next() {
		var aiCorrection domain.AICorrection
		if err := rows.Scan(
			&aiCorrection.AICorrectionID,
			&aiCorrection.MistakeID,
			&aiCorrection.Content,
		); err != nil {
			return nil, err
		}
		aiCorrections = append(aiCorrections, &aiCorrection)
	}
	return aiCorrections, nil
}

func (p *postgresAICorrectionRepository) GetByID(ctx context.Context, aiCorrectionID int) (*domain.AICorrection, error) {
	query := `
		SELECT ai_correction_id, mistake_id, content
		FROM jpcorrect.ai_correction
		WHERE ai_correction_id = $1`

	aiCorrections, err := p.fetch(ctx, query, aiCorrectionID)
	if err != nil {
		return nil, err
	}
	if len(aiCorrections) == 0 {
		return nil, domain.ErrNotFound
	}
	return aiCorrections[0], nil
}

func (p *postgresAICorrectionRepository) GetByMistakeID(ctx context.Context, mistakeID int) (*domain.AICorrection, error) {
	query := `
		SELECT ai_correction_id, mistake_id, content
		FROM jpcorrect.ai_correction
		WHERE mistake_id = $1`

	aiCorrections, err := p.fetch(ctx, query, mistakeID)
	if err != nil {
		return nil, err
	}
	if len(aiCorrections) == 0 {
		return nil, domain.ErrNotFound
	}
	return aiCorrections[0], nil
}

func (p *postgresAICorrectionRepository) Create(ctx context.Context, aiCorrection *domain.AICorrection) error {
	query := `
		INSERT INTO jpcorrect.ai_correction (mistake_id, content)
		VALUES ($1, $2)
		RETURNING ai_correction_id`

	if err := p.conn.QueryRow(ctx, query, aiCorrection.MistakeID, aiCorrection.Content).Scan(&aiCorrection.AICorrectionID); err != nil {
		return err
	}
	return nil
}

func (p *postgresAICorrectionRepository) Update(ctx context.Context, aiCorrection *domain.AICorrection) error {
	query := `
		UPDATE jpcorrect.ai_correction
		SET mistake_id = $1, content = $2
		WHERE ai_correction_id = $3`

	if _, err := p.conn.Exec(ctx, query, aiCorrection.MistakeID, aiCorrection.Content, aiCorrection.AICorrectionID); err != nil {
		return err
	}
	return nil
}

func (p *postgresAICorrectionRepository) Delete(ctx context.Context, aiCorrectionID int) error {
	query := `
		DELETE FROM jpcorrect.ai_correction
		WHERE ai_correction_id = $1`

	if _, err := p.conn.Exec(ctx, query, aiCorrectionID); err != nil {
		return err
	}
	return nil
}
