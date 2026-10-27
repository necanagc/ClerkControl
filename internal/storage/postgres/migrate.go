// Package postgres хранит задачи в PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // драйвер "pgx" для database/sql

	"taskbot/internal/storage/postgres/migrations"
)

// Migrate применяет все неприменённые миграции.
// Использует отдельное соединение database/sql: goose работает со стандартным интерфейсом,
// а рантайм ходит через pgxpool.
func Migrate(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open for migrate: %w", err)
	}
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("postgres: goose provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: migrate: %w", err)
	}

	return nil
}

// pingTimeout ограничивает проверку соединения при старте.
const pingTimeout = 5 * time.Second

// waitReady ждёт доступности базы: в compose она может подниматься дольше приложения.
func waitReady(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	return pool.Ping(ctx)
}
