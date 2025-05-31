# 🤖 Solana Trading Bot - Инструкция по настройке

## 📁 Что вам нужно

1. **Исполняемый файл**: `solana-bot` (или `solana-bot.exe` для Windows)
2. **Папка configs/** с тремя файлами:
   - `config.json` - основные настройки
   - `wallets.csv` - ваши кошельки
   - `tasks.csv` - торговые задачи

## ⚙️ Настройка конфигурации

### 1. config.json
Замените `YOUR-LICENSE-KEY-HERE` на ваш лицензионный ключ:

```json
{
  "license": "ВАШ-ЛИЦЕНЗИОННЫЙ-КЛЮЧ",
  "rpc_list": ["https://api.mainnet-beta.solana.com"],
  "websocket_url": "wss://api.mainnet-beta.solana.com",
  "monitor_delay": 1000,
  "rpc_delay": 100, 
  "price_delay": 1000,
  "debug_logging": false,
  "retries": 3,
  "workers": 1
}
```

### 2. wallets.csv
Добавьте ваши приватные ключи кошельков:

```csv
name,private_key
main,ВАШ_ПРИВАТНЫЙ_КЛЮЧ_SOLANA
trading,ДРУГОЙ_ПРИВАТНЫЙ_КЛЮЧ
```

### 3. tasks.csv
Настройте торговые задачи:

```csv
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
my_trade,snipe,main,snipe,0.005,20.0,default,АДРЕС_ТОКЕНА,200000,50
```

**Параметры:**
- `task_name` - название задачи
- `module` - используйте `snipe` для автовыбора DEX
- `wallet` - имя кошелька из wallets.csv  
- `operation` - оставьте `snipe`
- `amount_sol` - количество SOL для покупки (например: 0.001)
- `slippage_percent` - максимальное проскальзывание (10-30%)
- `priority_fee` - комиссия (`default` или число)
- `token_mint` - адрес токена для торговли
- `compute_units` - лимит вычислений (150000-300000)
- `percent_to_sell` - процент для автопродажи (1-100)

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

Бот автоматически выбирает между Pump.fun и Pump.swap:
- **Pump.fun** - для новых токенов (активная bonding curve)
- **Pump.swap** - для завершенных токенов (pool на Raydium)

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

## ⚠️ Важные моменты

1. **Безопасность**: Никогда не делитесь приватными ключами
2. **Тестирование**: Начните с малых сумм (0.001-0.01 SOL)
3. **RPC**: Используйте качественные RPC-ноды для быстрых транзакций
4. **Slippage**: Для волатильных токенов ставьте 20-30%

## 🆘 Помощь

При проблемах проверьте:
- ✅ Правильность лицензионного ключа
- ✅ Валидность приватных ключей
- ✅ Наличие SOL на кошельке
- ✅ Корректность адреса токена

Удачных торгов! 🚀