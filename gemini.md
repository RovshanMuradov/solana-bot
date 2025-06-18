

## 🎨 Финальный UI макет (от Gemini + улучшения)

```text
┌────────────────────────────────────────────────────────────────────────────────────────┐
│ 🚀 Solana Bot v1.0 | 💼 SOL...xyz | 🟢 RPC: OK (25ms) | 💰 Total PnL: +0.1234 SOL    │
└────────────────────────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────── 📊 Positions (5) ──────────────────────────────────┐
│ ID   TOKEN         ENTRY (SOL)      CURRENT (SOL)   PNL (%)     PNL (SOL)     STATUS   │
│> 1  Dmig...pump   0.00000010       0.00000003     -70.00% 📉   -0.0000070   Active    │
│  2  CAT...wEXP    0.00150000       0.00183000     +22.00% 📈   +0.0330000   Active    │
│  3  PUMP...qR4    0.01000000       0.01050000     +5.00%  📈   +0.0050000   Selling   │
│  4  WIF...hat     0.02500000       0.00500000     -80.00% 📉   -0.2000000   TP Trigger│
│  5  FOO...bar     0.10000000       0.09800000     -2.00%  📉   -0.0020000   Sold      │
└────────────────────────────────────────────────────────────────────────────────────────┘
┌──────────────────── 🎯 Focus: Dmig...pump (Level 3 Trader) ────────────────────────────┐
│                                                                                        │
│ 📈 Price Trend: -70.00% 📉    ████████▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄                           │
│                                                                                        │
│ 💰 Entry: 0.00000010 SOL       │ 🏦 Invested: 0.01 SOL                               │
│ 🎯 Current: 0.00000003 SOL     │ 💎 Value: 0.003 SOL                                 │
│ 🔢 Tokens: 1,000,000           │ 📊 P&L: -0.007 SOL (-70.00%) 📉                      │
│                                                                                        │
│ ⚡[S]ell Menu  🎯[T]P/SL  📊[M]ore  🚀[1-5] Quick Sell  ⏰Last: 09:26:45             │
└────────────────────────────────────────────────────────────────────────────────────────┘
┌────────────── 📜 Trading Logs (🔥 WARN+ERROR) [L]Toggle [F]ilter ─────────────────────┐
│ 🚨 09:26:45 [ERROR] RPC Failed to get balance DmigFW...pump → Invalid param (Retry #2)│
│ ⚠️  09:16:49 [WARN]  Price Alert: WIF...hat dropped -15% in 5min → Consider SL      │
│ 💰 09:15:30 [SUCCESS] Buy Complete: PUMP...qR4 → 1000 tokens @ 0.01 SOL             │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 🏗️ Техническая архитектура (от O3 + дополнения)

### 1. LogBus система (O3 + структурированность от Gemini)

```go
// internal/ui/logging/log_bus.go
type LogBus struct {
    tuiCh        chan tea.Msg
    fileWriter   *os.File
    tradesWriter *csv.Writer  
    errorWriter  *os.File
    buffer       *RingBuffer
    throttler    *LogThrottler
}

type TradingLogEntry struct {
    Level       LogLevel                 `json:"level"`
    Timestamp   time.Time               `json:"ts"`
    Component   string                  `json:"component"`
    Event       string                  `json:"event"`
    TaskID      string                  `json:"task_id,omitempty"`
    TokenMint   string                  `json:"token,omitempty"`
    Amount      float64                 `json:"amount,omitempty"`
    Price       float64                 `json:"price,omitempty"`
    TxSignature string                  `json:"tx,omitempty"`
    Error       string                  `json:"error,omitempty"`
    Details     map[string]interface{}  `json:"details,omitempty"`
    Icon        string                  // Для TUI отображения
    Color       lipgloss.Color          // Цвет для TUI
}

// Перехват логов без блокировки TUI
type tuiWriter struct {
    ch chan<- tea.Msg
}

func (w tuiWriter) Write(p []byte) (int, error) {
    select {
    case w.ch <- logMsg(p):
    default: // Не блокируем если канал полный
    }
    return len(p), nil
}
```

### 2. Структурированное логирование (от Gemini)

```go
// internal/logging/structured.go
type TradingLogger struct {
    *slog.Logger
    taskID      string
    correlationID string
    logBus      *LogBus
}

// Примеры структурированных событий
func (tl *TradingLogger) LogBuyStart(taskID, token string, amountSol float64) {
    entry := TradingLogEntry{
        Level:     LogInfo,
        Timestamp: time.Now(),
        Component: "trade",
        Event:     "buy_start",
        TaskID:    taskID,
        TokenMint: token,
        Amount:    amountSol,
        Icon:      "🟢",
        Color:     lipgloss.Color("46"),
    }
    tl.logBus.Send(entry)
    
    // JSON в файл
    slog.Info("Trading event",
        slog.String("event", "buy_start"),
        slog.String("task_id", taskID),
        slog.String("token", token),
        slog.Float64("amount_sol", amountSol))
}

func (tl *TradingLogger) LogError(component, event, token, error string) {
    entry := TradingLogEntry{
        Level:     LogError,
        Timestamp: time.Now(),
        Component: component,
        Event:     event,
        TokenMint: token,
        Error:     error,
        Icon:      "🚨",
        Color:     lipgloss.Color("196"),
    }
    tl.logBus.Send(entry)
}
```

### 3. Throttling для real-time обновлений (от O3)

```go
// internal/ui/throttling/price_throttler.go
type PriceThrottler struct {
    updateInterval time.Duration
    lastUpdate     time.Time
    pendingUpdate  *PriceUpdate
    outputCh       chan tea.Msg
}

func (pt *PriceThrottler) SubmitPriceUpdate(update PriceUpdate) {
    pt.pendingUpdate = &update
    
    if time.Since(pt.lastUpdate) >= pt.updateInterval {
        pt.flush()
    }
}

func (pt *PriceThrottler) flush() {
    if pt.pendingUpdate != nil {
        select {
        case pt.outputCh <- priceUpdateMsg(*pt.pendingUpdate):
            pt.lastUpdate = time.Now()
            pt.pendingUpdate = nil
        default:
        }
    }
}

// Запускаем в Init()
func (m rootModel) Init() tea.Cmd {
    return tea.Batch(
        throttledPriceUpdates(150*time.Millisecond), // O3 рекомендация
        setupLogBus(),
        subscribeToEvents(),
    )
}
```

---

## 🎨 UI компоненты с Lipgloss стилизацией

### 1. Header с живой информацией

```go
// internal/ui/components/header.go
type HeaderStyle struct {
    container    lipgloss.Style
    title        lipgloss.Style
    wallet       lipgloss.Style
    rpcGood      lipgloss.Style
    rpcBad       lipgloss.Style
    pnlPositive  lipgloss.Style
    pnlNegative  lipgloss.Style
}

func NewHeaderStyle() HeaderStyle {
    return HeaderStyle{
        container: lipgloss.NewStyle().
            Background(lipgloss.Color("63")).
            Foreground(lipgloss.Color("15")).
            Padding(0, 1).
            Width(90),
            
        title: lipgloss.NewStyle().
            Bold(true).
            Foreground(lipgloss.Color("226")), // Желтый
            
        wallet: lipgloss.NewStyle().
            Foreground(lipgloss.Color("33")), // Синий
            
        rpcGood: lipgloss.NewStyle().
            Foreground(lipgloss.Color("46")), // Зеленый
            
        rpcBad: lipgloss.NewStyle().
            Foreground(lipgloss.Color("196")), // Красный
            
        pnlPositive: lipgloss.NewStyle().
            Foreground(lipgloss.Color("46")).
            Bold(true),
            
        pnlNegative: lipgloss.NewStyle().
            Foreground(lipgloss.Color("196")).
            Bold(true),
    }
}

func (h *Header) Render() string {
    rpcStatus := h.style.rpcGood.Render("🟢 RPC: OK (25ms)")
    if h.rpcLatency > 1000 {
        rpcStatus = h.style.rpcBad.Render("🔴 RPC: SLOW (1200ms)")
    }
    
    pnlStyle := h.style.pnlPositive
    pnlIcon := "💰"
    if h.totalPnL < 0 {
        pnlStyle = h.style.pnlNegative
        pnlIcon = "💸"
    }
    
    content := lipgloss.JoinHorizontal(lipgloss.Left,
        h.style.title.Render("🚀 Solana Bot v1.0"),
        " | ",
        h.style.wallet.Render(fmt.Sprintf("💼 %s", h.walletAddr)),
        " | ",
        rpcStatus,
        " | ",
        pnlStyle.Render(fmt.Sprintf("%s Total PnL: %.4f SOL", pnlIcon, h.totalPnL)),
    )
    
    return h.style.container.Render(content)
}
```

### 2. Positions Table с gaming элементами

```go
// internal/ui/components/positions_table.go
type PositionRow struct {
    ID          int
    TokenSymbol string
    EntryPrice  float64
    CurrentPrice float64
    PnLPercent  float64
    PnLSol      float64
    Status      PositionStatus
    Icon        string // 🔥🐱🚀🎩⭐
    Level       int    // Gaming level на основе профита
}

func (pt *PositionsTable) renderRow(row PositionRow, isSelected bool) string {
    // Gaming level badge
    levelBadge := fmt.Sprintf("Lv%d", row.Level)
    if row.Level >= 10 {
        levelBadge = "🏆" + levelBadge
    }
    
    // PnL с цветом и иконками
    pnlColor := lipgloss.Color("46") // Зеленый
    pnlIcon := "📈"
    if row.PnLPercent < 0 {
        pnlColor = lipgloss.Color("196") // Красный  
        pnlIcon = "📉"
    }
    
    pnlStyle := lipgloss.NewStyle().Foreground(pnlColor).Bold(true)
    
    // Статус с иконками
    statusMap := map[PositionStatus]string{
        StatusActive:    "🟢 Active",
        StatusSelling:   "🟡 Selling", 
        StatusSold:      "⚪ Sold",
        StatusTPTrigger: "🎯 TP Trigger",
        StatusError:     "🔴 Error",
    }
    
    rowStyle := pt.style.normalRow
    if isSelected {
        rowStyle = pt.style.selectedRow
    }
    
    content := fmt.Sprintf("%d  %s%s  %.8f  %.8f  %s  %s  %s",
        row.ID,
        row.Icon,
        row.TokenSymbol,
        row.EntryPrice,
        row.CurrentPrice,
        pnlStyle.Render(fmt.Sprintf("%.2f%% %s", row.PnLPercent, pnlIcon)),
        pnlStyle.Render(fmt.Sprintf("%.7f", row.PnLSol)),
        statusMap[row.Status],
    )
    
    return rowStyle.Render(content)
}
```

### 3. Focus Pane с sparkline (от Gemini + gaming)

```go
// internal/ui/components/focus_pane.go
func (fp *FocusPane) Render() string {
    token := fp.selectedToken
    
    // Gaming level на основе PnL
    level := int(math.Max(1, math.Floor(token.PnLPercent/10)))
    levelBadge := fmt.Sprintf("(Level %d Trader)", level)
    if level >= 10 {
        levelBadge = "(🏆 Master Trader)"
    }
    
    title := fp.style.title.Render(fmt.Sprintf("🎯 Focus: %s %s", token.Symbol, levelBadge))
    
    // Sparkline график (используем простой ASCII)
    sparkline := fp.renderSparkline(token.PriceHistory)
    
    // Price trend с процентом и эмодзи
    trendIcon := "📈"
    trendColor := lipgloss.Color("46")
    if token.PriceChange < 0 {
        trendIcon = "📉"
        trendColor = lipgloss.Color("196")
    }
    
    trendStyle := lipgloss.NewStyle().Foreground(trendColor).Bold(true)
    trendLine := fmt.Sprintf("📈 Price Trend: %s    %s",
        trendStyle.Render(fmt.Sprintf("%.2f%% %s", token.PriceChange, trendIcon)),
        sparkline)
    
    // Левая колонка - цены
    leftCol := lipgloss.JoinVertical(lipgloss.Left,
        fmt.Sprintf("💰 Entry: %.8f SOL", token.EntryPrice),
        fmt.Sprintf("🎯 Current: %.8f SOL", token.CurrentPrice),
        fmt.Sprintf("🔢 Tokens: %s", formatNumber(token.Amount)),
    )
    
    // Правая колонка - инвестиции и PnL
    pnlColor := lipgloss.Color("46")
    pnlIcon := "📊"
    if token.PnL < 0 {
        pnlColor = lipgloss.Color("196")
        pnlIcon = "📉"
    }
    
    rightCol := lipgloss.JoinVertical(lipgloss.Left,
        fmt.Sprintf("🏦 Invested: %.3f SOL", token.Invested),
        fmt.Sprintf("💎 Value: %.3f SOL", token.CurrentValue),
        lipgloss.NewStyle().Foreground(pnlColor).Bold(true).Render(
            fmt.Sprintf("%s P&L: %.6f SOL (%.2f%%)", pnlIcon, token.PnL, token.PnLPercent)),
    )
    
    // Горячие клавиши с иконками
    hotkeys := "⚡[S]ell Menu  🎯[T]P/SL  📊[M]ore  🚀[1-5] Quick Sell  ⏰Last: " + token.LastUpdate.Format("15:04:05")
    
    content := lipgloss.JoinVertical(lipgloss.Left,
        title,
        "",
        trendLine,
        "",
        lipgloss.JoinHorizontal(lipgloss.Top,
            leftCol,
            strings.Repeat(" ", 10),
            rightCol,
        ),
        "",
        fp.style.hotkeys.Render(hotkeys),
    )
    
    return fp.style.container.Render(content)
}

func (fp *FocusPane) renderSparkline(priceHistory []float64) string {
    if len(priceHistory) < 2 {
        return "No data"
    }
    
    // Простой ASCII sparkline
    width := 50
    height := 1
    
    min := slices.Min(priceHistory)
    max := slices.Max(priceHistory)
    if max == min {
        return strings.Repeat("█", width)
    }
    
    var result strings.Builder
    for i := 0; i < width; i++ {
        if i < len(priceHistory) {
            normalized := (priceHistory[i] - min) / (max - min)
            if normalized > 0.75 {
                result.WriteString("█")
            } else if normalized > 0.5 {
                result.WriteString("▆")
            } else if normalized > 0.25 {
                result.WriteString("▄")
            } else {
                result.WriteString("▂")
            }
        } else {
            result.WriteString(" ")
        }
    }
    
    return result.String()
}
```

### 4. Log Viewer с фильтрацией и цветами

```go
// internal/ui/components/log_viewer.go
type LogViewer struct {
    entries     []TradingLogEntry
    filter      LogFilter
    viewport    viewport.Model
    maxEntries  int
    style       LogViewerStyle
}

type LogFilter int
const (
    FilterErrorsOnly LogFilter = iota
    FilterWarningsAndErrors
    FilterAll
    FilterTradingOnly
)

func (lv *LogViewer) renderLogEntry(entry TradingLogEntry) string {
    // Timestamp
    timeStr := entry.Timestamp.Format("15:04:05")
    timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
    
    // Level с цветом и иконкой
    var levelStyle lipgloss.Style
    levelIcon := entry.Icon
    if levelIcon == "" {
        switch entry.Level {
        case LogError:
            levelIcon = "🚨"
            levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
        case LogWarn:
            levelIcon = "⚠️"
            levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
        case LogInfo:
            levelIcon = "ℹ️"
            levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
        case LogSuccess:
            levelIcon = "✅"
            levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
        }
    } else {
        levelStyle = lipgloss.NewStyle().Foreground(entry.Color)
    }
    
    // Компонент
    componentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
    
    // Token (сокращенный)
    tokenStr := ""
    if entry.TokenMint != "" {
        tokenStr = entry.TokenMint[:6] + "..." + entry.TokenMint[len(entry.TokenMint)-4:]
    }
    
    // Сборка сообщения
    message := fmt.Sprintf("%s [%s] %s",
        entry.Event,
        entry.Component,
        tokenStr)
    
    if entry.Error != "" {
        message += " → " + entry.Error
    }
    
    // Детали для торговых операций
    details := ""
    if entry.Amount > 0 {
        details += fmt.Sprintf(" (%.3f SOL)", entry.Amount)
    }
    if entry.Price > 0 {
        details += fmt.Sprintf(" @ %.8f", entry.Price)
    }
    
    return lipgloss.JoinHorizontal(lipgloss.Left,
        timeStyle.Render(timeStr),
        " ",
        levelIcon,
        " ",
        levelStyle.Render(message + details),
    )
}

func (lv *LogViewer) filterEntries() []TradingLogEntry {
    var filtered []TradingLogEntry
    
    for _, entry := range lv.entries {
        switch lv.filter {
        case FilterErrorsOnly:
            if entry.Level == LogError {
                filtered = append(filtered, entry)
            }
        case FilterWarningsAndErrors:
            if entry.Level == LogError || entry.Level == LogWarn {
                filtered = append(filtered, entry)
            }
        case FilterTradingOnly:
            if entry.Component == "trade" || entry.Component == "monitor" {
                filtered = append(filtered, entry)
            }
        case FilterAll:
            filtered = append(filtered, entry)
        }
    }
    
    return filtered
}
```

---

## 🔄 Real-time updates и Event handling

### 1. Unified Event System

```go
// internal/ui/events/trading_events.go
type TradingEventMsg struct {
    Type      string
    Data      interface{}
    Timestamp time.Time
}

// Преобразование внутренних событий в TUI сообщения
func (es *EventSubscriber) OnEvent(event bot.TradingEvent) {
    var msg TradingEventMsg
    
    switch e := event.(type) {
    case bot.PositionCreatedEvent:
        msg = TradingEventMsg{
            Type: "position_created",
            Data: UIPosition{
                ID:          e.TaskID,
                TokenSymbol: extractSymbol(e.TokenMint),
                EntryPrice:  e.EntryPrice,
                Amount:      float64(e.TokenBalance) / 1e6,
                Icon:        getTokenIcon(e.TokenMint),
                Level:       1, // Начальный уровень
            },
            Timestamp: e.Timestamp,
        }
        
        // Логирование
        es.logger.LogBuySuccess(
            fmt.Sprintf("task_%d", e.TaskID),
            e.TokenMint,
            e.AmountSol,
            e.EntryPrice,
            e.TxSignature,
        )
        
    case monitor.PriceUpdateEvent:
        // Throttling через O3 систему
        es.priceThrottler.SubmitPriceUpdate(PriceUpdate{
            TokenMint:    e.TokenMint,
            CurrentPrice: e.CurrentPrice,
            Change:       e.PercentChange,
            Timestamp:    time.Now(),
        })
    }
    
    // Отправляем в TUI
    es.tuiProgram.Send(msg)
}
```

### 2. Model routing и state management

```go
// internal/ui/app.go
type AppModel struct {
    header        *Header
    positionsTable *PositionsTable
    focusPane     *FocusPane
    logViewer     *LogViewer
    
    // State
    selectedPosition int
    viewMode        ViewMode
    logFilter       LogFilter
    
    // Services
    logBus          *LogBus
    eventSubscriber *EventSubscriber
    priceThrottler  *PriceThrottler
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "l", "L":
            // Toggle log filter
            m.logViewer.ToggleFilter()
            
        case "c", "C":
            // Toggle compact mode
            m.positionsTable.ToggleCompactMode()
            
        case "enter":
            // Full-screen focus mode
            m.viewMode = ViewModeFullScreenFocus
            
        case "1", "2", "3", "4", "5":
            // Quick sell percentages
            if m.selectedPosition >= 0 {
                percentage := map[string]float64{
                    "1": 25.0, "2": 50.0, "3": 75.0, "4": 90.0, "5": 100.0,
                }[msg.String()]
                return m, m.sendSellCommand(percentage)
            }
        }
        
    case TradingEventMsg:
        // Route trading events to appropriate components
        switch msg.Type {
        case "position_created":
            m.positionsTable.AddPosition(msg.Data.(UIPosition))
            
        case "price_update":
            update := msg.Data.(PriceUpdate)
            m.positionsTable.UpdatePrice(update)
            m.focusPane.UpdatePrice(update)
            
            // Price alerts
            if math.Abs(update.Change) > 10.0 {
                return m, m.showPriceAlert(update)
            }
        }
        
    case logMsg:
        // Parse и добавляем в log viewer
        var entry TradingLogEntry
        if json.Unmarshal([]byte(msg), &entry) == nil {
            m.logViewer.AddEntry(entry)
        }
    }
    
    // Update sub-components
    m.header, cmd := m.header.Update(msg)
    cmds = append(cmds, cmd)
    
    m.positionsTable, cmd = m.positionsTable.Update(msg)
    cmds = append(cmds, cmd)
    
    m.focusPane, cmd = m.focusPane.Update(msg)
    cmds = append(cmds, cmd)
    
    m.logViewer, cmd = m.logViewer.Update(msg)
    cmds = append(cmds, cmd)
    
    return m, tea.Batch(cmds...)
}

func (m AppModel) View() string {
    switch m.viewMode {
    case ViewModeFullScreenFocus:
        return m.renderFullScreenFocus()
    case ViewModeCompact:
        return m.renderCompactMode()
    default:
        return m.renderMainDashboard()
    }
}

func (m AppModel) renderMainDashboard() string {
    return lipgloss.JoinVertical(lipgloss.Left,
        m.header.Render(),
        m.positionsTable.Render(),
        m.focusPane.Render(),
        m.logViewer.Render(),
    )
}
```

---

## 📁 Файловая структура логов (от Gemini)

```
logs/
├── bot.log              # Основной JSON лог со всеми событиями
├── trades.csv           # CSV для анализа торговли
├── errors.log           # Только ERROR события
└── daily/
    ├── 2024-01-15.log   # Дневные архивы
    └── 2024-01-16.log
```

### CSV структура для анализа:
```csv
timestamp,correlation_id,token,action,amount_sol,amount_tokens,entry_price,current_price,pnl_sol,pnl_percent,tx_signature,status
2024-01-15T09:17:14Z,task_buy_001,DmigFWpu6x...,buy,0.01,1000000,0.00000010,,,,3W5wGc8T...,success
2024-01-15T09:18:30Z,task_buy_001,DmigFWpu6x...,price_update,,,0.00000010,0.00000008,-0.002,-20.0,,
2024-01-15T09:25:45Z,task_buy_001,DmigFWpu6x...,sell,0.008,1000000,0.00000010,0.00000008,-0.002,-20.0,5N2kQv3F...,success
```

---

## 🚀 Поэтапный план реализации

### Неделя 1: Основа архитектуры (O3)
1. **LogBus система** - перехват логов, tuiWriter, каналы
2. **Throttling** - PriceThrottler для плавных обновлений
3. **Model hierarchy** - rootModel, layoutModel, component models
4. **Event routing** - TradingEventMsg, EventSubscriber

### Неделя 2: UI компоненты (Gemini макет)
1. **Header** - статус бар с RPC, wallet, PnL
2. **PositionsTable** - таблица с navigation, выделением
3. **FocusPane** - детальная панель с sparkline
4. **LogViewer** - viewport с фильтрацией

### Неделя 3: Стилизация и features (Claude + gaming)
1. **Lipgloss styles** - цветовое кодирование, иконки, borders
2. **Gaming elements** - levels, badges, progress bars
3. **Animations** - loading spinners, transitions
4. **Hotkeys** - quick sell, mode switching, filters

### Неделя 4: Advanced features
1. **Full-screen modes** - компактный режим, focus mode
2. **Price alerts** - уведомления при резких изменениях
3. **CSV export** - структурированная история торгов
4. **Performance optimization** - throttling, memory management

## 🎯 Результат

✅ **Чистый UI** - логи не мешают основному интерфейсу  
✅ **Real-time monitoring** - плавные обновления без лагов  
✅ **Production logs** - структурированные, фильтруемые, экспортируемые  
✅ **Gaming UX** - levels, badges, иконки для engagement  
✅ **Scalable architecture** - легко добавлять новые компоненты  
✅ **Professional look** - цветовое кодирование, современный дизайн

Финальный план объединяет лучшие идеи всех трех источников для создания production-ready TUI!