package telegram

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/storage/memory"
	"taskbot/internal/task"
	"taskbot/internal/user"
	usermemory "taskbot/internal/user/memory"
)

// fakeAPI записывает всё, что бот попытался отправить, вместо похода в Telegram.
type fakeAPI struct {
	mu      sync.Mutex
	nextID  int
	sent    []tgbotapi.MessageConfig
	edits   []tgbotapi.EditMessageTextConfig
	answers []tgbotapi.CallbackConfig
}

func (f *fakeAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, ok := c.(tgbotapi.MessageConfig)
	if !ok {
		return tgbotapi.Message{}, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.sent = append(f.sent, msg)
	f.nextID++

	return tgbotapi.Message{MessageID: f.nextID}, nil
}

func (f *fakeAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch v := c.(type) {
	case tgbotapi.EditMessageTextConfig:
		f.edits = append(f.edits, v)
	case tgbotapi.CallbackConfig:
		f.answers = append(f.answers, v)
	}

	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (f *fakeAPI) lastSent(t *testing.T) tgbotapi.MessageConfig {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.sent) == 0 {
		t.Fatal("бот ничего не отправил")
	}

	return f.sent[len(f.sent)-1]
}

func (f *fakeAPI) lastEdit(t *testing.T) tgbotapi.EditMessageTextConfig {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.edits) == 0 {
		t.Fatal("бот ничего не отредактировал")
	}

	return f.edits[len(f.edits)-1]
}

func (f *fakeAPI) lastAnswer(t *testing.T) tgbotapi.CallbackConfig {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.answers) == 0 {
		t.Fatal("бот не ответил на callback")
	}

	return f.answers[len(f.answers)-1]
}

func newTestBot() (*Bot, *fakeAPI) {
	api := &fakeAPI{}
	svc := task.NewService(memory.New(), nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(api, svc, usermemory.New(), "testbot", log), api
}

func messageUpdate(text string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			Text: text,
			Chat: &tgbotapi.Chat{ID: 100},
			From: &tgbotapi.User{ID: 7, UserName: "oleg", FirstName: "Олег"},
		},
	}
}

// replyUpdate — сообщение пользователя, отправленное ответом на сообщение бота.
func replyUpdate(text string, replyTo int) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 3,
		Message: &tgbotapi.Message{
			Text:           text,
			Chat:           &tgbotapi.Chat{ID: 100},
			From:           &tgbotapi.User{ID: 7, UserName: "oleg", FirstName: "Олег"},
			ReplyToMessage: &tgbotapi.Message{MessageID: replyTo},
		},
	}
}

func callbackUpdate(data string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 2,
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb-1",
			From: &tgbotapi.User{ID: 7, UserName: "oleg", FirstName: "Олег"},
			Message: &tgbotapi.Message{
				MessageID: 55,
				Chat:      &tgbotapi.Chat{ID: 100},
			},
			Data: data,
		},
	}
}

// keyboardData собирает callback_data всех кнопок сообщения.
func keyboardData(t *testing.T, msg tgbotapi.MessageConfig) []string {
	t.Helper()

	markup, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("нет inline-клавиатуры: %T", msg.ReplyMarkup)
	}

	data := make([]string, 0, 8)
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData != nil {
				data = append(data, *button.CallbackData)
			}
		}
	}

	return data
}

func TestStartShowsWelcomeAndMenu(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/start")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	msg := api.lastSent(t)
	if !strings.Contains(msg.Text, "Привет") {
		t.Fatalf("нет приветствия: %s", msg.Text)
	}

	data := strings.Join(keyboardData(t, msg), ",")
	for _, want := range []string{"ls:m:0", "ls:a:0", "hlp"} {
		if !strings.Contains(data, want) {
			t.Fatalf("в меню нет кнопки %q: %s", want, data)
		}
	}
}

func TestNewCreatesTaskWithButtons(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	msg := api.lastSent(t)
	if !strings.Contains(msg.Text, "создана") {
		t.Fatalf("ожидали подтверждение создания: %s", msg.Text)
	}

	data := strings.Join(keyboardData(t, msg), ",")
	if !strings.Contains(data, "asg:1:a:0") {
		t.Fatalf("нет кнопки «взять»: %s", data)
	}
}

func TestEmptyNewIsRejected(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/new   ")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "Нужен текст") {
		t.Fatalf("ожидали подсказку: %s", got)
	}
}

func TestUnknownCommandGetsHint(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/нечто")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "/help") {
		t.Fatalf("ожидали подсказку про /help: %s", got)
	}
}

func TestCommandForAnotherBotIsIgnored(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/tasks@otherbot")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.sent) != 0 {
		t.Fatalf("на чужую команду отвечать не должны: %v", api.sent)
	}
}

func TestPlainTextIsIgnored(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("привет")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.sent) != 0 {
		t.Fatalf("на обычный текст отвечать не должны: %v", api.sent)
	}
}

func TestCallbackAssignsTaskAndRefreshesList(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := bot.handle(ctx, callbackUpdate("asg:1:a:0")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	if answer := api.lastAnswer(t); answer.Text != "Готово" {
		t.Fatalf("ожидали подтверждение, получили %q", answer.Text)
	}

	edit := api.lastEdit(t)
	if !strings.Contains(edit.Text, "🙋 вы") {
		t.Fatalf("задача не назначена на пользователя: %s", edit.Text)
	}

	if edit.ReplyMarkup == nil {
		t.Fatal("у списка нет клавиатуры")
	}

	var hasDone, hasUnassign bool
	for _, row := range edit.ReplyMarkup.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == nil {
				continue
			}
			if strings.HasPrefix(*button.CallbackData, "don:1") {
				hasDone = true
			}
			if strings.HasPrefix(*button.CallbackData, "uns:1") {
				hasUnassign = true
			}
		}
	}
	if !hasDone || !hasUnassign {
		t.Fatalf("после назначения ожидали кнопки «Готово» и «Снять»: %v", edit.ReplyMarkup.InlineKeyboard)
	}
}

func TestCallbackListMine(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := bot.handle(ctx, callbackUpdate("asg:1:a:0")); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := bot.handle(ctx, callbackUpdate("ls:m:0")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	if got := api.lastEdit(t).Text; !strings.Contains(got, "Мои задачи") || !strings.Contains(got, "Починить деплой") {
		t.Fatalf("ожидали задачу в «Мои задачи»: %s", got)
	}
}

func TestCallbackHelp(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), callbackUpdate("hlp")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	if got := api.lastEdit(t).Text; !strings.Contains(got, "/new") {
		t.Fatalf("ожидали справку: %s", got)
	}
}

func TestUnknownCallbackIsAnswered(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), callbackUpdate("weird")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	// Главное — не оставить «часики»: ответ обязателен, текст может быть пустым.
	_ = api.lastAnswer(t)
}

func TestAllowedUpdatesIncludesCallbacks(t *testing.T) {
	// Регрессия: без callback_query нажатия кнопок вообще не доставляются.
	var hasCallbacks bool
	for _, kind := range allowedUpdates {
		if kind == "callback_query" {
			hasCallbacks = true
		}
	}

	if !hasCallbacks {
		t.Fatalf("в allowed_updates нет callback_query: %v", allowedUpdates)
	}
}

func TestNewTaskButtonAsksForTitle(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), callbackUpdate("new")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	msg := api.lastSent(t)
	if !strings.Contains(msg.Text, "название") {
		t.Fatalf("ожидали просьбу прислать название: %s", msg.Text)
	}

	reply, ok := msg.ReplyMarkup.(tgbotapi.ForceReply)
	if !ok || !reply.ForceReply {
		t.Fatalf("ожидали ForceReply: %T", msg.ReplyMarkup)
	}
}

func TestReplyToPromptCreatesTask(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, callbackUpdate("new")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	api.mu.Lock()
	promptID := api.nextID
	api.mu.Unlock()

	if err := bot.handle(ctx, replyUpdate("Купить хлеб", promptID)); err != nil {
		t.Fatalf("reply: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "создана") || !strings.Contains(got, "Купить хлеб") {
		t.Fatalf("задача не создана: %s", got)
	}
}

func TestReplyToOtherMessageIsIgnored(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), callbackUpdate("new")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	api.mu.Lock()
	sentBefore := len(api.sent)
	api.mu.Unlock()

	// Ответ на постороннее сообщение не должен создавать задачу.
	if err := bot.handle(context.Background(), replyUpdate("просто текст", 9999)); err != nil {
		t.Fatalf("reply: %v", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.sent) != sentBefore {
		t.Fatalf("на посторонний ответ бот ответил: %v", api.sent[sentBefore:])
	}
}

func TestCancelClearsPending(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, callbackUpdate("new")); err != nil {
		t.Fatalf("callback: %v", err)
	}

	api.mu.Lock()
	promptID := api.nextID
	api.mu.Unlock()

	if err := bot.handle(ctx, messageUpdate("/cancel")); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	api.mu.Lock()
	sentBefore := len(api.sent)
	api.mu.Unlock()

	if err := bot.handle(ctx, replyUpdate("Купить хлеб", promptID)); err != nil {
		t.Fatalf("reply: %v", err)
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.sent) != sentBefore {
		t.Fatalf("после /cancel задача создаваться не должна")
	}
}

// replyToUserUpdate — команда, отправленная ответом на сообщение другого пользователя.
func replyToUserUpdate(text string, targetID int64, targetUsername string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 4,
		Message: &tgbotapi.Message{
			Text: text,
			Chat: &tgbotapi.Chat{ID: 100},
			From: &tgbotapi.User{ID: 7, UserName: "oleg", FirstName: "Олег"},
			ReplyToMessage: &tgbotapi.Message{
				From: &tgbotapi.User{ID: targetID, UserName: targetUsername, FirstName: "Иван"},
			},
		},
	}
}

func TestAssignByUsername(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.users.Upsert(ctx, user.User{ID: 9, Username: "ivan"}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := bot.handle(ctx, messageUpdate("/assign_1 @ivan")); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "@ivan") {
		t.Fatalf("ожидали назначение на @ivan: %s", got)
	}
}

func TestAssignByReply(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := bot.handle(ctx, replyToUserUpdate("/assign_1", 9, "ivan")); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "@ivan") {
		t.Fatalf("ожидали назначение на @ivan: %s", got)
	}
}

func TestAssignToUnknownUsername(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := bot.handle(ctx, messageUpdate("/assign_1 @unknown")); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "Не знаю пользователя") {
		t.Fatalf("ожидали подсказку про неизвестного: %s", got)
	}
}

func TestAssignWithoutTargetAssignsSelf(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := bot.handle(ctx, messageUpdate("/assign_1")); err != nil {
		t.Fatalf("assign: %v", err)
	}

	if got := api.lastSent(t).Text; !strings.Contains(got, "на вас") {
		t.Fatalf("ожидали назначение на себя: %s", got)
	}
}
