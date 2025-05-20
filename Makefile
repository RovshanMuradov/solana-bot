# Makefile
export

.PHONY: run rebuild

# Локальный запуск
run:
	@echo "Building and running application..."
	@mkdir -p configs
	go build -o solana-bot ./cmd/bot/main.go
	./solana-bot

# Полная пересборка проекта
rebuild:
	@echo "Cleaning up Docker volumes..."
	docker volume rm -f solana-bot_postgres_data || true
	docker volume prune -f
	@echo "Building application in Docker..."
	docker-compose build app
	@echo "Starting PostgreSQL..."
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 10
	@echo "Running migrations..."
	docker-compose run --rm app ./bot
	@echo "=== Rebuild completed successfully ==="