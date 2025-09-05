package repository

import (
	"context"

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
		if err := rows.Scan(
			&practice.PracticeID,
			&practice.UserID,
		); err != nil {
			return nil, err
		}
		practices = append(practices, &practice)
	}
	return practices, nil
}

func (p *postgresPracticeRepository) GetByID(ctx context.Context, practiceID int) (*domain.Practice, error) {
	query := `
		SELECT practice_id, user_id
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
		SELECT practice_id, user_id
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
		INSERT INTO jpcorrect.practice (user_id)
		VALUES ($1)
		RETURNING practice_id`

	if err := p.conn.QueryRow(ctx, query, practice.UserID).Scan(&practice.PracticeID); err != nil {
		return err
	}
	return nil
}

func (p *postgresPracticeRepository) Update(ctx context.Context, practice *domain.Practice) error {
	query := `
		UPDATE jpcorrect.practice
		SET user_id = $1
		WHERE practice_id = $2`

	if _, err := p.conn.Exec(ctx, query, practice.UserID, practice.PracticeID); err != nil {
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
