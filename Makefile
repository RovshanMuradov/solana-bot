# Makefile
include .env
export

.PHONY: clean-db postgres docker migrate-up rebuild clean-volumes build-app docker-down run run-local build deploy clean

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

# Сборка приложения в Docker
build-app:
	@echo "Building application in Docker..."
	docker-compose build app

# Полная пересборка проекта
rebuild: docker-down clean-volumes build-app postgres migrate-up
	@echo "=== Rebuild completed successfully ==="

# Запуск в Docker
run:
	@echo "Running application in Docker..."
	docker-compose run --rm app /bot

# Локальная сборка
build:
	@echo "Building application locally..."
	go build -o solana-bot ./cmd/bot/main.go

deploy: build
	@echo "Deploying application..."
	mkdir -p $(DEPLOY_DIR)
	cp solana-bot $(DEPLOY_DIR)/
	cp -r configs/ $(DEPLOY_DIR)/
	@echo "Deployed to $(DEPLOY_DIR)"

# Локальный запуск
run-local: build
	@echo "Running application locally..."
	./solana-bot

# Очистка
clean:
	@echo "Cleaning local builds..."
	rm -f solana-bot
	rm -rf $(DEPLOY_DIR)