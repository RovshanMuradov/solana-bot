# Makefile for solana-bot
export

.PHONY: run build dist clean check test lint format rebuild docker quick-dist help

# Development commands
run: ## Run the application
	@echo "Building and running application..."
	@mkdir -p configs
	go build -o solana-bot ./cmd/bot/main.go
	./solana-bot

build: ## Build for current platform
	./build.sh dev

dist: ## Build distribution for all platforms
	./build.sh dist

clean: ## Remove old builds
	./build.sh clean

check: ## Run all code checks
	./build.sh check

# Testing and code quality
test: ## Run tests
	go test ./... -v

lint: ## Run linter
	golangci-lint run

lint-fix: ## Run linter with auto-fixes
	golangci-lint run --fix

format: ## Format code
	go fmt ./...

# Docker commands
rebuild: ## Clean up Docker volumes and rebuild
	docker-compose down -v
	docker-compose up --build

docker: ## Build and run with Docker
	docker-compose up --build

# Quick distribution build
quick-dist: clean ## Quick distribution build without checks
	@echo "🚀 Quick distribution build..."
	@mkdir -p distribution/configs
	@GOOS=linux GOARCH=amd64 go build -o distribution/solana-bot-linux cmd/bot/main.go
	@GOOS=windows GOARCH=amd64 go build -o distribution/solana-bot-windows.exe cmd/bot/main.go
	@GOOS=darwin GOARCH=amd64 go build -o distribution/solana-bot-macos cmd/bot/main.go
	@echo "✅ Quick build completed!"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'