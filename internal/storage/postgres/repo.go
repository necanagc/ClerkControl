package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"taskbot/internal/task"
)

// Repo — реализация task.Repository поверх пула соединений pgx.
type Repo struct {
	pool *pgxpool.Pool
}

// Open создаёт пул и проверяет, что база отвечает.
func Open(ctx context.Context, dsn string) (*Repo, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: pool: %w", err)
	}

	if err := waitReady(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return &Repo{pool: pool}, nil
}

// Close закрывает пул.
func (r *Repo) Close() {
	r.pool.Close()
}

const insertTask = `
INSERT INTO tasks (chat_id, title, creator_id, creator, assignee_id, assignee, done)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`

// Create сохраняет задачу и возвращает её с присвоенным ID.
func (r *Repo) Create(ctx context.Context, t task.Task) (task.Task, error) {
	err := r.pool.QueryRow(ctx, insertTask,
		t.ChatID, t.Title, t.CreatorID, t.Creator, t.AssigneeID, t.Assignee, t.Done,
	).Scan(&t.ID)
	if err != nil {
		return task.Task{}, fmt.Errorf("postgres: create: %w", err)
	}

	return t, nil
}

// $6::bigint обязателен: без каста Postgres выводит тип параметра из литерала 0 (int4),
// и идентификаторы групп (-5 000 000 000) перестают помещаться в параметр.
const updateTask = `
UPDATE tasks
SET title = $1, assignee_id = $2, assignee = $3, done = $4
WHERE id = $5 AND ($6::bigint = 0 OR chat_id = $6)`

// Update перезаписывает изменяемые поля задачи.
func (r *Repo) Update(ctx context.Context, t task.Task) error {
	tag, err := r.pool.Exec(ctx, updateTask,
		t.Title, t.AssigneeID, t.Assignee, t.Done, t.ID, t.ChatID,
	)
	if err != nil {
		return fmt.Errorf("postgres: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return task.ErrNotFound
	}

	return nil
}

// $2::bigint — по той же причине, что и в updateTask: ID групп не влезают в int4.
const selectTask = `
SELECT id, chat_id, title, creator_id, creator, assignee_id, assignee, done
FROM tasks
WHERE id = $1 AND ($2::bigint = 0 OR chat_id = $2)`

// Get возвращает задачу по ID в пределах чата (chatID = 0 — любой чат).
func (r *Repo) Get(ctx context.Context, chatID int64, id int) (task.Task, error) {
	var t task.Task
	err := r.pool.QueryRow(ctx, selectTask, id, chatID).Scan(
		&t.ID, &t.ChatID, &t.Title, &t.CreatorID, &t.Creator, &t.AssigneeID, &t.Assignee, &t.Done,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("postgres: get: %w", err)
	}

	return t, nil
}

const selectTasks = `
SELECT id, chat_id, title, creator_id, creator, assignee_id, assignee, done
FROM tasks`

// List возвращает задачи по фильтру. Условия собираются динамически,
// поэтому аргументы нумеруются по мере добавления ($1, $2, ...).
func (r *Repo) List(ctx context.Context, f task.Filter) ([]task.Task, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if f.ChatID != 0 {
		args = append(args, f.ChatID)
		where = append(where, fmt.Sprintf("chat_id = $%d", len(args)))
	}
	if f.OnlyOpen {
		where = append(where, "done = FALSE")
	}
	if f.AssigneeID != nil {
		args = append(args, *f.AssigneeID)
		where = append(where, fmt.Sprintf("assignee_id = $%d", len(args)))
	}
	if f.CreatorID != nil {
		args = append(args, *f.CreatorID)
		where = append(where, fmt.Sprintf("creator_id = $%d", len(args)))
	}

	query := selectTasks
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY id"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list: %w", err)
	}
	defer rows.Close()

	tasks := make([]task.Task, 0, 16)
	for rows.Next() {
		var t task.Task
		if err := rows.Scan(
			&t.ID, &t.ChatID, &t.Title, &t.CreatorID, &t.Creator, &t.AssigneeID, &t.Assignee, &t.Done,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: rows: %w", err)
	}

	return tasks, nil
}
