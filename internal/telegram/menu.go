package telegram

import (
	"log/slog"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

// Requester — минимальная часть BotAPI, нужная для регистрации меню команд.
type Requester interface {
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// SetCommands регистрирует меню команд в клиенте Telegram.
// Ошибка не фатальна: без меню бот работает, просто команды не подсказываются.
func SetCommands(api Requester, log *slog.Logger) {
	menu := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "new", Description: "Создать задачу"},
		tgbotapi.BotCommand{Command: "tasks", Description: "Открытые задачи чата"},
		tgbotapi.BotCommand{Command: "my", Description: "Мои задачи"},
		tgbotapi.BotCommand{Command: "owner", Description: "Созданные мной"},
		tgbotapi.BotCommand{Command: "help", Description: "Справка"},
	)

	if _, err := api.Request(menu); err != nil {
		log.Warn("setMyCommands", "err", err)
	}
}
