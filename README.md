Show Image
# Solana Trading Bot

Solana Trading Bot - это высокопроизводительное решение для автоматизированной торговли на DEX в сети Solana. Бот поддерживает различные DEX (currently Raydium и Pump.fun) и предоставляет гибкие возможности для настройки торговых стратегий.

## 🌟 Основные возможности

- 🚀 Поддержка множества DEX (Raydium, Pump.fun)
- 💼 Управление множеством кошельков
- ⚡ Высокопроизводительная обработка транзакций
- 🔄 Автоматический свап токенов
- 📊 Мониторинг цен в реальном времени
- 🛡️ Надежная обработка ошибок и переподключений
- 📈 Настраиваемые торговые стратегии
- 🔍 Подробное логирование операций

## 🏗️ Архитектура

Бот построен на модульной архитектуре, что обеспечивает легкость расширения и поддержки:

```
solana-bot/
├── cmd/                    # Точки входа приложения
├── internal/              
│   ├── blockchain/         # Взаимодействие с блокчейном
│   ├── config/             # Конфигурация приложения
│   ├── dex/                # Интеграции с DEX
│   ├── eventlistener/      # WebSocket мониторинг
│   ├── sniping/            # Логика торговли
│   ├── types/              # Общие типы данных
│   ├── wallet/             # Управление кошельками
│   └── utils/              # Вспомогательные функции
└── configs/                # Конфигурационные файлы
```

## 🔄 Логика работы

### Инициализация:

1. Загрузка конфигурации из `configs/config.json`
2. Подключение к RPC-узлам Solana
3. Инициализация кошельков из `configs/wallets.csv`
4. Загрузка торговых задач из `configs/tasks.csv`

### Мониторинг:

- WebSocket подключение для отслеживания событий
- Мониторинг цен и ликвидности
- Автоматическое переподключение при разрыве соединения

### Выполнение торговых операций:

- Параллельное выполнение задач через worker pool
- Автоматический свап токенов при достижении условий
- Управление приоритетом транзакций
- Автопродажа по достижении целевой цены

## ⚙️ Конфигурация

### `config.json`

```json
{
    "license": "your-license-key",
    "rpc_list": ["https://rpc1.solana.com", "https://rpc2.solana.com"],
    "websocket_url": "wss://api.solana.com",
    "monitor_delay": 1000,
    "rpc_delay": 100,
    "price_delay": 500,
    "debug_logging": true,
    "tps_logging": true,
    "retries": 3,
    "webhook_url": "https://your-webhook.com",
    "workers": 5
}
```

### `wallets.csv`

```csv
name,private_key
wallet1,your-private-key-1
wallet2,your-private-key-2
```

### `tasks.csv`

```csv
task_name,module,workers,wallet_name,delta,priority_fee,amm_id,source_token,target_token,amount_in,min_amount_out,autosell_percent,autosell_delay,autosell_amount,transaction_delay,autosell_priority_fee
Task1,Raydium,2,wallet1,100,0.000001,amm-id-1,SOL,USDC,1.0,0.9,150,5,0.5,1000,0.000002
```

## 🚀 Установка и запуск

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

## 🔧 Настройка DEX

Бот поддерживает различные DEX через модульную систему. Каждый DEX реализует общий интерфейс:

```go
type DEX interface {
        Name() string
        PrepareSwapInstruction(ctx context.Context, ...) (solana.Instruction, error)
        ExecuteSwap(ctx context.Context, task *Task, wallet *Wallet) error
}
```

## 📊 Мониторинг и логирование

- Подробное логирование всех операций
- Метрики Prometheus для мониторинга производительности
- WebSocket уведомления о важных событиях
- Автоматическое восстановление при сбоях

## 🔐 Безопасность

- Безопасное хранение приватных ключей
- Защита от двойной отправки транзакций
- Проверка подписи транзакций
- Контроль лимитов и проскальзывания

## 📝 Пример использования

1. Настройте конфигурационные файлы в директории `configs/`
2. Запустите бота
3. Мониторьте логи для отслеживания операций
4. Получайте уведомления о выполненных транзакциях

## 🤝 Вклад в развитие

Мы приветствуем вклад в развитие проекта! Пожалуйста:

1. Форкните репозиторий
2. Создайте ветку для ваших изменений
3. Отправьте pull request

## 📄 Лицензия

MIT License - смотрите файл LICENSE для деталей.