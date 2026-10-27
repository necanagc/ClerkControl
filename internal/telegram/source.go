package telegram

import (
	"context"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

// Source — любой способ получать апдейты от Telegram.
// Ниже уровня Source код не знает, polling это или webhook.
type Source interface {
	Updates(ctx context.Context) (<-chan tgbotapi.Update, error)
	Close() error
}

// allowedUpdates ограничивает поток только нужными типами.
var allowedUpdates = []string{"message"}
