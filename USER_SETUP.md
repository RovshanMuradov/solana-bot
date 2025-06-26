# 🤖 Solana Trading Bot - User Guide

[🇺🇸 English](#english) | [🇷🇺 Русский](#русский)

---

## English

## 📋 Table of Contents
- [What You Need](#-what-you-need)
- [Initial Setup](#-initial-setup)
- [Configuration](#️-configuration)
- [Running the Bot](#-launch)
- [Monitoring and Management](#-monitoring-interface)
- [Advanced Settings](#-advanced-settings)
- [Troubleshooting](#-troubleshooting)
- [FAQ](#-frequently-asked-questions)

## 📁 What You Need

### Minimum Requirements:
- ✅ **Operating System**: Windows 10+, macOS 10.14+, Linux (Ubuntu 20.04+)
- ✅ **Executable File**: `solana-bot` (or `solana-bot.exe` for Windows)
- ✅ **License Key**: Obtain from developer
- ✅ **Solana Wallet**: With at least 0.1 SOL for transactions
- ✅ **Internet**: Stable connection

### File Structure:
```
solana-bot/
├── solana-bot           # Executable file
└── configs/            # Configuration folder
    ├── config.json     # Main settings
    ├── wallets.csv     # Your wallets  
    └── tasks.csv       # Trading tasks
```

## 🚀 Initial Setup

### Step 1: Create Structure
```bash
# Create folder for the bot
mkdir solana-bot
cd solana-bot

# Create configuration folder
mkdir configs
```

### Step 2: Place Executable File
Copy `solana-bot` (or `solana-bot.exe`) to the created folder.

## ⚙️ Configuration

### 1. config.json - Main Settings

#### Basic Configuration:
```json
{
  "license": "YOUR-LICENSE-KEY",
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

#### Advanced Configuration (with premium RPC):
```json
{
  "license": "YOUR-LICENSE-KEY",
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

**Parameter Descriptions:**
- `license` - Your license key
- `rpc_list` - List of RPC nodes (first one is primary)
- `websocket_url` - WebSocket for monitoring
- `monitor_delay` - Monitoring update delay (ms)
- `rpc_delay` - Delay between RPC requests (ms)
- `price_delay` - Price update delay (ms)
- `debug_logging` - Detailed logging
- `tps_logging` - TPS metrics logging
- `retries` - Number of retry attempts
- `webhook_url` - URL for notifications (optional)
- `workers` - Number of parallel workers

### 2. wallets.csv - Wallet Management

#### File Format:
```csv
name,private_key
main,YOUR_SOLANA_PRIVATE_KEY
trading,ANOTHER_PRIVATE_KEY
sniper,THIRD_PRIVATE_KEY
```

#### Examples:
```csv
name,private_key
main_wallet,5K9bZqkhFWYX3N8kV...
fast_sniper,3XmM8qY7wN9kP2L...
high_volume,7YnL5pQ8mK3jR4T...
```

**⚠️ IMPORTANT:**
- Use only base58 format private keys
- DO NOT use seed phrases
- Store file in a secure location
- Never share private keys
- Recommended to use separate wallets for the bot

### 3. tasks.csv - Trading Tasks

#### File Format:
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
```

#### Task Examples:

**Sniping New Token (Pump.fun):**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
pump_snipe,smart,main,snipe,0.1,25.0,0.000005,DmigFWPu6xFSntkBqWAm5MqTFDrC1ZtFiJj8ir74pump,250000,50
```

**Buying Token on Raydium:**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
ray_swap,raydium,trading,swap,0.5,15.0,0.000002,So11111111111111111111111111111111111111112,200000,0
```

**Selling Tokens:**
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
sell_all,smart,main,sell,0,10.0,0.000001,YOUR_TOKEN_MINT,200000,100
```

#### Parameter Descriptions:

| Parameter | Description | Example Values |
|-----------|-------------|----------------|
| `task_name` | Unique task name | pump_snipe, quick_buy |
| `module` | DEX module | smart, pumpfun, pumpswap, raydium |
| `wallet` | Wallet name from wallets.csv | main, trading, sniper |
| `operation` | Operation type | snipe, swap, sell |
| `amount_sol` | SOL amount | 0.001-100.0 (0 for sell) |
| `slippage_percent` | Max slippage % | 5.0-50.0 |
| `priority_fee` | Priority fee in SOL | 0.000001-0.01 |
| `token_mint` | Token address | Base58 address |
| `compute_units` | Compute limit | 100000-400000 |
| `percent_to_sell` | % to sell | 0-100 |

#### Recommended Settings:

**For New Tokens (High Risk):**
- `amount_sol`: 0.01-0.1
- `slippage_percent`: 20-30
- `priority_fee`: 0.000005-0.00001
- `compute_units`: 250000-300000

**For Stable Tokens:**
- `amount_sol`: 0.1-10.0
- `slippage_percent`: 5-15
- `priority_fee`: 0.000001-0.000003
- `compute_units`: 150000-200000

## 🚀 Launch

### Windows:
```cmd
solana-bot.exe
```

### Linux/macOS:
```bash
./solana-bot
```

## 🎯 How Smart DEX Works

### Automatic DEX Selection
When using the `smart` module, the bot automatically determines the optimal DEX:

1. **Pump.fun** - for tokens with active bonding curve
   - New tokens that haven't completed the curve yet
   - Direct purchase from protocol
   - Discrete pricing

2. **Pump.swap** - for completed tokens
   - Tokens that moved to Raydium
   - Trading through liquidity pools
   - Continuous pricing

3. **Raydium** - direct AMM access
   - For any tokens with pools
   - Maximum liquidity
   - Standard swaps

### Selection Priority:
1. Check Pump.fun bonding curve
2. Check Pump.swap pools
3. Fallback to Raydium

## 📊 Monitoring Interface

After purchasing tokens you'll see a beautiful interface:
```
╔════════════════ TOKEN MONITOR ════════════════╗
║ Token: Abcd...1234                            ║
╟───────────────────────────────────────────────╢
║ Current Price:       0.00000123          SOL ║
║ Price Change:        +15.2%                   ║
║ P&L:                 +0.001234 SOL (+15.2%)   ║
╚═══════════════════════════════════════════════╝
```

**Commands:**
- `Enter` - sell tokens
- `q` - exit without selling

## 🛡️ Security and Best Practices

### Security Rules:
1. **Private Keys**
   - ❌ Never share keys
   - ❌ Don't store in cloud
   - ✅ Use separate wallets for bot
   - ✅ Keep minimal amounts

2. **Configuration**
   - ❌ Don't commit configs/ to git
   - ✅ Make local backups
   - ✅ Use .gitignore

3. **Trading**
   - ✅ Start with small amounts (0.001-0.01 SOL)
   - ✅ Test on known tokens
   - ✅ Set loss limits
   - ✅ Monitor transactions

### Performance Optimization:
1. **RPC Nodes**
   - Free: 50-100 TPS, delays
   - Premium: 1000+ TPS, low latency
   - Recommended: Helius, Alchemy, QuickNode

2. **Speed Settings**
   ```json
   {
     "rpc_delay": 50,
     "price_delay": 500,
     "workers": 3,
     "retries": 5
   }
   ```

3. **Priority Fees for Different Scenarios**
   - Regular trading: 0.000001-0.000003 SOL
   - Competitive sniping: 0.000005-0.00001 SOL
   - High network load: 0.00001-0.0001 SOL

## 🛠️ Advanced Settings

### Multiple Tasks
You can run multiple tasks simultaneously:
```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
snipe_1,smart,wallet1,snipe,0.1,25.0,0.000005,TOKEN1_MINT,250000,50
snipe_2,smart,wallet2,snipe,0.1,25.0,0.000005,TOKEN2_MINT,250000,50
snipe_3,smart,wallet3,snipe,0.1,25.0,0.000005,TOKEN3_MINT,250000,50
```

### Webhook Notifications
For Discord notifications:
1. Create webhook in Discord channel
2. Add URL to config.json:
   ```json
   "webhook_url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_URL"
   ```

### Logging and Debugging
For detailed logs:
```json
{
  "debug_logging": true,
  "tps_logging": true
}
```

Logs are saved to `logs/` folder

## 🔧 Troubleshooting

### Common Problems and Solutions:

| Problem | Possible Cause | Solution |
|---------|---------------|----------|
| "Invalid license" | Wrong key | Check license key |
| "RPC error" | Node issues | Change RPC endpoint |
| "Insufficient balance" | Low SOL | Fund wallet |
| "Transaction failed" | High slippage | Increase slippage to 30-50% |
| "Token not found" | Wrong address | Check token mint |
| "Timeout" | Slow RPC | Use premium RPC |

### Diagnostics:
1. **Check Configuration:**
   ```bash
   # Linux/macOS
   cat configs/config.json | jq .
   
   # Windows
   type configs\config.json
   ```

2. **Check Wallet Balance:**
   - Use [Solscan](https://solscan.io)
   - Minimum 0.1 SOL recommended

3. **Check Token:**
   - Verify address on [Solscan](https://solscan.io)
   - Ensure token is active

## ❓ Frequently Asked Questions

**Q: What minimum SOL balance is needed?**
A: Minimum 0.1 SOL recommended per wallet for transactions and fees.

**Q: Can I use VPN?**
A: Yes, but it may increase latency. Choose VPN server closer to RPC node.

**Q: How to choose correct slippage?**
A: For new tokens 20-30%, for stable tokens 5-15%. Increase if transactions fail.

**Q: What are compute units?**
A: Computational resource limit for transaction. More = more reliable but more expensive.

**Q: Can I trade with multiple wallets?**
A: Yes, add them to wallets.csv and create separate tasks for each.

**Q: How to stop the bot?**
A: Press Ctrl+C (Cmd+C on macOS) in terminal.

## 📞 Support

- 📧 Email: support@example.com
- 💬 Telegram: @solana_bot_support
- 📚 Documentation: [GitHub Wiki](https://github.com/RovshanMuradov/solana-bot/wiki)
- 🐛 Bug Reports: [GitHub Issues](https://github.com/RovshanMuradov/solana-bot/issues)

---

⚡ **Tip**: Start with small amounts and gradually increase as you learn!

🚀 Happy Trading!

---

## Русский

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