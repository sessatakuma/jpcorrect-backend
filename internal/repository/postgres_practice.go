package repository

import (
	"context"
	"time"

	"jpcorrect-backend/internal/domain"
)

type postgresPracticeRepository struct {
	conn Connection
}

func NewPostgresPractice(conn Connection) domain.PracticeRepository {
	return &postgresPracticeRepository{conn: conn}
}

func (p *postgresPracticeRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.Practice, error) {
	rows, err := p.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var practices []*domain.Practice
	for rows.Next() {
		var practice domain.Practice
		var date time.Time
		if err := rows.Scan(
			&practice.PracticeID,
			&practice.UserID,
			&date,
			&practice.Duration,
		); err != nil {
			return nil, err
		}
		practice.Date = date.Format(time.DateOnly)
		practices = append(practices, &practice)
	}
	return practices, nil
}

func (p *postgresPracticeRepository) GetByID(ctx context.Context, practiceID int) (*domain.Practice, error) {
	query := `
        SELECT practice_id, user_id, date, duration
        FROM jpcorrect.practice
        WHERE practice_id = $1`

	practices, err := p.fetch(ctx, query, practiceID)
	if err != nil {
		return nil, err
	}
	if len(practices) == 0 {
		return nil, domain.ErrNotFound
	}
	return practices[0], nil
}

func (p *postgresPracticeRepository) GetByUserID(ctx context.Context, userID int) ([]*domain.Practice, error) {
	query := `
        SELECT practice_id, user_id, date, duration
        FROM jpcorrect.practice
        WHERE user_id = $1`

	practices, err := p.fetch(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	if len(practices) == 0 {
		return nil, domain.ErrNotFound
	}
	return practices, nil
}

func (p *postgresPracticeRepository) Create(ctx context.Context, practice *domain.Practice) error {
	query := `
        INSERT INTO jpcorrect.practice (user_id, date, duration)
        VALUES ($1, $2, $3)
        RETURNING practice_id`

	if err := p.conn.QueryRow(ctx, query, practice.UserID, practice.Date, practice.Duration).Scan(&practice.PracticeID); err != nil {
		return err
	}
	return nil
}

func (p *postgresPracticeRepository) Update(ctx context.Context, practice *domain.Practice) error {
	query := `
        UPDATE jpcorrect.practice
        SET user_id = $1, date = $2, duration = $3
        WHERE practice_id = $4`

	if _, err := p.conn.Exec(ctx, query, practice.UserID, practice.Date, practice.Duration, practice.PracticeID); err != nil {
		return err
	}
	return nil
}

func (p *postgresPracticeRepository) Delete(ctx context.Context, practiceID int) error {
	query := `
        DELETE FROM jpcorrect.practice
        WHERE practice_id = $1`

	if _, err := p.conn.Exec(ctx, query, practiceID); err != nil {
		return err
	}
	return nil
}
