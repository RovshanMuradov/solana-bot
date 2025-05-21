# Финальный план по API для Solana Trading Bot

## Сравнение подходов

| Аспект | API.md | min.md | Итоговое решение |
|--------|--------|--------|------------------|
| Сложность | Формализованная структура | Минималистичный подход | Прагматичный с элементами формализации |
| Структура | Централизованный botapi | Функциональный botapi | botapi с миксинами для группировки |
| Организация UI | Отдельный пакет | Отдельный пакет | Отдельный пакет с чистым API |
| Особенности | Модели данных | Обработка ошибок | Обработка ошибок + интерфейсы |

## Пошаговый план

### 1. Подготовка (10 минут)

```bash
mkdir -p internal/botapi
mkdir -p internal/tui
mkdir -p internal/errs
mkdir -p internal/core
```

### 2. Обработка ошибок (20 минут)

Создать файл `internal/errs/errors.go`:

```go
package errs

import "fmt"

// Типы ошибок
const (
    ErrRPC         = "rpc_error"
    ErrInvalidTask = "invalid_task"
    ErrDEXConnect  = "dex_connection"
)

// BotError структурированная ошибка
type BotError struct {
    Type    string
    Message string
    Cause   error
}

func (e *BotError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %s (%v)", e.Type, e.Message, e.Cause)
    }
    return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// New создает новую ошибку
func New(errType, message string, cause error) error {
    return &BotError{Type: errType, Message: message, Cause: cause}
}

// Wrap оборачивает существующую ошибку
func Wrap(errType, message string, err error) error {
    if err == nil {
        return nil
    }
    return &BotError{Type: errType, Message: message, Cause: err}
}
```

### 3. API с миксинами (60 минут)

#### 3.1. Основная структура API

Файл `internal/botapi/api.go`:

```go
package botapi

// BotAPI централизованный доступ к боту
type BotAPI struct {
    Runner  *bot.Runner
    TaskMgr *task.Manager
    
    // Миксины для группировки функциональности
    *botControl
    *monitoringControl
    *tradingControl
}

// New создает API
func New(runner *bot.Runner, taskMgr *task.Manager) *BotAPI {
    api := &BotAPI{
        Runner:  runner,
        TaskMgr: taskMgr,
    }
    
    // Инициализация миксинов
    api.botControl = &botControl{api: api}
    api.monitoringControl = &monitoringControl{api: api}
    api.tradingControl = &tradingControl{api: api}
    
    return api
}
```

#### 3.2. Миксин для управления ботом

Файл `internal/botapi/bot_control.go`:

```go
package botapi

// botControl операции управления ботом
type botControl struct {
    api *BotAPI
}

// Start запускает бота
func (c *botControl) Start() error {
    return c.api.Runner.Start()
}

// Stop останавливает бота
func (c *botControl) Stop() error {
    return c.api.Runner.Stop()
}

// Status возвращает статус бота
func (c *botControl) Status() (isRunning bool, workerCount int) {
    return c.api.Runner.IsRunning(), c.api.Runner.WorkerCount()
}
```

#### 3.3. Миксин для мониторинга

Файл `internal/botapi/monitoring_control.go`:

```go
package botapi

// monitoringControl операции мониторинга
type monitoringControl struct {
    api *BotAPI
}

// GetSessions возвращает сессии мониторинга
func (c *monitoringControl) GetSessions() []monitor.Session {
    return c.api.Runner.GetMonitoringSessions()
}

// GetPnL возвращает P&L
func (c *monitoringControl) GetPnL(sessionID string) (PnLInfo, error) {
    investment, current, profit, err := c.api.Runner.GetPnL(sessionID)
    if err != nil {
        return PnLInfo{}, err
    }
    
    var pctProfit decimal.Decimal
    if !investment.IsZero() {
        pctProfit = profit.Div(investment).Mul(decimal.NewFromInt(100))
    }
    
    return PnLInfo{
        InitialInvestment: investment,
        CurrentValue:      current,
        PnLAmount:         profit,
        PnLPercentage:     pctProfit,
    }, nil
}
```

#### 3.4. Миксин для торговли

Файл `internal/botapi/trading_control.go`:

```go
package botapi

// tradingControl операции торговли
type tradingControl struct {
    api *BotAPI
}

// SellToken продает токен
func (c *tradingControl) SellToken(sessionID string, amount decimal.Decimal) (string, error) {
    return c.api.Runner.SellToken(sessionID, amount)
}

// AddTask добавляет задачу
func (c *tradingControl) AddTask(task task.Task) error {
    return c.api.TaskMgr.AddTask(task)
}
```

#### 3.5. Модели данных

Файл `internal/botapi/types.go`:

```go
package botapi

// PnLInfo информация о прибыли/убытке
type PnLInfo struct {
    InitialInvestment decimal.Decimal
    CurrentValue      decimal.Decimal
    PnLAmount         decimal.Decimal
    PnLPercentage     decimal.Decimal
}

// TokenBalance баланс токена
type TokenBalance struct {
    Symbol   string
    Address  string
    Balance  decimal.Decimal
    USDValue decimal.Decimal
}
```

### 4. Интерфейсы для тестирования (20 минут)

Файл `internal/core/interfaces.go`:

```go
package core

// DEXProvider интерфейс для биржи
type DEXProvider interface {
    Swap(params dex.SwapParams) (string, error)
    GetPrice(tokenAddress string) (decimal.Decimal, error)
}

// TaskProvider интерфейс для задач
type TaskProvider interface {
    GetTasks() []task.Task
    AddTask(task task.Task) error
}

// MonitorProvider интерфейс для мониторинга
type MonitorProvider interface {
    StartSession(params MonitorParams) (string, error)
    GetPnL(sessionID string) (decimal.Decimal, decimal.Decimal, decimal.Decimal, error)
}
```

### 5. TUI в отдельном пакете (40 минут)

Файл `internal/tui/handler.go`:

```go
package tui

// Handler обработчик TUI
type Handler struct {
    api    *botapi.BotAPI
    logger *zap.Logger
}

// NewHandler создает обработчик TUI
func NewHandler(api *botapi.BotAPI, logger *zap.Logger) *Handler {
    return &Handler{api: api, logger: logger}
}

// Run запускает TUI
func (h *Handler) Run() error {
    // Реализация TUI с использованием API
    return nil
}
```

### 6. Обновление main.go (20 минут)

```go
package main

func main() {
    // Инициализация логгера
    logger, err := zap.NewProduction()
    if err != nil {
        log.Fatal(err)
    }
    defer logger.Sync()

    // Загрузка конфигурации
    cfg, err := loadConfig("configs/config.json")
    if err != nil {
        logger.Fatal("Config error", zap.Error(err))
    }

    // Инициализация компонентов
    taskManager := task.NewManager(cfg.TasksPath, logger)
    runner := bot.NewRunner(cfg, taskManager, logger)

    // Создание API
    api := botapi.New(runner, taskManager)

    // Запуск TUI
    handler := tui.NewHandler(api, logger)
    if err := handler.Run(); err != nil {
        logger.Fatal("UI error", zap.Error(err))
    }
}
```

### 7. Тестирование и отладка (30 минут)

1. Базовые тесты API
2. Тесты обработки ошибок
3. Интеграционные тесты

## Преимущества решения

✅ **Чистый API**: централизованный доступ к функциям бота  
✅ **Логическая группировка**: методы сгруппированы в миксины  
✅ **Разделение UI и логики**: TUI работает через API  
✅ **Обработка ошибок**: структурированные ошибки с контекстом  
✅ **Тестируемость**: интерфейсы для легкого тестирования  
✅ **Модульность**: простота добавления новых функций  

## Время реализации: 3 часа

Это решение сочетает лучшие аспекты обоих подходов: прагматичность для небольшого проекта и хорошую структуру для будущего расширения.