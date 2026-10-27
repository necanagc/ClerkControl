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
)

// fakeSender собирает отправленные тексты вместо похода в Telegram.
type fakeSender struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, ok := c.(tgbotapi.MessageConfig)
	if ok {
		f.mu.Lock()
		f.sent = append(f.sent, msg.Text)
		f.mu.Unlock()
	}

	return tgbotapi.Message{}, nil
}

func (f *fakeSender) last(t *testing.T) string {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.sent) == 0 {
		t.Fatal("бот ничего не отправил")
	}

	return f.sent[len(f.sent)-1]
}

func newTestBot() (*Bot, *fakeSender) {
	api := &fakeSender{}
	svc := task.NewService(memory.New(), nil)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return New(api, svc, "testbot", log), api
}

func messageUpdate(text string) tgbotapi.Update {
	return tgbotapi.Update{
		UpdateID: 1,
		Message: &tgbotapi.Message{
			Text: text,
			Chat: &tgbotapi.Chat{ID: 100},
			From: &tgbotapi.User{ID: 7, UserName: "oleg"},
		},
	}
}

func TestStartShowsHelp(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/start")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	got := api.last(t)
	for _, want := range []string{"/new", "/tasks", "/help"} {
		if !strings.Contains(got, want) {
			t.Fatalf("в справке нет %q:\n%s", want, got)
		}
	}
}

func TestNewCreatesTaskAndListsIt(t *testing.T) {
	bot, api := newTestBot()
	ctx := context.Background()

	if err := bot.handle(ctx, messageUpdate("/new Починить деплой")); err != nil {
		t.Fatalf("handle new: %v", err)
	}
	if got := api.last(t); !strings.Contains(got, "создана") {
		t.Fatalf("ожидали подтверждение создания: %s", got)
	}

	if err := bot.handle(ctx, messageUpdate("/tasks")); err != nil {
		t.Fatalf("handle tasks: %v", err)
	}
	if got := api.last(t); !strings.Contains(got, "Починить деплой") {
		t.Fatalf("задача не попала в список: %s", got)
	}
}

func TestEmptyNewIsRejected(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/new   ")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := api.last(t); !strings.Contains(got, "непустая") {
		t.Fatalf("ожидали отказ: %s", got)
	}
}

func TestUnknownCommandGetsHint(t *testing.T) {
	bot, api := newTestBot()

	if err := bot.handle(context.Background(), messageUpdate("/нечто")); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := api.last(t); !strings.Contains(got, "/help") {
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
