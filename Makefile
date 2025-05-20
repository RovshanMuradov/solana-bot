# Makefile
export

.PHONY: run rebuild lint lint-fix

# Локальный запуск
run:
	@echo "Building and running application..."
	@mkdir -p configs
	go build -o solana-bot ./cmd/bot/main.go
	./solana-bot

# Линтинг
lint: ## Запустить линтер локально
	golangci-lint run

lint-fix: ## Запустить линтер с автоисправлениями
	golangci-lint run --fix

help: ## Показать справку по командам
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'