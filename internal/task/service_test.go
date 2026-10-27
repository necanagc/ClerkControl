package task_test

import (
	"context"
	"errors"
	"testing"

	"taskbot/internal/storage/memory"
	"taskbot/internal/task"
)

// sink запоминает, кто выполнил действие.
type sink struct {
	assigned    int
	lastActorID int64
}

func (s *sink) Assigned(_ context.Context, _ task.Task, actorID int64, _ string, _ *int64) {
	s.assigned++
	s.lastActorID = actorID
}

func (s *sink) Unassigned(context.Context, task.Task, int64)        {}
func (s *sink) Completed(context.Context, task.Task, int64, string) {}

func newService() (*task.Service, *sink) {
	s := &sink{}
	return task.NewService(memory.New(), s), s
}

func TestAssignToOtherKeepsActor(t *testing.T) {
	ctx := context.Background()
	svc, notifications := newService()

	created, err := svc.Create(ctx, task.CreateInput{ChatID: 1, Title: "Починить деплой", CreatorID: 1, Creator: "oleg"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	assigned, err := svc.AssignTo(ctx, 1, created.ID, 9, "ivan", 7, "oleg")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	if !assigned.AssignedTo(9) || assigned.Assignee != "ivan" {
		t.Fatalf("задача не назначена на ivan: %+v", assigned)
	}
	// Уведомления должны знать, что действие сделал oleg, а не ivan.
	if notifications.lastActorID != 7 {
		t.Fatalf("актор передан неверно: %d", notifications.lastActorID)
	}
}

func TestAssignToSelfEqualsActor(t *testing.T) {
	ctx := context.Background()
	svc, notifications := newService()

	created, err := svc.Create(ctx, task.CreateInput{ChatID: 1, Title: "Задача", CreatorID: 1, Creator: "oleg"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	assigned, err := svc.Assign(ctx, 1, created.ID, 7, "oleg")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if !assigned.AssignedTo(7) {
		t.Fatalf("задача не на себе: %+v", assigned)
	}
	if notifications.lastActorID != 7 {
		t.Fatalf("актор передан неверно: %d", notifications.lastActorID)
	}
}

func TestOnlyAssigneeCanComplete(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()

	created, err := svc.Create(ctx, task.CreateInput{ChatID: 1, Title: "Задача", CreatorID: 1, Creator: "oleg"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AssignTo(ctx, 1, created.ID, 9, "ivan", 7, "oleg"); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if _, err := svc.Complete(ctx, 1, created.ID, 7, "oleg"); !errors.Is(err, task.ErrForbidden) {
		t.Fatalf("чужой не должен закрывать задачу: %v", err)
	}
	if _, err := svc.Complete(ctx, 1, created.ID, 9, "ivan"); err != nil {
		t.Fatalf("исполнитель должен закрывать задачу: %v", err)
	}
}

func TestEmptyTitleRejected(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()

	if _, err := svc.Create(ctx, task.CreateInput{ChatID: 1, Title: "   "}); !errors.Is(err, task.ErrEmptyTitle) {
		t.Fatalf("ожидали ErrEmptyTitle, получили %v", err)
	}
}
