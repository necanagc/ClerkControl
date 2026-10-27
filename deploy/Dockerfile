# syntax=docker/dockerfile:1

# --- сборка ---
FROM golang:1.27-alpine AS build

WORKDIR /src

# Зависимости копируются отдельно: пока go.mod и go.sum не менялись,
# этот слой берётся из кеша и модули не скачиваются заново.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 обязателен: статический бинарник нужен для distroless/scratch.
# -trimpath убирает из бинарника пути сборки, -s -w уменьшают размер.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/bot ./cmd/bot

# --- запуск ---
# distroless: нет shell и пакетного менеджера, но есть CA-сертификаты для HTTPS к Telegram.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/bot /bot

USER nonroot:nonroot

ENTRYPOINT ["/bot"]
