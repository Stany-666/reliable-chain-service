package user

import (
	"context"
	"database/sql"
	"errors"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ExistsByUsernameEmail(ctx context.Context, username string, email string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM users WHERE email = $1 OR username = $2",
		email,
		username,
	).Scan(&count)

	if err != nil {
		return false, err
	}

	if count > 0 {
		return true, nil
	}

	return false, nil
}

func (r *Repository) CreateUser(ctx context.Context, user User) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO users (username, email, password_hash) VALUES ($1, $2, $3)",
		user.Username,
		user.Email,
		user.PasswordHash,
	)
	return err
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, username, email, password_hash FROM users WHERE username = $1",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash)

	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}
