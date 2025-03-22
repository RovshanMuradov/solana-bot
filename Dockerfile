FROM golang:1.23.2-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod download
# Компилируем приложение в корень контейнера
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot/main.go

FROM alpine:latest

# Копируем ТОЛЬКО бинарный файл
COPY --from=builder /bot /bot
# Устанавливаем права на выполнение
RUN chmod +x /bot

# НЕ копируем configs, они монтируются через docker-compose
# COPY configs/ /configs/

ENTRYPOINT ["/bot"]