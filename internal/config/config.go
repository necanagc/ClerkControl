// Package config собирает настройки из окружения и файла .env.
package config

import (
	"errors"
	"os"
	"strings"
)

// Config — настройки процесса.
type Config struct {
	Token       string
	DatabaseURL string // пусто — задачи хранятся в памяти
	Debug       bool
}

// Load читает .env (если есть), затем окружение и проверяет результат.
// Переменные окружения имеют приоритет над файлом.
func Load() (Config, error) {
	loadDotEnv(".env")

	cfg := Config{
		Token:       strings.TrimSpace(os.Getenv("TG_TOKEN")),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Debug:       os.Getenv("DEBUG") == "1",
	}

	if cfg.Token == "" {
		return Config{}, errors.New("config: TG_TOKEN не задан")
	}

	return cfg, nil
}

// loadDotEnv повторяет поведение старого loadEnv: существующее окружение не перетирается.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
