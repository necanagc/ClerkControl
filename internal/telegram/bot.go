package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
	"taskbot/internal/user"
)

// API — минимальная часть BotAPI, нужная боту:
// отправка сообщений, правка сообщений и ответы на нажатия кнопок.
type API interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// Bot обрабатывает апдейты и не знает, откуда они пришли.
type Bot struct {
	api   API
	svc   *task.Service
	users user.Repository
	self  string
	log   *slog.Logger
	dedup *dedup

	mu      sync.Mutex
	pending map[int64]pendingPrompt // chatID -> приглашение, ждущее названия
}

// New собирает обработчик. self — username бота, нужен для /command@bot.
func New(api API, svc *task.Service, users user.Repository, self string, log *slog.Logger) *Bot {
	return &Bot{
		api:     api,
		svc:     svc,
		users:   users,
		self:    self,
		log:     log,
		dedup:   newDedup(24*time.Hour, 1000),
		pending: make(map[int64]pendingPrompt),
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

// handle разбирает апдейт: нажатие кнопки или текстовую команду.
func (b *Bot) handle(ctx context.Context, update tgbotapi.Update) error {
	if update.CallbackQuery != nil {
		b.handleCallback(ctx, *update.CallbackQuery)
		return nil
	}

	msg := update.Message
	if msg == nil || msg.From == nil || msg.Chat == nil {
		return nil
	}

	b.rememberUser(ctx, msg.From)

	// Ответ на приглашение «напишите название» создаёт задачу.
	if msg.ReplyToMessage != nil && b.consumePending(msg.Chat.ID, msg.ReplyToMessage.MessageID, msg.From.ID) {
		b.createTask(ctx, msg.Chat.ID, msg.From.ID, msg.From.UserName, msg.Text)
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
	case head == "/start":
		b.cmdStart(chatID, displayName(msg.From))
	case head == "/help":
		b.cmdHelp(chatID)
	case head == "/tasks":
		b.cmdTasks(ctx, chatID, userID)
	case head == "/my":
		b.cmdTasksMine(ctx, chatID, userID)
	case head == "/owner":
		b.cmdTasksOwner(ctx, chatID, userID)
	case head == "/new":
		b.cmdNew(ctx, chatID, userID, username, rest)
	case head == "/cancel":
		b.clearPending(chatID)
		b.send(chatID, "Ок, отменил — ничего не создано.")
	case strings.HasPrefix(head, "/assign_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/assign_"))
		if !ok {
			b.send(chatID, "🤔 Не понял номер задачи. Пример: /assign_5")
			return nil
		}
		reply, ref := assignTarget(msg, rest)
		b.cmdAssign(ctx, chatID, userID, username, id, reply, ref)
	case strings.HasPrefix(head, "/unassign_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/unassign_"))
		if !ok {
			b.send(chatID, "🤔 Не понял номер задачи. Пример: /unassign_5")
			return nil
		}
		b.cmdUnassign(ctx, chatID, userID, id)
	case strings.HasPrefix(head, "/resolve_"):
		id, ok := parseTaskID(strings.TrimPrefix(head, "/resolve_"))
		if !ok {
			b.send(chatID, "🤔 Не понял номер задачи. Пример: /resolve_5")
			return nil
		}
		b.cmdResolve(ctx, chatID, userID, username, id)
	default:
		b.log.Debug("unknown command", "cmd", head, "chat_id", chatID)
		b.send(chatID, "🤔 Не знаю такой команды.\nПосмотреть всё: /help")
	}

	return nil
}

// displayName выбирает, как обратиться к пользователю.
func displayName(u *tgbotapi.User) string {
	if u.UserName != "" {
		return "@" + u.UserName
	}

	return u.FirstName
}

// send отправляет текст без клавиатуры.
func (b *Bot) send(chatID int64, text string) {
	if _, err := b.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		b.log.Error("send message", "chat_id", chatID, "err", err)
	}
}

// sendWithKeyboard отправляет текст с inline-кнопками.
func (b *Bot) sendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard

	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("send message", "chat_id", chatID, "err", err)
	}
}

// edit правит существующее сообщение: так кнопки не плодят новые сообщения.
func (b *Bot) edit(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, keyboard)

	if _, err := b.api.Request(edit); err != nil && !isNotModified(err) {
		b.log.Error("edit message", "chat_id", chatID, "message_id", messageID, "err", err)
	}
}

// answerCallback обязателен: без него у пользователя крутятся «часики» на кнопке.
func (b *Bot) answerCallback(callbackID, text string) {
	if _, err := b.api.Request(tgbotapi.NewCallback(callbackID, text)); err != nil {
		b.log.Debug("answer callback", "err", err)
	}
}

// isNotModified — Telegram отвечает ошибкой, если текст и клавиатура не изменились.
func isNotModified(err error) bool {
	var apiErr *tgbotapi.Error

	return errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "message is not modified")
}

// pendingPrompt — приглашение ввести название задачи.
type pendingPrompt struct {
	messageID int
	userID    int64
}

// setPending запоминает сообщение, ответ на которое считается названием задачи.
func (b *Bot) setPending(chatID int64, messageID int, userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pending[chatID] = pendingPrompt{messageID: messageID, userID: userID}
}

// consumePending сообщает, ждали ли мы ответ именно на это сообщение.
// Запись удаляется: одно приглашение — одна задача.
func (b *Bot) consumePending(chatID int64, messageID int, userID int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	want, ok := b.pending[chatID]
	if !ok || want.messageID != messageID || want.userID != userID {
		return false
	}

	delete(b.pending, chatID)

	return true
}

// clearPending сбрасывает приглашение (команда /cancel).
func (b *Bot) clearPending(chatID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.pending, chatID)
}

// rememberUser сохраняет ник и имя: без этого @username нельзя превратить в id.
func (b *Bot) rememberUser(ctx context.Context, u *tgbotapi.User) {
	if u == nil || u.IsBot {
		return
	}

	err := b.users.Upsert(ctx, user.User{ID: u.ID, Username: u.UserName, FirstName: u.FirstName})
	if err != nil {
		b.log.Warn("remember user", "user_id", u.ID, "err", err)
	}
}

// assignTarget решает, на кого назначать: ответ на сообщение важнее @username.
func assignTarget(msg *tgbotapi.Message, rest string) (*tgbotapi.User, string) {
	if reply := msg.ReplyToMessage; reply != nil && reply.From != nil && !reply.From.IsBot {
		return reply.From, ""
	}

	ref := strings.TrimSpace(rest)
	if strings.HasPrefix(ref, "@") {
		return nil, strings.TrimPrefix(ref, "@")
	}

	return nil, ""
}
