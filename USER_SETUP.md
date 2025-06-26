# 🤖 Solana Trading Bot - Руководство пользователя

## 📋 Содержание
- [Что вам нужно](#-что-вам-нужно)
- [Первоначальная настройка](#-первоначальная-настройка)
- [Конфигурация](#️-настройка-конфигурации)
- [Запуск бота](#-запуск)
- [Мониторинг и управление](#-интерфейс-мониторинга)
- [Продвинутые настройки](#-продвинутые-настройки)
- [Устранение неполадок](#-устранение-неполадок)
- [FAQ](#-часто-задаваемые-вопросы)

## 📁 Что вам нужно

### Минимальные требования:
- ✅ **Операционная система**: Windows 10+, macOS 10.14+, Linux (Ubuntu 20.04+)
- ✅ **Исполняемый файл**: `solana-bot` (или `solana-bot.exe` для Windows)
- ✅ **Лицензионный ключ**: Получите у разработчика
- ✅ **Solana кошелек**: С минимум 0.1 SOL для транзакций
- ✅ **Интернет**: Стабильное соединение

### Структура файлов:
```
solana-bot/
├── solana-bot           # Исполняемый файл
└── configs/            # Папка с конфигурацией
    ├── config.json     # Основные настройки
    ├── wallets.csv     # Ваши кошельки  
    └── tasks.csv       # Торговые задачи
```

## 🚀 Первоначальная настройка

### Шаг 1: Создание структуры
```bash
# Создайте папку для бота
mkdir solana-bot
cd solana-bot

# Создайте папку для конфигурации
mkdir configs
```

### Шаг 2: Разместите исполняемый файл
Скопируйте `solana-bot` (или `solana-bot.exe`) в созданную папку.

## ⚙️ Настройка конфигурации

### 1. config.json - Основные настройки

#### Базовая конфигурация:
```json
{
  "license": "ВАШ-ЛИЦЕНЗИОННЫЙ-КЛЮЧ",
  "rpc_list": [
    "https://api.mainnet-beta.solana.com"
  ],
  "websocket_url": "wss://api.mainnet-beta.solana.com",
  "monitor_delay": 1000,
  "rpc_delay": 100,
  "price_delay": 1000,
  "debug_logging": false,
  "tps_logging": false,
  "retries": 3,
  "webhook_url": "",
  "workers": 1
}
```

#### Продвинутая конфигурация (с премиум RPC):
```json
{
  "license": "ВАШ-ЛИЦЕНЗИОННЫЙ-КЛЮЧ",
  "rpc_list": [
    "https://solana-mainnet.g.alchemy.com/v2/YOUR-API-KEY",
    "https://api.mainnet-beta.solana.com"
  ],
  "websocket_url": "wss://solana-mainnet.g.alchemy.com/v2/YOUR-API-KEY",
  "monitor_delay": 500,
  "rpc_delay": 50,
  "price_delay": 500,
  "debug_logging": false,
  "tps_logging": true,
  "retries": 5,
  "webhook_url": "https://discord.com/api/webhooks/YOUR-WEBHOOK",
  "workers": 3
}
```

**Описание параметров:**
- `license` - Ваш лицензионный ключ
- `rpc_list` - Список RPC узлов (первый - основной)
- `websocket_url` - WebSocket для мониторинга
- `monitor_delay` - Задержка обновления мониторинга (мс)
- `rpc_delay` - Задержка между RPC запросами (мс)
- `price_delay` - Задержка обновления цен (мс)
- `debug_logging` - Подробное логирование
- `tps_logging` - Логирование TPS метрик
- `retries` - Количество повторных попыток
- `webhook_url` - URL для уведомлений (опционально)
- `workers` - Количество параллельных воркеров

### 2. wallets.csv - Управление кошельками

#### Формат файла:
```csv
name,private_key
main,ВАШ_ПРИВАТНЫЙ_КЛЮЧ_SOLANA
trading,ДРУГОЙ_ПРИВАТНЫЙ_КЛЮЧ
sniper,ТРЕТИЙ_ПРИВАТНЫЙ_КЛЮЧ
```

#### Примеры:
```csv
name,private_key
main_wallet,5K9bZqkhFWYX3N8kV...
fast_sniper,3XmM8qY7wN9kP2L...
high_volume,7YnL5pQ8mK3jR4T...
```

**⚠️ ВАЖНО:**
- Используйте только base58 формат приватных ключей
- НЕ используйте seed фразы
- Храните файл в безопасном месте
- Никогда не делитесь приватными ключами
- Рекомендуется использовать отдельные кошельки для бота

### 3. tasks.csv - Торговые задачи

#### Формат файла:
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
```

#### Примеры задач:

**Снайпинг нового токена (Pump.fun):**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
pump_snipe,smart,main,snipe,0.1,25.0,0.000005,DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump,250000,50
```

**Покупка токена на Raydium:**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
ray_swap,raydium,trading,swap,0.5,15.0,0.000002,So11111111111111111111111111111111111111112,200000,0
```

**Продажа токенов:**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
sell_all,smart,main,sell,0,10.0,0.000001,YOUR_TOKEN_MINT,200000,100
```

#### Описание параметров:

| Параметр | Описание | Примеры значений |
|----------|----------|------------------|
| `task_name` | Уникальное имя задачи | pump_snipe, quick_buy |
| `module` | DEX модуль | smart, pumpfun, pumpswap, raydium |
| `wallet` | Имя кошелька из wallets.csv | main, trading, sniper |
| `operation` | Тип операции | snipe, swap, sell |
| `amount_sol` | Количество SOL | 0.001-100.0 (0 для sell) |
| `slippage_percent` | Макс. проскальзывание % | 5.0-50.0 |
| `priority_fee` | Приоритет комиссия в SOL | 0.000001-0.01 |
| `token_mint` | Адрес токена | Base58 адрес |
| `compute_units` | Лимит вычислений | 100000-400000 |
| `percent_to_sell` | % для продажи | 0-100 |

#### Рекомендуемые настройки:

**Для новых токенов (высокий риск):**
- `amount_sol`: 0.01-0.1
- `slippage_percent`: 20-30
- `priority_fee`: 0.000005-0.00001
- `compute_units`: 250000-300000

**Для стабильных токенов:**
- `amount_sol`: 0.1-10.0
- `slippage_percent`: 5-15
- `priority_fee`: 0.000001-0.000003
- `compute_units`: 150000-200000

## 🚀 Запуск

### Windows:
```cmd
solana-bot.exe
```

### Linux/macOS:
```bash
./solana-bot
```

## 🎯 Как работает Smart DEX

### Автоматический выбор DEX
При использовании модуля `smart`, бот автоматически определяет оптимальный DEX:

1. **Pump.fun** - для токенов с активной bonding curve
   - Новые токены, еще не завершившие кривую
   - Прямая покупка у протокола
   - Дискретное ценообразование

2. **Pump.swap** - для завершенных токенов
   - Токены, перешедшие на Raydium
   - Торговля через пулы ликвидности
   - Непрерывное ценообразование

3. **Raydium** - прямой доступ к AMM
   - Для любых токенов с пулами
   - Максимальная ликвидность
   - Стандартные свопы

### Приоритет выбора:
1. Проверка Pump.fun bonding curve
2. Проверка Pump.swap пулов
3. Fallback на Raydium

## 📊 Интерфейс мониторинга

После покупки токенов увидите красивый интерфейс:
```
╔════════════════ TOKEN MONITOR ════════════════╗
║ Token: Abcd...1234                            ║
╟───────────────────────────────────────────────╢
║ Current Price:       0.00000123          SOL ║
║ Price Change:        +15.2%                   ║
║ P&L:                 +0.001234 SOL (+15.2%)   ║
╚═══════════════════════════════════════════════╝
```

**Команды:**
- `Enter` - продать токены
- `q` - выйти без продажи

## 🛡️ Безопасность и лучшие практики

### Правила безопасности:
1. **Приватные ключи**
   - ❌ Никогда не делитесь ключами
   - ❌ Не храните в облаке
   - ✅ Используйте отдельные кошельки для бота
   - ✅ Держите минимальные суммы

2. **Конфигурация**
   - ❌ Не коммитьте configs/ в git
   - ✅ Делайте резервные копии локально
   - ✅ Используйте .gitignore

3. **Торговля**
   - ✅ Начинайте с малых сумм (0.001-0.01 SOL)
   - ✅ Тестируйте на известных токенах
   - ✅ Устанавливайте лимиты потерь
   - ✅ Мониторьте транзакции

### Оптимизация производительности:
1. **RPC узлы**
   - Бесплатные: 50-100 TPS, задержки
   - Премиум: 1000+ TPS, низкая задержка
   - Рекомендуемые: Helius, Alchemy, QuickNode

2. **Настройки для скорости**
   ```json
   {
     "rpc_delay": 50,
     "price_delay": 500,
     "workers": 3,
     "retries": 5
   }
   ```

3. **Priority fees для разных сценариев**
   - Обычная торговля: 0.000001-0.000003 SOL
   - Конкурентный снайпинг: 0.000005-0.00001 SOL
   - Высокая нагрузка сети: 0.00001-0.0001 SOL

## 🛠️ Продвинутые настройки

### Множественные задачи
Вы можете запускать несколько задач одновременно:
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
snipe_1,smart,wallet1,snipe,0.1,25.0,0.000005,TOKEN1_MINT,250000,50
snipe_2,smart,wallet2,snipe,0.1,25.0,0.000005,TOKEN2_MINT,250000,50
snipe_3,smart,wallet3,snipe,0.1,25.0,0.000005,TOKEN3_MINT,250000,50
```

### Webhook уведомления
Для получения уведомлений в Discord:
1. Создайте webhook в канале Discord
2. Добавьте URL в config.json:
   ```json
   "webhook_url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"
   ```

### Логирование и отладка
Для подробных логов:
```json
{
  "debug_logging": true,
  "tps_logging": true
}
```

Логи сохраняются в папку `logs/`

## 🔧 Устранение неполадок

### Частые проблемы и решения:

| Проблема | Возможная причина | Решение |
|----------|-------------------|----------|
| "Invalid license" | Неверный ключ | Проверьте лицензионный ключ |
| "RPC error" | Проблемы с узлом | Смените RPC endpoint |
| "Insufficient balance" | Мало SOL | Пополните кошелек |
| "Transaction failed" | Высокий slippage | Увеличьте slippage до 30-50% |
| "Token not found" | Неверный адрес | Проверьте token mint |
| "Timeout" | Медленный RPC | Используйте премиум RPC |

### Диагностика:
1. **Проверка конфигурации:**
   ```bash
   # Linux/macOS
   cat configs/config.json | jq .
   
   # Windows
   type configs\config.json
   ```

2. **Проверка баланса кошелька:**
   - Используйте [Solscan](https://solscan.io)
   - Минимум 0.1 SOL рекомендуется

3. **Проверка токена:**
   - Проверьте адрес на [Solscan](https://solscan.io)
   - Убедитесь что токен активен

## ❓ Часто задаваемые вопросы

**Q: Какой минимальный баланс SOL нужен?**
A: Рекомендуется минимум 0.1 SOL на кошельке для покрытия транзакций и комиссий.

**Q: Можно ли использовать VPN?**
A: Да, но это может увеличить задержку. Выбирайте VPN сервер ближе к RPC узлу.

**Q: Как выбрать правильный slippage?**
A: Для новых токенов 20-30%, для стабильных 5-15%. При неудачах увеличивайте.

**Q: Что такое compute units?**
A: Лимит вычислительных ресурсов для транзакции. Больше = надежнее, но дороже.

**Q: Можно ли торговать несколькими кошельками?**
A: Да, добавьте их в wallets.csv и создайте отдельные задачи для каждого.

**Q: Как остановить бота?**
A: Нажмите Ctrl+C (Cmd+C на macOS) в терминале.

## 📞 Поддержка

- 📧 Email: support@example.com
- 💬 Telegram: @solana_bot_support
- 📚 Документация: [GitHub Wiki](https://github.com/RovshanMuradov/solana-bot/wiki)
- 🐛 Баг-репорты: [GitHub Issues](https://github.com/RovshanMuradov/solana-bot/issues)

---

⚡ **Совет**: Начните с небольших сумм и постепенно увеличивайте по мере освоения!

🚀 Удачной торговли!