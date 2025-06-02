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
│   ├── task/               # Определение и управление задачами
│   └── wallet/wallet.go        # Управление кошельками
```

## 📦 Модульная структура
- [solana-bot/](#solana-bot)
    - [cmd/](#cmd) - Точки входа приложения
    - [configs/](#configs) - Конфигурационные файлы
    - [internal/](#internal) - Внутренняя логика приложения
        - [blockchain/](#rpc) - Клиент для Solana блокчейна
        - [bot/](#bot) - Основная логика бота
        - [dex/](#dex) - Интеграции с DEX
            - [pumpfun/](#pumpfun) - Реализация Pump.fun
            - [pumpswap/](#pumpswap) - Реализация Pump.swap
        - [monitor/](#monitor) - Мониторинг цен и операций
        - [task/](#task) - Определение и управление задачами
        - [wallet/](#wallet) - Управление кошельками

## Описание пакетов и файлов

### <a id="cmd"></a>cmd/

Точки входа в приложение:

- `bot/main.go` - Инициализирует и запускает бота, устанавливает контекст выполнения и обрабатывает сигналы операционной системы.

### <a id="configs"></a>configs/

Конфигурационные файлы (добавлены в .gitignore, создаются пользователем):

- `config.json` - Основная конфигурация бота
- `wallets.csv` - Файл с кошельками
- `tasks.csv` - Файл с задачами для бота

### <a id="internal"></a>internal/

Внутренняя логика приложения:

#### <a id="blockchain"></a>blockchain/

Реализация клиента Solana:

- `types.go` - Определяет интерфейсы и типы для взаимодействия с блокчейном


- `client.go` - Клиент для взаимодействия с Solana RPC API

#### <a id="bot"></a>bot/

Основная логика бота:

- `runner.go` - Управляет жизненным циклом бота и координирует работу компонентов
- `tasks.go` - Логика обработки задач, включая мониторинг цен
- `worker.go` - Реализует параллельное выполнение задач через worker pool

#### <a id="dex"></a>dex/

Интеграции с DEX:

- `base_adapter.go` - Базовый адаптер для DEX
- `factory.go` - Фабрика для создания DEX адаптеров
- `pumpfun_adapter.go` - Адаптер для Pump.fun
- `pumpswap_adapter.go` - Адаптер для Pump.swap
- `types.go` - Типы данных и интерфейсы для DEX
- `utils.go` - Вспомогательные функции для работы с DEX

##### <a id="pumpfun"></a>pumpfun/

Реализация Pump.fun DEX:

- `accounts.go` - Работа с аккаунтами и определение токенов
- `config.go` - Настройки для Pump.fun
- `discrete_pnl.go` - Расчет дискретного P&L с учетом ступенчатой bonding curve
- `instructions.go` - Инструкции для транзакций (buy, sell)
- `pumpfun.go` - Основная реализация DEX интерфейса
- `trade.go` - Логика подготовки транзакций для торговли
- `transactions.go` - Отправка и подтверждение транзакций
- `types.go` - Специфические типы данных Pump.fun

##### <a id="pumpswap"></a>pumpswap/

Реализация Pump.swap DEX:

- `calculations.go` - Расчеты для операций свапа и определения цены
- `config.go` - Конфигурация для Pump.swap
- `dex.go` - Основная реализация DEX интерфейса
- `errors.go` - Обработка специфических ошибок (slippage exceeded и др.)
- `instructions.go` - Инструкции для транзакций
- `pool.go` - Управление пулами ликвидности и расчеты ценообразования
- `pumpswap.go` - Вспомогательные функции и утилиты
- `transaction.go` - Создание, подписание и отправка транзакций
- `types.go` - Определение типов данных для Pump.swap

#### <a id="monitor"></a>monitor/

Мониторинг цен и операций:

- `input.go` - Обработка пользовательского ввода в режиме мониторинга
- `price.go` - Мониторинг цен токенов
- `session.go` - Управление сессиями мониторинга и отображение P&L


#### <a id="task"></a>task/

Определение и управление задачами:

- `config.go` - Конфигурация для задач
- `models.go` - Загрузка и валидация задач из CSV
- `task.go` - Определение структуры задачи и методов работы с ней

#### <a id="wallet"></a>wallet/

- `wallet.go` - Управление кошельками Solana, загрузка из CSV, подписание транзакций

## 🚀 Установка и запуск

### Локальная установка

```bash
# Клонирование репозитория
git clone https://github.com/rovshanmuradov/solana-bot.git

# Переход в директорию проекта
cd solana-bot

# Установка зависимостей
go mod download

# Сборка проекта
go build -o solana-bot cmd/bot/main.go

# Запуск бота
./solana-bot
```

## 🔄 Логика работы

### Инициализация:

1. Загрузка конфигурации из `configs/config.json`
2. Подключение к RPC-узлам Solana
3. Инициализация кошельков из `configs/wallets.csv`
4. Загрузка торговых задач из `configs/tasks.csv`


### Выполнение торговых операций:

- Параллельное выполнение задач через worker pool
- Автоматический свап токенов при достижении условий
- Управление приоритетом транзакций
- Автопродажа по достижении целевой цены


## 📊 Мониторинг и логирование

- Подробное логирование всех операций
- Метрики Prometheus для мониторинга производительности
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

### Линтинг кода

Проект использует локальную установку golangci-lint для обеспечения качества кода. Первый запуск команды `make lint` установит необходимую версию линтера локально в директорию проекта.

```bash
# Запустить линтер
make lint

# Запустить линтер с автоматическим исправлением проблем
make lint-fix
```

Конфигурация линтера находится в файле `.golangci.yml`. Pre-commit хук автоматически запускает линтер перед каждым коммитом для проверки измененных файлов.

## 📄 Лицензия

MIT License - смотрите файл LICENSE для деталей.