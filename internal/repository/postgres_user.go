package repository

import (
	"context"

	"jpcorrect-backend/internal/domain"
)

type postgresUserRepository struct {
	conn Connection
}

func NewPostgresUser(conn Connection) domain.UserRepository {
	return &postgresUserRepository{conn: conn}
}

func (u *postgresUserRepository) fetch(ctx context.Context, query string, args ...any) ([]*domain.User, error) {
	rows, err := u.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(
			&user.UserID,
			&user.Name,
		); err != nil {
			return nil, err
		}
		users = append(users, &user)
	}
	return users, nil
}

func (u *postgresUserRepository) GetByID(ctx context.Context, userID int) (*domain.User, error) {
	query := `
		SELECT user_id, name
		FROM jpcorrect.user
		WHERE user_id = $1`

	users, err := u.fetch(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, domain.ErrNotFound
	}
	return users[0], nil
}

func (u *postgresUserRepository) GetByName(ctx context.Context, name string) ([]*domain.User, error) {
	query := `
		SELECT user_id, name
		FROM jpcorrect.user
		WHERE name = $1`

	users, err := u.fetch(ctx, query, name)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, domain.ErrNotFound
	}
	return users, nil
}

func (u *postgresUserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO jpcorrect.user (name)
		VALUES ($1)
		RETURNING user_id`

	if err := u.conn.QueryRow(ctx, query, user.Name).Scan(&user.UserID); err != nil {
		return err
	}
	return nil
}

func (u *postgresUserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE jpcorrect.user
		SET name = $1
		WHERE user_id = $2`

	if _, err := u.conn.Exec(ctx, query, user.Name, user.UserID); err != nil {
		return err
	}
	return nil
}

func (u *postgresUserRepository) Delete(ctx context.Context, userID int) error {
	query := `
		DELETE FROM jpcorrect.user
		WHERE user_id = $1`

	if _, err := u.conn.Exec(ctx, query, userID); err != nil {
		return err
	}
	return nil
}
