package telegram

import (
	"context"
	"errors"
	"strconv"

	"taskbot/internal/task"
)

// cmdTasks показывает все открытые задачи чата.
func (b *Bot) cmdTasks(ctx context.Context, chatID, userID int64) {
	b.list(ctx, chatID, userID, task.Filter{ChatID: chatID, OnlyOpen: true})
}

// cmdTasksMine показывает задачи, назначенные на пользователя.
func (b *Bot) cmdTasksMine(ctx context.Context, chatID, userID int64) {
	assignee := userID
	b.list(ctx, chatID, userID, task.Filter{ChatID: chatID, AssigneeID: &assignee, OnlyOpen: true})
}

// cmdTasksOwner показывает задачи, созданные пользователем.
func (b *Bot) cmdTasksOwner(ctx context.Context, chatID, userID int64) {
	creator := userID
	b.list(ctx, chatID, userID, task.Filter{ChatID: chatID, CreatorID: &creator, OnlyOpen: true})
}

func (b *Bot) list(ctx context.Context, chatID, viewerID int64, f task.Filter) {
	tasks, err := b.svc.List(ctx, f)
	if err != nil {
		b.log.Error("list tasks", "chat_id", chatID, "err", err)
		b.send(chatID, "Не удалось получить задачи")
		return
	}

	b.send(chatID, renderList(tasks, viewerID))
}

// cmdNew создаёт задачу из остатка сообщения.
func (b *Bot) cmdNew(ctx context.Context, chatID, userID int64, username, title string) {
	t, err := b.svc.Create(ctx, task.CreateInput{
		ChatID:    chatID,
		Title:     title,
		CreatorID: userID,
		Creator:   username,
	})
	if err != nil {
		if errors.Is(err, task.ErrEmptyTitle) {
			b.send(chatID, "Требуется непустая задача")
			return
		}
		b.log.Error("create task", "chat_id", chatID, "err", err)
		b.send(chatID, "Не удалось создать задачу")
		return
	}

	b.send(chatID, taskCreated(t))
}

// cmdAssign назначает задачу на себя.
func (b *Bot) cmdAssign(ctx context.Context, chatID, userID int64, username string, id int) {
	t, err := b.svc.Assign(ctx, chatID, id, userID, username)
	if err != nil {
		b.reportCommandError(chatID, "assign", err, "Задача не на вас")
		return
	}

	b.send(chatID, taskAssigned(t))
}

// cmdUnassign снимает задачу с себя.
func (b *Bot) cmdUnassign(ctx context.Context, chatID, userID int64, id int) {
	if _, err := b.svc.Unassign(ctx, chatID, id, userID); err != nil {
		b.reportCommandError(chatID, "unassign", err, "Задача не на вас")
		return
	}

	b.send(chatID, "Принято")
}

// cmdResolve закрывает задачу.
func (b *Bot) cmdResolve(ctx context.Context, chatID, userID int64, username string, id int) {
	t, err := b.svc.Complete(ctx, chatID, id, userID, username)
	if err != nil {
		b.reportCommandError(chatID, "resolve", err, "Задача назначена не на вас")
		return
	}

	b.send(chatID, taskCompleted(t))
}

// reportCommandError переводит ошибки сервиса в прежние тексты.
func (b *Bot) reportCommandError(chatID int64, op string, err error, forbiddenText string) {
	switch {
	case errors.Is(err, task.ErrNotFound):
		b.send(chatID, "Задача не найдена")
	case errors.Is(err, task.ErrForbidden):
		b.send(chatID, forbiddenText)
	default:
		b.log.Error("command failed", "op", op, "chat_id", chatID, "err", err)
		b.send(chatID, "Не удалось выполнить команду")
	}
}

func parseTaskID(raw string) (int, bool) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}

	return id, true
}
