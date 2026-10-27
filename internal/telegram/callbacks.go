package telegram

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
)

// handleCallback обрабатывает нажатия на inline-кнопки.
// Каждая ветка обязана ответить на callback, иначе у пользователя крутятся «часики».
func (b *Bot) handleCallback(ctx context.Context, cq tgbotapi.CallbackQuery) {
	if cq.From == nil || cq.Message == nil || cq.Message.Chat == nil {
		b.answerCallback(cq.ID, "")
		return
	}

	chatID := cq.Message.Chat.ID
	messageID := cq.Message.MessageID
	userID := cq.From.ID
	username := cq.From.UserName

	parts := strings.Split(cq.Data, ":")
	if len(parts) == 0 {
		b.answerCallback(cq.ID, "")
		return
	}

	switch parts[0] {
	case "noop":
		b.answerCallback(cq.ID, "")
	case "new":
		b.answerCallback(cq.ID, "")
		b.promptNew(chatID, userID)
	case "hlp":
		b.answerCallback(cq.ID, "")
		b.edit(chatID, messageID, helpText(), backKeyboard())
	case "ls":
		if len(parts) != 3 {
			b.answerCallback(cq.ID, "")
			return
		}
		b.answerCallback(cq.ID, "")
		b.showList(ctx, chatID, messageID, userID, parts[1], atoiOr(parts[2], 0))
	case "asg", "uns", "don":
		if len(parts) != 4 {
			b.answerCallback(cq.ID, "")
			return
		}
		filter, page := parts[2], atoiOr(parts[3], 0)
		b.taskAction(ctx, cq.ID, chatID, messageID, userID, username, parts[0], atoiOr(parts[1], 0), filter, page)
	default:
		b.answerCallback(cq.ID, "")
	}
}

// taskAction выполняет действие над задачей и перерисовывает список.
func (b *Bot) taskAction(ctx context.Context, callbackID string, chatID int64, messageID int, userID int64, username, action string, id int, filter string, page int) {
	var err error

	switch action {
	case "asg":
		_, err = b.svc.Assign(ctx, chatID, id, userID, username)
	case "uns":
		_, err = b.svc.Unassign(ctx, chatID, id, userID)
	case "don":
		_, err = b.svc.Complete(ctx, chatID, id, userID, username)
	}

	switch {
	case err == nil:
		b.answerCallback(callbackID, "Готово")
	case errors.Is(err, task.ErrNotFound):
		b.answerCallback(callbackID, "Задача уже недоступна")
	case errors.Is(err, task.ErrForbidden):
		b.answerCallback(callbackID, "Эта задача не на вас")
	default:
		b.log.Error("task action failed", "action", action, "task_id", id, "err", err)
		b.answerCallback(callbackID, "Не получилось, попробуйте ещё раз")
	}

	// Перерисовываем список в любом случае: он мог устареть.
	b.showList(ctx, chatID, messageID, userID, filter, page)
}

// showList рисует список задач. messageID == 0 — отправить новое сообщение,
// иначе отредактировать существующее (кнопки не плодят сообщения).
func (b *Bot) showList(ctx context.Context, chatID int64, messageID int, userID int64, filter string, page int) {
	tasks, err := b.svc.List(ctx, listFilter(chatID, userID, filter))
	if err != nil {
		b.log.Error("list tasks", "chat_id", chatID, "err", err)
		b.send(chatID, "😔 Не удалось получить задачи, попробуйте позже")
		return
	}

	pages := (len(tasks) + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}

	visible := tasks[page*pageSize:]
	if len(visible) > pageSize {
		visible = visible[:pageSize]
	}

	text := renderList(visible, userID, filter, page, pages)

	// Пустой список: кнопки действий не нужны, вместо них — меню.
	keyboard := mainMenu()
	if len(visible) > 0 {
		keyboard = listKeyboard(visible, userID, filter, page, pages)
	}

	if messageID == 0 {
		b.sendWithKeyboard(chatID, text, keyboard)
		return
	}

	b.edit(chatID, messageID, text, keyboard)
}

// listFilter переводит код фильтра из callback_data в задачу выборки.
func listFilter(chatID, userID int64, filter string) task.Filter {
	f := task.Filter{ChatID: chatID, OnlyOpen: true}

	switch filter {
	case filterMine:
		f.AssigneeID = &userID
	case filterOwner:
		f.CreatorID = &userID
	}

	return f
}

func atoiOr(raw string, fallback int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return n
}
