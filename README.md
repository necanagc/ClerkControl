# TaskManagerBot

[![CI](https://github.com/necanagc/ClerkControl/actions/workflows/ci.yml/badge.svg)](https://github.com/necanagc/ClerkControl/actions/workflows/ci.yml)

Telegram-бот для управления задачами: личный список в личных сообщениях и общий — в группе.
Задачи создаются, назначаются на исполнителя и закрываются прямо в чате, без внешней панели.

## Быстрый старт

Нужны: Go 1.27+, Docker и токен от [@BotFather](https://t.me/BotFather).

```bash
git clone git@github.com:necanagc/ClerkControl.git
cd ClerkControl

make db-up                 # 1. PostgreSQL в Docker (порт только на 127.0.0.1)
cp .env.example .env       # 2. вписать TG_TOKEN в .env
make run                   # 3. миграции + запуск бота
```

Ожидаемые строки в логе:

```
level=INFO msg="storage ready" driver=postgres
level=INFO msg="starting" bot=my_bot mode=polling
```

Дальше в Telegram: `/new Починить деплой`, затем `/tasks`.

Остановить бота — `Ctrl+C`, базу — `make db-down`.

### Проверка, что всё работает

```bash
make db-psql
# внутри psql:
\dt          # должна быть таблица tasks и goose_db_version
\q

curl -s "https://api.telegram.org/bot$TG_TOKEN/getMe" | head -c 200
```

### Если что-то не так

| Симптом | Причина | Что делать |
|---|---|---|
| `config: TG_TOKEN не задан` | нет `.env` или пустой токен | `cp .env.example .env`, вписать токен |
| `postgres: ping` при старте | база не поднялась или занят порт 5432 | `make db-up`, `docker compose ps` |
| `bind: address already in use` при `db-up` | локальный PostgreSQL уже слушает 5432 | поменять порт в `docker-compose.yml` на `5433` и в `DATABASE_URL` |
| нет таблицы `tasks` | том создан старым запуском, `init.sql` не выполнялся | `make db-reset` |
| `Unauthorized` | токен отозван или неверный | перевыпустить в @BotFather |

### Запуск без базы

Если `DATABASE_URL` пуст, бот поднимается на хранилище в памяти: удобно для демонстрации,
но задачи исчезнут при перезапуске. В лог пишется предупреждение.

### Запуск в Docker целиком

```bash
cp .env.example .env       # вписать TG_TOKEN
docker compose --profile full up -d --build
docker compose logs -f bot
```

## Возможности

- создание задачи одной командой: `/new Починить деплой`;
- назначение задачи на себя и снятие с себя;
- закрытие задачи с уведомлением автора;
- три среза списка: все задачи, мои, созданные мной;
- изоляция по чатам: задачи группы не смешиваются с личными;
- уведомления участникам при назначении, снятии и выполнении;
- хранение в PostgreSQL: данные переживают перезапуск;
- автомиграции при старте, встроенные в бинарник;
- защита от повторной обработки одного и того же обновления (`update_id`);
- корректная остановка по `SIGINT`/`SIGTERM`.

## Команды

| Команда | Действие |
|---|---|
| `/new <текст>` | создать задачу |
| `/tasks` | все открытые задачи чата |
| `/my` | задачи, назначенные на меня |
| `/owner` | задачи, которые создал я |
| `/assign_<id>` | назначить задачу на себя |
| `/unassign_<id>` | снять задачу с себя |
| `/resolve_<id>` | отметить задачу выполненной |

В группе команды можно адресовать боту: `/tasks@my_bot`. Команды, адресованные другому боту, игнорируются.

## База данных

PostgreSQL 16 в контейнере, порт опубликован только на `127.0.0.1`.

```bash
make db-up      # поднять
make db-psql    # psql внутри контейнера
make db-reset   # снести том и поднять заново (данные пропадут)
```

Схема:

```sql
tasks(id, chat_id, title, creator_id, creator, assignee_id, assignee, done)
```

Индексы под конкретные запросы:

| Индекс | Для чего |
|---|---|
| `tasks_chat_open_idx (chat_id, id) WHERE NOT done` | `/tasks` — открытые задачи чата |
| `tasks_assignee_idx (chat_id, assignee_id) WHERE assignee_id IS NOT NULL` | `/my` |
| `tasks_creator_idx (chat_id, creator_id)` | `/owner` |

Частичные индексы выбраны осознанно: закрытые задачи не нужны в двух из трёх запросов,
поэтому их можно не хранить в индексе.

**Миграции** лежат в `internal/storage/postgres/migrations` и встраиваются в бинарник через
`embed`, применяются библиотекой `goose` при старте. Драйвер — `pgx/v5` с пулом соединений.
Ограничение времени на миграции — 30 секунд, чтобы бот не висел вечно на недоступной базе.

## Docker

`deploy/Dockerfile` — multi-stage: сборка в `golang:1.27-alpine`, запуск в
`gcr.io/distroless/static-debian12:nonroot`.

- `CGO_ENABLED=0` — статический бинарник, без libc в финальном образе;
- `-trimpath`, `-ldflags="-s -w"` — воспроизводимая сборка и меньший размер;
- non-root пользователь — запуск не от root;
- distroless содержит CA-сертификаты, поэтому HTTPS к Telegram работает.

Приложение не запускается раньше базы: в compose используется
`depends_on: condition: service_healthy` вместе с healthcheck Postgres.

## Архитектура

```mermaid
flowchart LR
    TG[Telegram] -->|getUpdates| SRC[telegram.Source]
    SRC --> BOT[telegram.Bot]
    BOT --> SVC[task.Service]
    SVC --> REPO[(task.Repository)]
    REPO --> PG[(PostgreSQL)]
    SVC -.события.-> NTF[notify.Notifier]
    NTF --> TG
```

Слои и направление зависимостей:

- `telegram` — транспорт: приём обновлений, разбор команд, отправка ответов;
- `task` — домен и сценарии: что можно делать с задачей и какие ошибки при этом бывают;
- `storage` — реализации `task.Repository` (память и PostgreSQL);
- `notify` — реакция на события сервиса (кому и что написать);
- `cmd/bot` — сборка зависимостей и жизненный цикл процесса.

`task` не импортирует ничего из проекта — это и есть граница, ради которой всё разделено.
Любой транспорт (polling, webhook) и любое хранилище (memory, PostgreSQL) подставляются в `main`.

## Структура

```
cmd/bot/                          точка входа, выбор хранилища, сигналы
internal/config/                  настройки из окружения и .env
internal/task/                    модель, ошибки, интерфейс хранилища, сценарии
internal/storage/memory/          хранилище в памяти (для тестов)
internal/storage/postgres/        пул pgx, SQL-репозиторий, миграции
internal/notify/                  уведомления по событиям сервиса
internal/telegram/                источник обновлений, роутинг, рендеринг
deploy/Dockerfile                 сборка образа приложения
deploy/postgres/init.sql          создание тестовой базы при первом запуске
.github/workflows/ci.yml          CI: форматирование, vet, тесты, сборка образа
```

## Решения и почему именно так

**Хранилище за интерфейсом.** `task.Repository` объявлен в домене, реализации — снаружи.
Это позволяет держать быстрые тесты логики на памяти и подключать PostgreSQL без правок сервиса.

**Чат как арендатор.** `Task.ChatID` одновременно решает две задачи: в личке задачи приватны
(там `chat_id` равен `user_id`), в группе — общие. Отдельная сущность «команда» не понадобилась.

**Источник обновлений за интерфейсом.** `telegram.Source` скрывает разницу между long polling и
webhook: обработчики команд не знают, откуда пришло сообщение, поэтому переход на webhook —
это добавление одной реализации, а не переписывание логики.

**Дедупликация `update_id`.** Telegram повторяет доставку, если не получил подтверждение;
повторное выполнение команды не должно создавать вторую задачу или второе уведомление.

**Миграции в бинарнике.** `embed` + `goose`: деплой не требует ни CLI, ни отдельных файлов,
а версия схемы хранится в таблице `goose_db_version`.

## Тесты

```bash
make test       # юнит-тесты, без инфраструктуры
make db-up
make test-db    # интеграционные тесты репозитория на реальном PostgreSQL
```

Интеграционные тесты пропускаются, если не задан `TEST_DATABASE_URL` — поэтому `make test`
работает на чистой машине. В CI база поднимается сервисным контейнером.

## CI

`.github/workflows/ci.yml` на каждый push и pull request: проверка `gofmt`, `go vet`,
`go test -race` (включая интеграционные тесты с Postgres), сборка бинарника и Docker-образа.

## План

- [x] вынести домен, хранилище и транспорт в отдельные пакеты
- [x] long polling, graceful shutdown, дедупликация обновлений
- [x] PostgreSQL: пул соединений, миграции, индексы
- [x] Docker-образ приложения и docker compose целиком
- [x] CI: `go vet`, `go test -race`, интеграционные тесты на сервисном контейнере
- [ ] webhook-режим для продакшена
- [ ] inline-кнопки вместо команд с идентификаторами
