-- +goose Up
-- Пользователи, которые хотя бы раз взаимодействовали с ботом.
-- Нужны, чтобы превращать @username в id при назначении задачи.
CREATE TABLE users (
    id         BIGINT PRIMARY KEY,
    username   TEXT NOT NULL DEFAULT '',
    first_name TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Поиск по нику регистронезависимый; пользователи без ника не индексируются.
CREATE UNIQUE INDEX users_username_idx ON users (lower(username)) WHERE username <> '';

-- +goose Down
DROP TABLE users;
