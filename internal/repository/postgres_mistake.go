package repository

import (
	"context"

	"jpcorrect-backend/internal/domain"
)

type postgresMistakeRepository struct {
	conn Connection
}

func NewPostgresMistake(conn Connection) domain.MistakeRepository {
	return &postgresMistakeRepository{conn: conn}
}

func (p *postgresMistakeRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.Mistake, error) {
	rows, err := p.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mistakes []*domain.Mistake
	for rows.Next() {
		var mistake domain.Mistake
		if err := rows.Scan(
			&mistake.MistakeID,
			&mistake.PracticeID,
			&mistake.UserID,
			&mistake.StartTime,
			&mistake.EndTime,
			&mistake.MistakeStatus,
			&mistake.MistakeType,
		); err != nil {
			return nil, err
		}
		mistakes = append(mistakes, &mistake)
	}
	return mistakes, nil
}

func (p *postgresMistakeRepository) GetByID(ctx context.Context, mistakeID int) (*domain.Mistake, error) {
	query := `
		SELECT mistake_id, practice_id, user_id, start_time, end_time, mistake_status, mistake_type
		FROM jpcorrect.mistake
		WHERE mistake_id = $1`

	mistakes, err := p.fetch(ctx, query, mistakeID)
	if err != nil {
		return nil, err
	}
	if len(mistakes) == 0 {
		return nil, domain.ErrNotFound
	}
	return mistakes[0], nil
}

func (p *postgresMistakeRepository) GetByPracticeID(ctx context.Context, practiceID int) ([]*domain.Mistake, error) {
	query := `
		SELECT mistake_id, practice_id, user_id, start_time, end_time, mistake_status, mistake_type
		FROM jpcorrect.mistake
		WHERE practice_id = $1`

	mistakes, err := p.fetch(ctx, query, practiceID)
	if err != nil {
		return nil, err
	}
	if len(mistakes) == 0 {
		return nil, domain.ErrNotFound
	}
	return mistakes, nil
}

func (p *postgresMistakeRepository) GetByUserID(ctx context.Context, userID int) ([]*domain.Mistake, error) {
	query := `
		SELECT mistake_id, practice_id, user_id, start_time, end_time, mistake_status, mistake_type
		FROM jpcorrect.mistake
		WHERE user_id = $1`

	mistakes, err := p.fetch(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	if len(mistakes) == 0 {
		return nil, domain.ErrNotFound
	}
	return mistakes, nil
}

func (p *postgresMistakeRepository) Create(ctx context.Context, m *domain.Mistake) error {
	query := `
		INSERT INTO jpcorrect.mistake (practice_id, user_id, start_time, end_time, mistake_status, mistake_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING mistake_id`

	if err := p.conn.QueryRow(ctx, query, m.PracticeID, m.UserID, m.StartTime, m.EndTime, m.MistakeStatus, m.MistakeType).Scan(&m.MistakeID); err != nil {
		return err
	}
	return nil
}

func (p *postgresMistakeRepository) Update(ctx context.Context, m *domain.Mistake) error {
	query := `
		UPDATE jpcorrect.mistake
		SET practice_id = $1, user_id = $2, start_time = $3, end_time = $4, mistake_status = $5, mistake_type = $6
		WHERE mistake_id = $7`

	if _, err := p.conn.Exec(ctx, query, m.PracticeID, m.UserID, m.StartTime, m.EndTime, m.MistakeStatus, m.MistakeType, m.MistakeID); err != nil {
		return err
	}
	return nil
}

func (p *postgresMistakeRepository) Delete(ctx context.Context, mistakeID int) error {
	query := `
		DELETE FROM jpcorrect.mistake
		WHERE mistake_id = $1`

	if _, err := p.conn.Exec(ctx, query, mistakeID); err != nil {
		return err
	}
	return nil
}
