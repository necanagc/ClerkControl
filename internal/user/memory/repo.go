// Package memory хранит пользователей в памяти процесса.
package memory

import (
	"context"
	"strings"
	"sync"

	"taskbot/internal/user"
)

// Repo — потокобезопасное хранилище пользователей.
type Repo struct {
	mu   sync.Mutex
	byID map[int64]user.User
}

// New создаёт пустое хранилище.
func New() *Repo {
	return &Repo{byID: make(map[int64]user.User)}
}

// Upsert сохраняет или обновляет пользователя.
func (r *Repo) Upsert(_ context.Context, u user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byID[u.ID] = u

	return nil
}

// ByUsername ищет пользователя без учёта регистра.
func (r *Repo) ByUsername(_ context.Context, username string) (user.User, error) {
	want := strings.ToLower(strings.TrimPrefix(username, "@"))

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.byID {
		if strings.ToLower(u.Username) == want {
			return u, nil
		}
	}

	return user.User{}, user.ErrNotFound
}
