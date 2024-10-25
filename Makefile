# Makefile
include .env
export

.PHONY: clean-db postgres docker migrate-up

# Запуск PostgreSQL
postgres:
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 10

# Остановка всех контейнеров и очистка данных
clean-db:
	docker-compose down -v
	docker volume rm -f solana-bot_postgres_data
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 10

# Применение миграций через GORM
migrate-up:
	docker-compose run --rm app ./bot

# Остановка всех контейнеров
docker-down:
	docker-compose down