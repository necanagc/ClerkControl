package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"taskbot/internal/user"
)

// UserRepo — реализация user.Repository поверх того же пула, что и задачи.
type UserRepo struct {
	pool *pgxpool.Pool
}

// Users отдаёт хранилище пользователей, работающее с тем же пулом.
func (r *Repo) Users() *UserRepo {
	return &UserRepo{pool: r.pool}
}

const upsertUser = `
INSERT INTO users (id, username, first_name)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE
SET username = EXCLUDED.username,
    first_name = EXCLUDED.first_name,
    updated_at = now()`

// Upsert сохраняет пользователя или обновляет ник.
func (r *UserRepo) Upsert(ctx context.Context, u user.User) error {
	if _, err := r.pool.Exec(ctx, upsertUser, u.ID, u.Username, u.FirstName); err != nil {
		return fmt.Errorf("postgres: upsert user: %w", err)
	}

	return nil
}

const selectUserByUsername = `
SELECT id, username, first_name
FROM users
WHERE lower(username) = lower($1)
LIMIT 1`

// ByUsername ищет пользователя по нику без учёта регистра.
func (r *UserRepo) ByUsername(ctx context.Context, username string) (user.User, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")

	var u user.User
	err := r.pool.QueryRow(ctx, selectUserByUsername, username).Scan(&u.ID, &u.Username, &u.FirstName)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("postgres: user by username: %w", err)
	}

	return u, nil
}
