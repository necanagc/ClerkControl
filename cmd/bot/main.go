// Команда bot запускает Telegram-бота.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/config"
	"taskbot/internal/notify"
	"taskbot/internal/storage/memory"
	"taskbot/internal/storage/postgres"
	"taskbot/internal/task"
	"taskbot/internal/telegram"
)

const migrateTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("bot stopped with error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg.Debug)

	store, closeStore, err := openStorage(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer closeStore()

	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return err
	}
	api.Debug = cfg.Debug

	telegram.SetCommands(api, log)

	svc := task.NewService(store, notify.New(api, log))
	handler := telegram.New(api, svc, api.Self.UserName, log)
	src := telegram.NewPollingSource(api)

	log.Info("starting", "bot", api.Self.UserName, "mode", "polling")

	runErr := telegram.Run(ctx, src, handler)

	if err := src.Close(); err != nil && !errors.Is(err, context.Canceled) {
		log.Warn("close source", "err", err)
	}

	log.Info("stopped")

	return runErr
}

func newLogger(debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// openStorage выбирает хранилище: PostgreSQL при заданном DATABASE_URL, иначе память.
// Возвращает функцию закрытия, чтобы вызывающий код освободил пул.
func openStorage(ctx context.Context, cfg config.Config, log *slog.Logger) (task.Repository, func(), error) {
	if cfg.DatabaseURL == "" {
		log.Warn("DATABASE_URL не задан: задачи хранятся в памяти и теряются при перезапуске")
		return memory.New(), func() {}, nil
	}

	migrateCtx, cancel := context.WithTimeout(ctx, migrateTimeout)
	defer cancel()

	if err := postgres.Migrate(migrateCtx, cfg.DatabaseURL); err != nil {
		return nil, nil, err
	}

	repo, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	log.Info("storage ready", "driver", "postgres")

	return repo, repo.Close, nil
}
