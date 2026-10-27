// Package memory хранит задачи в памяти процесса.
// Используется для быстрых тестов и как временное хранилище до перехода на Postgres.
package memory

import (
	"context"
	"sync"

	"taskbot/internal/task"
)

// Repo — потокобезопасное хранилище в памяти.
type Repo struct {
	mu     sync.Mutex
	tasks  []task.Task
	nextID int
}

// New создаёт пустое хранилище. Нумерация начинается с 1.
func New() *Repo {
	return &Repo{nextID: 1}
}

// Create сохраняет задачу и выдаёт ей ID.
func (r *Repo) Create(_ context.Context, t task.Task) (task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t.ID = r.nextID
	r.nextID++
	r.tasks = append(r.tasks, t)

	return t, nil
}

// Update заменяет задачу с тем же ID (в пределах чата, если он указан).
func (r *Repo) Update(_ context.Context, t task.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.tasks {
		if r.tasks[i].ID != t.ID {
			continue
		}
		if t.ChatID != 0 && r.tasks[i].ChatID != t.ChatID {
			continue
		}

		r.tasks[i] = t
		return nil
	}

	return task.ErrNotFound
}

// Get возвращает копию задачи.
func (r *Repo) Get(_ context.Context, chatID int64, id int) (task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range r.tasks {
		if t.ID != id {
			continue
		}
		if chatID != 0 && t.ChatID != chatID {
			continue
		}

		return t, nil
	}

	return task.Task{}, task.ErrNotFound
}

// List возвращает задачи по фильтру.
func (r *Repo) List(_ context.Context, f task.Filter) ([]task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		if f.ChatID != 0 && t.ChatID != f.ChatID {
			continue
		}
		if f.OnlyOpen && !t.IsOpen() {
			continue
		}
		if f.AssigneeID != nil && !t.AssignedTo(*f.AssigneeID) {
			continue
		}
		if f.CreatorID != nil && t.CreatorID != *f.CreatorID {
			continue
		}

		out = append(out, t)
	}

	return out, nil
}
