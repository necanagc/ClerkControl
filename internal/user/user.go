// Package user хранит соответствие «username → id» для назначения задач.
package user

import (
	"context"
	"errors"
)

// ErrNotFound — пользователь ещё не писал боту, его username неизвестен.
var ErrNotFound = errors.New("user: not found")

// User — известный боту пользователь Telegram.
type User struct {
	ID        int64
	Username  string
	FirstName string
}

// Repository — хранилище пользователей.
type Repository interface {
	Upsert(ctx context.Context, u User) error
	// ByUsername ищет без учёта регистра и без ведущей «@».
	ByUsername(ctx context.Context, username string) (User, error)
}
