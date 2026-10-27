package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"taskbot/internal/task"
)

// testRepo подключается к базе из TEST_DATABASE_URL и очищает таблицу.
// Без переменной тест пропускается: юнит-тесты не должны требовать инфраструктуру.
func testRepo(t *testing.T) *Repo {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}

	ctx := context.Background()

	if err := Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(repo.Close)

	// TRUNCATE ... RESTART IDENTITY — каждый тест начинает с id = 1.
	const truncate = `TRUNCATE tasks RESTART IDENTITY`
	if _, err := repo.pool.Exec(ctx, truncate); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	return repo
}

func newTask(chatID int64, title string) task.Task {
	return task.Task{
		ChatID:    chatID,
		Title:     title,
		CreatorID: 7,
		Creator:   "oleg",
	}
}

func TestCreateAndGet(t *testing.T) {
	repo := testRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, newTask(100, "первая"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("create не вернул ID")
	}

	got, err := repo.Get(ctx, 100, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "первая" || got.ChatID != 100 {
		t.Fatalf("несовпадение полей: %+v", got)
	}

	if _, err := repo.Get(ctx, 999, created.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound для чужого чата, получили %v", err)
	}
	if _, err := repo.Get(ctx, 100, 424242); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound для несуществующего ID, получили %v", err)
	}
}

func TestUpdate(t *testing.T) {
	repo := testRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, newTask(100, "до"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	assignee := int64(42)
	created.AssigneeID = &assignee
	created.Assignee = "ivan"
	created.Done = true

	if err := repo.Update(ctx, created); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.Get(ctx, 100, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Done || !got.AssignedTo(42) || got.Assignee != "ivan" {
		t.Fatalf("изменения не сохранились: %+v", got)
	}

	missing := created
	missing.ID = 999999
	if err := repo.Update(ctx, missing); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("ожидали ErrNotFound, получили %v", err)
	}
}

func TestListFilters(t *testing.T) {
	repo := testRepo(t)
	ctx := context.Background()

	first, err := repo.Create(ctx, newTask(100, "в чате 100"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, newTask(100, "вторая в 100")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.Create(ctx, newTask(200, "в чате 200")); err != nil {
		t.Fatalf("create: %v", err)
	}

	all, err := repo.List(ctx, task.Filter{ChatID: 100, OnlyOpen: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ожидали 2 задачи чата 100, получили %d", len(all))
	}

	assignee := int64(7)
	first.AssigneeID = &assignee
	first.Assignee = "oleg"
	if err := repo.Update(ctx, first); err != nil {
		t.Fatalf("update: %v", err)
	}

	mine, err := repo.List(ctx, task.Filter{ChatID: 100, AssigneeID: &assignee, OnlyOpen: true})
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != first.ID {
		t.Fatalf("фильтр по исполнителю не сработал: %+v", mine)
	}

	creator := int64(7)
	owned, err := repo.List(ctx, task.Filter{ChatID: 100, CreatorID: &creator})
	if err != nil {
		t.Fatalf("list owner: %v", err)
	}
	if len(owned) != 2 {
		t.Fatalf("фильтр по автору: ожидали 2, получили %d", len(owned))
	}

	first.Done = true
	if err := repo.Update(ctx, first); err != nil {
		t.Fatalf("update done: %v", err)
	}

	open, err := repo.List(ctx, task.Filter{ChatID: 100, OnlyOpen: true})
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("закрытая задача не должна попадать в открытые: %d", len(open))
	}
}

func TestMigrateIdempotent(t *testing.T) {
	repo := testRepo(t)
	ctx := context.Background()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if err := Migrate(ctx, dsn); err != nil {
		t.Fatalf("повторная миграция: %v", err)
	}

	if _, err := repo.List(ctx, task.Filter{}); err != nil {
		t.Fatalf("list после повторной миграции: %v", err)
	}
}
