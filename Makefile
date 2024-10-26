# Makefile
include .env
export

.PHONY: clean-db postgres docker migrate-up rebuild clean-volumes build-app docker-down

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
	docker-compose down -v

# Очистка всех Docker volumes
clean-volumes:
	@echo "Cleaning up Docker volumes..."
	docker volume rm -f solana-bot_postgres_data || true
	docker volume prune -f

# Сборка приложения
build-app:
	@echo "Building application..."
	docker-compose build app

# Полная пересборка проекта
rebuild: docker-down clean-volumes build-app postgres migrate-up
	@echo "=== Rebuild completed successfully ==="