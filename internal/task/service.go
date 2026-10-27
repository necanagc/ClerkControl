package task

import (
	"context"
	"strings"
)

// Sink получает события об изменениях задач. Реализация решает, кому и куда писать.
type Sink interface {
	Assigned(ctx context.Context, t Task, actorID int64, actorName string, previous *int64)
	Unassigned(ctx context.Context, t Task, actorID int64)
	Completed(ctx context.Context, t Task, actorID int64, actorName string)
}

// Service — сценарии работы с задачами.
type Service struct {
	repo Repository
	sink Sink
}

// NewService собирает сервис. sink может быть nil — тогда уведомлений нет.
func NewService(repo Repository, sink Sink) *Service {
	return &Service{repo: repo, sink: sink}
}

// CreateInput — данные для создания задачи.
type CreateInput struct {
	ChatID    int64
	Title     string
	CreatorID int64
	Creator   string
}

// Create создаёт задачу.
func (s *Service) Create(ctx context.Context, in CreateInput) (Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}

	return s.repo.Create(ctx, Task{
		ChatID:    in.ChatID,
		Title:     title,
		CreatorID: in.CreatorID,
		Creator:   in.Creator,
	})
}

// Get возвращает задачу чата.
func (s *Service) Get(ctx context.Context, chatID int64, id int) (Task, error) {
	return s.repo.Get(ctx, chatID, id)
}

// List возвращает задачи по фильтру.
func (s *Service) List(ctx context.Context, f Filter) ([]Task, error) {
	return s.repo.List(ctx, f)
}

// Assign назначает задачу на самого пользователя.
func (s *Service) Assign(ctx context.Context, chatID int64, id int, userID int64, username string) (Task, error) {
	return s.AssignTo(ctx, chatID, id, userID, username, userID, username)
}

// AssignTo назначает задачу на другого пользователя.
// targetID/targetName — на кого назначают, actorID/actorName — кто это сделал:
// различие нужно, чтобы уведомления ушли правильным людям.
func (s *Service) AssignTo(ctx context.Context, chatID int64, id int, targetID int64, targetName string, actorID int64, actorName string) (Task, error) {
	t, err := s.openTask(ctx, chatID, id)
	if err != nil {
		return Task{}, err
	}

	previous := t.AssigneeID
	assignee := targetID
	t.AssigneeID = &assignee
	t.Assignee = targetName

	if err := s.repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	if s.sink != nil {
		s.sink.Assigned(ctx, t, actorID, actorName, previous)
	}

	return t, nil
}

// Unassign снимает задачу с текущего исполнителя.
func (s *Service) Unassign(ctx context.Context, chatID int64, id int, userID int64) (Task, error) {
	t, err := s.openTask(ctx, chatID, id)
	if err != nil {
		return Task{}, err
	}
	if !t.AssignedTo(userID) {
		return Task{}, ErrForbidden
	}

	t.AssigneeID = nil
	t.Assignee = ""

	if err := s.repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	if s.sink != nil {
		s.sink.Unassigned(ctx, t, userID)
	}

	return t, nil
}

// Complete закрывает задачу.
func (s *Service) Complete(ctx context.Context, chatID int64, id int, userID int64, username string) (Task, error) {
	t, err := s.openTask(ctx, chatID, id)
	if err != nil {
		return Task{}, err
	}
	if !t.AssignedTo(userID) {
		return Task{}, ErrForbidden
	}

	t.Done = true

	if err := s.repo.Update(ctx, t); err != nil {
		return Task{}, err
	}
	if s.sink != nil {
		s.sink.Completed(ctx, t, userID, username)
	}

	return t, nil
}

// openTask ищет незакрытую задачу: выполненные считаются ненайденными, как и раньше.
func (s *Service) openTask(ctx context.Context, chatID int64, id int) (Task, error) {
	t, err := s.repo.Get(ctx, chatID, id)
	if err != nil {
		return Task{}, err
	}
	if !t.IsOpen() {
		return Task{}, ErrNotFound
	}

	return t, nil
}
