# Solana Trading Bot

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

### Пример вывода мониторинга

При запуске снайпинга с автоматическим мониторингом, бот будет отображать информацию о текущей цене, изменении цены, теоретической стоимости и P&L с дискретным расчетом (для Pump.fun):

```
make run
Building and running application...
go build -o solana-bot ./cmd/bot/main.go
./solana-bot
21:38:11	[INFO]	🌐 Configured RPC endpoints: 1
21:38:11	[INFO]	🎯 Primary RPC: https://mainnet.helius-rpc.com/?api-key=premium
21:38:11	[INFO]	✅ License validated (basic mode)
21:38:11	[INFO]	📋 Loaded 1 trading tasks
21:38:11	[INFO]	📋 Loaded 1 trading tasks
21:38:11	[INFO]	🚀 Starting execution with 1 workers
21:38:11	[INFO]	🚀 Trading worker started
21:38:11	[INFO]	⚡ Executing swap on Smart DEX for Dmig...pump
21:38:11	[INFO]	📊 Starting monitored trade for Dmig...pump
21:38:11	[INFO]	PumpFun configuration prepared	{"program_id": "6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P", "global_account": "4wTV1YmiEkRvAtNtsSGPtUrqRYQMe5SKy2uB4Jjaxnjf", "token_mint": "DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump", "event_authority": "Ce6TQqeHC9p8KetsN6JsjHK7UTZk7nasjjnr7XxXp9F1"}
21:38:11	[INFO]	🏗️  Creating PumpFun DEX for Dmig...pump
21:38:11	[INFO]	📧 Updated fee recipient: 62qc2CNXwrYqQScmEdiZFFAnJR262PxWEuNQtxfafNgV
21:38:11	[INFO]	🎯 Smart DEX selected: Pump.fun (bonding curve active)	{"token": "Dmig...pump"}
21:38:11	[INFO]	🎯 Pump.fun snipe: 0.000 SOL for Dmig...pump
21:38:11	[INFO]	💰 Starting Pump.fun buy: 0.000 SOL (20.0% slippage)
21:38:11	[INFO]	📊 Using exact SOL amount: 0.000010000 SOL
21:38:12	[INFO]	Using creator vault	{"vault": "9EPR5fRnTGhtyL5rcCUvf4iVtE9aL2CBmGZBXK7tmGQh", "creator": "7hGZjLKxMdkk5mykGKkeYYBdaaJA1zzziiRQgKuNYxb6"}
21:38:12	[INFO]	📤 Transaction sent: 5B6MSsKs...
21:38:12	[INFO]	⏳ Waiting for confirmation: 5B6MSsKs...
21:38:12	[INFO]	✅ Transaction confirmed: 5B6MSsKs...
21:38:12	[INFO]	✅ Transaction confirmed: 5B6MSsKs...
21:38:12	[INFO]	🎉 Trade executed successfully: swap
21:38:13	[INFO]	💰 Tokens received: 849815101
21:38:13	[INFO]	📊 Preparing monitoring for Dmig...pump (0.000 SOL)
21:38:13	[INFO]	🚀 Monitor started: 1203.904399 tokens @ $0.00000001 each

Monitoring started. Press Enter to sell tokens or 'q' to exit.
21:38:13	[INFO]	PriceMonitor: start	{"token": "DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump"}

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

21:38:14	[INFO]	💰 Sell requested by user

Preparing to sell tokens...
21:38:14	[INFO]	💱 Processing sell request for: DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump
Selling tokens now...
21:38:14	[INFO]	PriceMonitor: context done, exiting loop
21:38:14	[INFO]	💱 Starting token sell: DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump (100.0% at 20.0% slippage)
21:38:15	[INFO]	Selling tokens	{"token_mint": "DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump", "total_balance": 1203904399, "percent": 100, "tokens_to_sell": 1203904399}
21:38:15	[INFO]	💱 Starting Pump.fun sell: 1203904399 tokens (20.0% slippage)
21:38:15	[INFO]	Using creator vault for sell	{"vault": "9EPR5fRnTGhtyL5rcCUvf4iVtE9aL2CBmGZBXK7tmGQh", "creator": "7hGZjLKxMdkk5mykGKkeYYBdaaJA1zzziiRQgKuNYxb6"}
21:38:15	[INFO]	📤 Transaction sent: hxQYXoEV...
21:38:15	[INFO]	⏳ Waiting for confirmation: hxQYXoEV...
21:38:18	[INFO]	✅ Transaction confirmed: hxQYXoEV...
21:38:18	[INFO]	✅ Transaction confirmed: hxQYXoEV...
21:38:18	[INFO]	✅ Token sell completed successfully
21:38:18	[INFO]	✅ Tokens sold successfully!
Tokens sold successfully!
21:38:18	[INFO]	✅ All tasks completed
^Z
[29]  + 44706 suspended  make run
```

### Workflow для разработки

```bash
# Создание тестовой ветки
git checkout -b feature/your-feature-name

# Запуск с режимом отладки
go run cmd/bot/main.go -debug

# Запуск тестов
go test ./...

# Форматирование кода
go fmt ./...
```

### Makefile команды

```bash
# Локальный запуск
make run

# Полная пересборка проекта
make rebuild

# Запуск линтера локально
make lint

# Запуск линтера с автоисправлениями
make lint-fix

# Показать все доступные команды
make help
```


### Метрики производительности
- Время выполнения транзакций
- Успешность операций
- Статистика по DEX
- Мониторинг RPC соединений

### Уведомления
- Webhook интеграция
- Критические ошибки
- Статус выполнения заданий

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