# Solana Trading Bot

[🇺🇸 English](#english) | [🇷🇺 Русский](#русский)

---

## English

Solana Trading Bot is a high-performance solution for automated trading on DEX in the Solana network. The bot supports various DEX platforms (currently Raydium, Pump.fun, and Pump.swap) and provides flexible capabilities for configuring trading strategies.

## 📋 Table of Contents

- [Key Features](#-key-features)
- [Architecture](#️-architecture)
- [Modular Structure](#-modular-structure)
- [Configuration](#️-configuration)
- [Installation and Launch](#-installation-and-launch)
- [Working Logic](#-working-logic)
- [Monitoring and Logging](#-monitoring-and-logging)
- [License](#-license)

## 🌟 Key Features

- 🚀 Support for multiple DEX (Raydium, Pump.fun, Pump.swap)
- 💼 Multi-wallet management
- ⚡ High-performance transaction processing
- 🔄 Automatic token swapping
- 📊 Real-time price monitoring
- 🛡️ Reliable error handling and reconnections
- 📈 Configurable trading strategies
- 🔍 Detailed operation logging
- 🔐 Secure private key management

## 🏗️ Architecture

The bot is built on a modular architecture that ensures ease of expansion and maintenance:

```
solana-bot/
├── cmd/                    # Application entry points
│   └── bot/                # Main executable file
├── configs/                # Configuration files
├── internal/               # Internal application logic
│   ├── blockchain/         # Blockchain interaction
│   │   ├── solbc/          # Solana blockchain client
│   │   └── types.go        # Blockchain interfaces and types
│   ├── bot/                # Main bot logic
│   │   ├── runner.go       # Bot process management
│   │   ├── tasks.go        # Task processing
│   │   └── worker.go       # Concurrent task execution
│   ├── config/             # Application configuration
│   ├── dex/                # DEX integrations
│   │   ├── pumpfun/        # Pump.fun implementation
│   │   ├── pumpswap/       # Pump.swap implementation
│   │   └── factory.go      # Factory for creating DEX adapters
│   ├── monitor/            # Price and operation monitoring
│   ├── task/               # Task management
│   │   ├── config.go       # Task configuration
│   │   ├── manager.go      # Task manager
│   │   └── task.go         # Task model
│   └── wallet/             # Wallet management
└── pkg/                    # Public libraries
```

## 🔧 Modular Structure

### Blockchain Layer
- **solbc**: Client for interacting with Solana blockchain
- Support for RPC and WebSocket connections
- Automatic reconnection on failures

### DEX Adapters
- **Universal DEX Interface**: Unified interface for all supported DEX
- **Pump.fun**: Specialized support for bonding curve tokens
- **Pump.swap**: Optimization for swaps on Pump protocol
- **Raydium**: Integration with the largest DEX on Solana

### Task Management
- **CSV Loading**: Task configuration through files
- **Validation**: Parameter correctness checking
- **Prioritization**: Execution queue management

### Monitoring & Execution
- **Real-time**: Price and position status monitoring
- **Concurrent Workers**: Parallel task execution
- **Error Recovery**: Automatic recovery after errors

## ⚙️ Configuration

### config.json
```json
{
  "license": "YOUR_LICENSE_KEY",
  "rpc_list": [
    "https://api.mainnet-beta.solana.com",
    "https://your-premium-rpc-endpoint.com"
  ],
  "websocket_url": "wss://api.mainnet-beta.solana.com",
  "monitor_delay": 10000,
  "rpc_delay": 100,
  "price_delay": 1000,
  "debug_logging": false,
  "tps_logging": false,
  "retries": 8,
  "webhook_url": "",
  "workers": 1
}
```

### wallets.csv
```csv
name,private_key
main_wallet,YOUR_PRIVATE_KEY_HERE
trading_wallet_2,ANOTHER_PRIVATE_KEY
```

### tasks.csv
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
snipe_task,pumpfun,main_wallet,snipe,0.001,20.0,0.000001,TokenMintAddress,200000,25
swap_task,raydium,trading_wallet_2,swap,0.005,15.0,0.000002,AnotherTokenMint,300000,50
```

## 🚀 Installation and Launch

### Requirements
- Go 1.21 or higher
- Access to Solana RPC nodes
- Wallet private keys

### Quick Start

1. **Clone Repository**
```bash
git clone https://github.com/RovshanMuradov/solana-bot.git
cd solana-bot
```

2. **Build Project**
```bash
make build
# or
go build -o solana-bot ./cmd/bot/main.go
```

3. **Configure Settings**
```bash
mkdir -p configs
# Create and fill config.json, wallets.csv, tasks.csv
```

4. **Launch**
```bash
make run
# or
./solana-bot
```

### Make Commands

```bash
make run          # Build and run
make build        # Build for current platform
make dist         # Build for all platforms
make test         # Run tests
make lint         # Code check
make clean        # Clean builds
```

## 🔄 Working Logic

### 1. Initialization
- Load configuration from `config.json`
- Connect to Solana RPC nodes
- Load wallets from `wallets.csv`
- Initialize worker pool

### 2. Task Processing
- Load tasks from `tasks.csv`
- Parameter validation
- Distribution across workers
- Parallel execution

### 3. Operation Execution
- **Snipe**: Quick purchase of new tokens
- **Swap**: Token exchange at market price
- **Sell**: Position selling

### 4. Monitoring
- Real-time price tracking
- P&L calculation
- Important event notifications

## 📊 Monitoring and Logging

### Structured Logs
```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "message": "Task executed successfully",
  "task": "snipe_pump_token",
  "token": "TokenMintAddress",
  "amount": 0.001,
  "price": 0.0001234
}
```

### Monitoring Features
- Detailed logging of all operations
- Performance metrics and statistics
- WebSocket notifications for important events
- Automatic recovery on failures

### Example Monitoring Output

When running sniping with automatic monitoring, the bot will display information about current price, price change, theoretical value, and P&L with discrete calculation (for Pump.fun):

```
make run
Building and running application...
go build -o solana-bot ./cmd/bot/main.go
./solana-bot
21:38:11	[INFO]	🌐 Configured RPC endpoints: 1
21:38:11	[INFO]	🎯 Primary RPC: https://mainnet.helius-rpc.com/?api-key=premium
21:38:11	[INFO]	✅ License validated (basic mode)
21:38:11	[INFO]	📋 Loaded 1 trading tasks
21:38:11	[INFO]	🚀 Starting execution with 1 workers
21:38:11	[INFO]	🚀 Trading worker started
21:38:11	[INFO]	⚡ Executing swap on Smart DEX for Dmig...pump
21:38:11	[INFO]	📊 Starting monitored trade for Dmig...pump

╔════════════════ TOKEN MONITOR ════════════════╗
║ Token: DmigFW…74pump                          ║
╟───────────────────────────────────────────────╢
║ Current Price:       0.00000003           SOL ║
║ Initial Price:       0.00000001           SOL ║
║ Price Change:        +236.60%                 ║
║ Tokens Owned:        1203.904399              ║
╟───────────────────────────────────────────────╢
║ Sold (Estimate):     0.00003332           SOL ║
║ Invested:            0.00000990           SOL ║
║ P&L:                 +0.00002342 SOL (236.60%) ║
╚═══════════════════════════════════════════════╝
Press Enter to sell tokens, 'q' to exit without selling
```

### Development Workflow

```bash
# Create test branch
git checkout -b feature/your-feature-name

# Run with debug mode
go run cmd/bot/main.go -debug

# Run tests
go test ./...

# Format code
go fmt ./...
```

### Makefile Commands

```bash
# Local run
make run

# Full project rebuild
make rebuild

# Run linter locally
make lint

# Run linter with auto-fixes
make lint-fix

# Show all available commands
make help
```

### Performance Metrics
- Transaction execution time
- Operation success rate
- DEX statistics
- RPC connection monitoring

### Notifications
- Webhook integration
- Critical errors
- Task execution status

## 🔐 Security

- **Local Key Storage**: Private keys are not transmitted over network
- **Transaction Validation**: All parameters checked before execution
- **Rate Limiting**: Protection from RPC node overload
- **Error Isolation**: Error isolation between tasks

## 🛠️ Development

### Adding New DEX
1. Create package in `internal/dex/newdex/`
2. Implement `DEX` interface
3. Add to factory `dex.GetDEXByName()`
4. Write tests

### Logging Configuration
```go
logger := zap.NewProduction()
defer logger.Sync()
```

### Testing
```bash
go test ./... -v
go test ./... -race
```

## 🆔 Versions

### v1.0.0 - Stable Version
- Full support for all declared features
- Ready for production use
- Basic command-line interface

### v1.1.0-beta - TUI Interface (Beta)
- New terminal user interface
- Interactive real-time monitoring
- Improved UX for trading management

## 📄 License

This project is distributed under the MIT License. See [LICENSE](LICENSE) file for details.

## 🤝 Support

- **Issues**: [GitHub Issues](https://github.com/RovshanMuradov/solana-bot/issues)
- **Discussions**: [GitHub Discussions](https://github.com/RovshanMuradov/solana-bot/discussions)

## ⚠️ Disclaimer

This bot is intended for educational and research purposes. Cryptocurrency trading involves high risks. Use at your own risk. Authors are not responsible for financial losses.

---

## Русский

Solana Trading Bot - это высокопроизводительное решение для автоматизированной торговли на DEX в сети Solana. Бот поддерживает различные DEX (в настоящее время Raydium, Pump.fun и Pump.swap) и предоставляет гибкие возможности для настройки торговых стратегий.

## 📋 Оглавление

- [Основные возможности](#-основные-возможности)
- [Архитектура](#-архитектура)
- [Модульная структура](#-модульная-структура)
- [Конфигурация](#-конфигурация)
- [Установка и запуск](#-установка-и-запуск)
- [Логика работы](#-логика-работы)
- [Мониторинг и логирование](#-мониторинг-и-логирование)
- [Лицензия](#-лицензия)

## 🌟 Основные возможности

- 🚀 Поддержка множества DEX (Raydium, Pump.fun, Pump.swap)
- 💼 Управление множеством кошельков
- ⚡ Высокопроизводительная обработка транзакций
- 🔄 Автоматический свап токенов
- 📊 Мониторинг цен в реальном времени
- 🛡️ Надежная обработка ошибок и переподключений
- 📈 Настраиваемые торговые стратегии
- 🔍 Подробное логирование операций
- 🔐 Безопасное управление приватными ключами

## 🏗️ Архитектура

Бот построен на модульной архитектуре, что обеспечивает легкость расширения и поддержки:

```
solana-bot/
├── cmd/                    # Точки входа приложения
│   └── bot/                # Основной исполняемый файл
├── configs/                # Конфигурационные файлы
├── internal/               # Внутренняя логика приложения
│   ├── blockchain/         # Взаимодействие с блокчейном
│   │   ├── solbc/          # Клиент для Solana блокчейна
│   │   └── types.go        # Интерфейсы и типы блокчейна
│   ├── bot/                # Основная логика бота
│   │   ├── runner.go       # Управление процессом бота
│   │   ├── tasks.go        # Обработка заданий
│   │   └── worker.go       # Конкурентное выполнение заданий
│   ├── config/             # Конфигурация приложения
│   ├── dex/                # Интеграции с DEX
│   │   ├── pumpfun/        # Реализация Pump.fun
│   │   ├── pumpswap/       # Реализация Pump.swap
│   │   └── factory.go      # Фабрика для создания DEX-адаптеров
│   ├── monitor/            # Мониторинг цен и операций
│   ├── task/               # Управление заданиями
│   │   ├── config.go       # Конфигурация заданий
│   │   ├── manager.go      # Менеджер заданий
│   │   └── task.go         # Модель задания
│   └── wallet/             # Управление кошельками
└── pkg/                    # Публичные библиотеки
```

[Полная русская версия README сохраняется здесь со всеми остальными разделами...]

## 🔧 Модульная структура

### Blockchain Layer
- **solbc**: Клиент для взаимодействия с блокчейном Solana
- Поддержка RPC и WebSocket соединений
- Автоматическое переподключение при сбоях

### DEX Adapters
- **Универсальный интерфейс DEX**: Единый интерфейс для всех поддерживаемых DEX
- **Pump.fun**: Специализированная поддержка bonding curve токенов
- **Pump.swap**: Оптимизация для свопов на Pump протоколе
- **Raydium**: Интеграция с крупнейшим DEX на Solana

### Task Management
- **Загрузка из CSV**: Конфигурация заданий через файлы
- **Валидация**: Проверка корректности параметров
- **Приоритизация**: Управление очередью выполнения

### Monitoring & Execution
- **Реальное время**: Мониторинг цен и состояния позиций
- **Concurrent Workers**: Параллельное выполнение заданий
- **Error Recovery**: Автоматическое восстановление после ошибок

## ⚙️ Конфигурация

### config.json
```json
{
  "license": "YOUR_LICENSE_KEY",
  "rpc_list": [
    "https://api.mainnet-beta.solana.com",
    "https://your-premium-rpc-endpoint.com"
  ],
  "websocket_url": "wss://api.mainnet-beta.solana.com",
  "monitor_delay": 10000,
  "rpc_delay": 100,
  "price_delay": 1000,
  "debug_logging": false,
  "tps_logging": false,
  "retries": 8,
  "webhook_url": "",
  "workers": 1
}
```

### wallets.csv
```csv
name,private_key
main_wallet,YOUR_PRIVATE_KEY_HERE
trading_wallet_2,ANOTHER_PRIVATE_KEY
```

### tasks.csv
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
snipe_task,pumpfun,main_wallet,snipe,0.001,20.0,0.000001,TokenMintAddress,200000,25
swap_task,raydium,trading_wallet_2,swap,0.005,15.0,0.000002,AnotherTokenMint,300000,50
```

## 🚀 Установка и запуск

### Требования
- Go 1.21 или выше
- Доступ к RPC узлам Solana
- Приватные ключи кошельков

### Быстрый старт

1. **Клонирование репозитория**
```bash
git clone https://github.com/RovshanMuradov/solana-bot.git
cd solana-bot
```

2. **Сборка проекта**
```bash
make build
# или
go build -o solana-bot ./cmd/bot/main.go
```

3. **Настройка конфигурации**
```bash
mkdir -p configs
# Создать и заполнить config.json, wallets.csv, tasks.csv
```

4. **Запуск**
```bash
make run
# или
./solana-bot
```

### Команды Make

```bash
make run          # Сборка и запуск
make build        # Сборка для текущей платформы
make dist         # Сборка для всех платформ
make test         # Запуск тестов
make lint         # Проверка кода
make clean        # Очистка сборок
```

## 🔄 Логика работы

### 1. Инициализация
- Загрузка конфигурации из `config.json`
- Подключение к RPC узлам Solana
- Загрузка кошельков из `wallets.csv`
- Инициализация пула воркеров

### 2. Обработка заданий
- Загрузка заданий из `tasks.csv`
- Валидация параметров
- Распределение по воркерам
- Параллельное выполнение

### 3. Выполнение операций
- **Snipe**: Быстрая покупка новых токенов
- **Swap**: Обмен токенов по рыночной цене
- **Sell**: Продажа позиций

### 4. Мониторинг
- Отслеживание цен в реальном времени
- Расчет P&L
- Уведомления о важных событиях

## 📊 Мониторинг и логирование

### Структурированные логи
```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "message": "Task executed successfully",
  "task": "snipe_pump_token",
  "token": "TokenMintAddress",
  "amount": 0.001,
  "price": 0.0001234
}
```

### Функции мониторинга
- Подробное логирование всех операций
- Метрики производительности и статистика
- WebSocket уведомления о важных событиях
- Автоматическое восстановление при сбоях

## 🔐 Безопасность

- **Локальное хранение ключей**: Приватные ключи не передаются в сеть
- **Валидация транзакций**: Проверка всех параметров перед выполнением
- **Rate limiting**: Защита от перегрузки RPC узлов
- **Error isolation**: Изоляция ошибок между заданиями

## 🛠️ Разработка

### Добавление нового DEX
1. Создать пакет в `internal/dex/newdex/`
2. Реализовать интерфейс `DEX`
3. Добавить в фабрику `dex.GetDEXByName()`
4. Написать тесты

### Настройка логирования
```go
logger := zap.NewProduction()
defer logger.Sync()
```

### Тестирование
```bash
go test ./... -v
go test ./... -race
```

## 🆔 Версии

### v1.0.0 - Стабильная версия
- Полная поддержка всех заявленных функций
- Готова для продакшн-использования
- Базовый интерфейс командной строки

### v1.1.0-beta - TUI Interface (Beta)
- Новый терминальный пользовательский интерфейс
- Интерактивный мониторинг в реальном времени
- Улучшенный UX для управления торговлей

## 📄 Лицензия

Этот проект распространяется под лицензией MIT. Подробности в файле [LICENSE](LICENSE).

## 🤝 Поддержка

- **Issues**: [GitHub Issues](https://github.com/RovshanMuradov/solana-bot/issues)
- **Discussions**: [GitHub Discussions](https://github.com/RovshanMuradov/solana-bot/discussions)

## ⚠️ Дисклеймер

Этот бот предназначен для образовательных и исследовательских целей. Торговля криптовалютами сопряжена с высокими рисками. Используйте на свой страх и риск. Авторы не несут ответственности за финансовые потери.