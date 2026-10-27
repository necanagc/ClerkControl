// Package notify превращает события сервиса в сообщения Telegram.
package notify

import (
	"context"
	"fmt"
	"log/slog"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
)

// Sender — минимальная часть BotAPI, нужная уведомлениям.
type Sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// Notifier реализует task.Sink.
type Notifier struct {
	api Sender
	log *slog.Logger
}

// New создаёт нотификатор.
func New(api Sender, log *slog.Logger) *Notifier {
	return &Notifier{api: api, log: log}
}

// Assigned повторяет прежнюю рассылку: прежнему исполнителю и автору.
func (n *Notifier) Assigned(ctx context.Context, t task.Task, actorID int64, actorName string, previous *int64) {
	if previous != nil && *previous != actorID {
		n.send(*previous, fmt.Sprintf(`Задача "%s" назначена на @%s`, t.Title, actorName))
	}
	if previous == nil && t.CreatorID != actorID {
		n.send(t.CreatorID, fmt.Sprintf(`Задача "%s" назначена на @%s`, t.Title, actorName))
	}
}

// Unassigned сообщает автору, что задача осталась без исполнителя.
func (n *Notifier) Unassigned(ctx context.Context, t task.Task, actorID int64) {
	if t.CreatorID != actorID {
		n.send(t.CreatorID, fmt.Sprintf(`Задача "%s" осталась без исполнителя`, t.Title))
	}
}

// Completed сообщает автору о выполнении.
func (n *Notifier) Completed(ctx context.Context, t task.Task, actorID int64, actorName string) {
	if t.CreatorID != actorID {
		n.send(t.CreatorID, fmt.Sprintf(`Задача "%s" выполнена @%s`, t.Title, actorName))
	}
}

// send не возвращает ошибку: уведомление не должно ломать основной сценарий.
func (n *Notifier) send(chatID int64, text string) {
	if _, err := n.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		n.log.Error("notify: send failed", "chat_id", chatID, "err", err)
	}
}
