package telegram

import (
	"context"
	"fmt"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

// PollingSource забирает апдейты через getUpdates.
type PollingSource struct {
	api *tgbotapi.BotAPI
}

// NewPollingSource создаёт источник long polling.
func NewPollingSource(api *tgbotapi.BotAPI) *PollingSource {
	return &PollingSource{api: api}
}

// Updates снимает возможный webhook и запускает получение апдейтов.
func (s *PollingSource) Updates(_ context.Context) (<-chan tgbotapi.Update, error) {
	// Пока установлен webhook, getUpdates вернёт 409 Conflict.
	if _, err := s.api.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false}); err != nil {
		return nil, fmt.Errorf("polling: deleteWebhook: %w", err)
	}

	return s.api.GetUpdatesChan(tgbotapi.UpdateConfig{
		Timeout:        50,
		AllowedUpdates: allowedUpdates,
	}), nil
}

// Close останавливает приём апдейтов.
func (s *PollingSource) Close() error {
	s.api.StopReceivingUpdates()
	return nil
}
