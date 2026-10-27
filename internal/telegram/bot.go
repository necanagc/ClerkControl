package telegram

import (
	"context"
	"log/slog"
	"strings"
	"time"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
)

// Sender — минимальная часть BotAPI, нужная боту.
type Sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// Bot обрабатывает апдейты и не знает, откуда они пришли.
type Bot struct {
	api   Sender
	svc   *task.Service
	self  string
	log   *slog.Logger
	dedup *dedup
}

// New собирает обработчик. self — username бота, нужен для /command@bot.
func New(api Sender, svc *task.Service, self string, log *slog.Logger) *Bot {
	return &Bot{
		api:   api,
		svc:   svc,
		self:  self,
		log:   log,
		dedup: newDedup(24*time.Hour, 1000),
	}
}

// Run читает апдейты из источника, пока не отменят контекст.
func Run(ctx context.Context, src Source, b *Bot) error {
	updates, err := src.Updates(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if !b.dedup.first(update.UpdateID) {
				b.log.Debug("duplicate update skipped", "update_id", update.UpdateID)
				continue
			}
			if err := b.handle(ctx, update); err != nil {
				b.log.Error("handle update", "update_id", update.UpdateID, "err", err)
			}
		}
	}
}

// handle разбирает сообщение и вызывает нужную команду.
func (b *Bot) handle(ctx context.Context, update tgbotapi.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat == nil {
		return nil
	}

	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return nil
	}

	head, rest := text, ""
	if i := strings.IndexAny(head, " \t"); i >= 0 {
		head, rest = head[:i], strings.TrimSpace(head[i:])
	}
	if i := strings.IndexByte(head, '@'); i >= 0 {
		// /tasks@otherbot — команда не нам
		if b.self != "" && !strings.EqualFold(head[i+1:], b.self) {
			return nil
		}
		head = head[:i]
	}

	chatID := msg.Chat.ID
	userID := msg.From.ID
	username := msg.From.UserName

	switch {
	case head == "/start" || head == "/help":
		b.send(chatID, helpText())
	case head == "/tasks":
		b.cmdTasks(ctx, chatID, userID)
	case head == "/my":
		b.cmdTasksMine(ctx, chatID, userID)
	case head == "/owner":
		b.cmdTasksOwner(ctx, chatID, userID)
	case head == "/new":
		b.cmdNew(ctx, chatID, userID, username, rest)
	case strings.HasPrefix(head, "/assign_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/assign_"))
		if !ok {
			b.send(chatID, "Неверный ID задачи")
			return nil
		}
		b.cmdAssign(ctx, chatID, userID, username, id)
	case strings.HasPrefix(head, "/unassign_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/unassign_"))
		if !ok {
			b.send(chatID, "Неверный ID задачи")
			return nil
		}
		b.cmdUnassign(ctx, chatID, userID, id)
	case strings.HasPrefix(head, "/resolve_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/resolve_"))
		if !ok {
			b.send(chatID, "Неверный ID задачи")
			return nil
		}
		b.cmdResolve(ctx, chatID, userID, username, id)
	default:
		b.log.Debug("unknown command", "cmd", head, "chat_id", chatID)
		b.send(chatID, "Неизвестная команда. Список команд: /help")
	}

	return nil
}

// send отправляет текст и не считает ошибку доставки фатальной.
func (b *Bot) send(chatID int64, text string) {
	if _, err := b.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		b.log.Error("send message", "chat_id", chatID, "err", err)
	}
}
