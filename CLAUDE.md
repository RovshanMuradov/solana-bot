# CLAUDE.md - Solana Bot Repository Guidelines

## Build Commands
- `make build-app` - Build the application in Docker
- `go build -o bot ./cmd/bot` - Build locally
- `go run ./cmd/bot/main.go` - Run without building
- `make rebuild` - Full rebuild (Docker + postgres)

## Test Commands
- `go test ./...` - Run all tests
- `go test ./internal/path/to/package` - Test a specific package
- `go test -v ./internal/path/to/package` - Verbose test output
- `go test -run TestFunctionName ./internal/path/to/package` - Run a specific test

## Lint Commands
- `go vet ./...` - Basic code analysis
- `gofmt -s -w .` - Format Go source code

## Code Style Guidelines
- **Imports**: Group stdlib, external, and internal imports with blank lines between
- **Error handling**: Use `utils.HandleError` or `utils.WrapError` for consistent error handling
- **Logging**: Use zap logger, structured logs with appropriate levels
- **Naming**: Use PascalCase for exported symbols, camelCase for internal ones
- **Comments**: Add package/file headers, document all exported functions
- **Error returns**: Return detailed errors with context using fmt.Errorf and %w
- **Context**: Pass context.Context as first parameter in long-running operations