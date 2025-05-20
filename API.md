# API для Solana Trading Bot

## Текущая структура проекта

- **Bot**: ядро системы (runner.go, worker.go)
- **DEX**: адаптеры для разных бирж
- **Monitor**: отслеживание цен и PnL
- **Task**: управление задачами
- **Blockchain**: взаимодействие с Solana
- **Wallet**: управление кошельками

## Рекомендации по API (простой подход)

Для небольшого проекта лучше избежать излишней формализации. Предлагается:

### 1. Создать пакет `botapi`

```go
// botapi/api.go
package botapi

import (
    "github.com/rovshanmuradov/solana-bot/internal/bot"
    "github.com/rovshanmuradov/solana-bot/internal/monitor"
    "github.com/rovshanmuradov/solana-bot/internal/task"
    "github.com/shopspring/decimal"
)

// BotAPI централизованный доступ к функциям бота
type BotAPI struct {
    Runner       *bot.Runner
    TaskManager  *task.Manager
}

// New создает экземпляр API
func New(runner *bot.Runner, taskManager *task.Manager) *BotAPI {
    return &BotAPI{
        Runner:      runner,
        TaskManager: taskManager,
    }
}
```

### 2. Основные функции API

```go
// Управление ботом
func (api *BotAPI) Start() error
func (api *BotAPI) Stop() error
func (api *BotAPI) Status() (isRunning bool, workerCount int)

// Управление задачами
func (api *BotAPI) Tasks() []task.Task
func (api *BotAPI) AddTask(t task.Task) error
func (api *BotAPI) RemoveTask(id string) error

// Мониторинг и торговля
func (api *BotAPI) MonitoringSessions() []monitor.Session
func (api *BotAPI) GetPnL(sessionID string) (investment, current, profit decimal.Decimal, profitPercent float64, err error)
func (api *BotAPI) SellToken(sessionID string, amount decimal.Decimal, slippage float64) (txID string, err error)

// Информация о кошельках
func (api *BotAPI) Wallets() []WalletInfo
func (api *BotAPI) TokenBalances(walletIndex int) ([]TokenBalance, error)
```

### 3. Новый пакет для TUI

Переместить ui/handler.go в новый пакет tui:

```go
// tui/tui.go
package tui

import "github.com/rovshanmuradov/solana-bot/internal/botapi"

// UI представляет терминальный интерфейс
type UI struct {
    api *botapi.BotAPI
}

// New создает новый UI
func New(api *botapi.BotAPI) *UI {
    return &UI{api: api}
}

// Run запускает интерфейс
func (ui *UI) Run() error {
    // Реализация TUI
}
```

### 4. Простые модели данных

```go
// botapi/types.go

// WalletInfo информация о кошельке
type WalletInfo struct {
    Address     string
    SolBalance  float64
    TokensCount int
}

// TokenBalance баланс токена
type TokenBalance struct {
    Symbol     string
    Address    string
    Balance    decimal.Decimal
    USDValue   decimal.Decimal
}
```

### 5. Обновление main.go

```go
func main() {
    // Инициализация компонентов
    runner := bot.NewRunner(...)
    taskManager := task.NewManager(...)
    
    // Создание API
    api := botapi.New(runner, taskManager)
    
    // Запуск TUI
    ui := tui.New(api)
    ui.Run()
}
```

## Преимущества этого подхода

1. **Простота** — минимум дополнительных абстракций
2. **Практичность** — прямой доступ к нужным функциям
3. **Идиоматичность для Go** — использование простых функций вместо сложных интерфейсов
4. **Расширяемость** — легко добавлять новые функции при необходимости

## Дальнейшие шаги

1. Реализовать базовый набор функций API
2. Создать новый пакет для TUI
3. Обеспечить обработку ошибок
4. Добавить логирование при необходимости