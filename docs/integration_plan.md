# 🚨 CRITICAL: Integration Plan for Dead Code

## Executive Summary

All Phase 1-3 refactoring components were built but NEVER integrated. This plan shows how to properly integrate them and remove old code.

---

## 🎯 Integration Priority Order

### Priority 1: Core Safety (MUST DO FIRST)
1. **ShutdownHandler** - Prevent data loss on exit
2. **SafeFileWriter** - Prevent file corruption
3. **LogBuffer** - Prevent log loss

### Priority 2: Monitoring Integration
4. **PriceThrottler** - Prevent UI flooding
5. **GlobalBus & GlobalCache** - Thread-safe communication
6. **TradeHistory** - Audit trail

### Priority 3: Features
7. **AlertManager** - Trading alerts
8. **UIManager** - UI crash recovery
9. **Export & Summary** - Analytics

---

## 📋 PHASE 1: Core Safety Integration (Day 1)

### 1.1 Integrate ShutdownHandler in main.go

**Current Code to Replace:**
```go
// cmd/bot/main.go (CURRENT - BAD)
func main() {
    // ... initialization ...
    
    // Simple shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan
    
    logger.Info("Shutting down...")
}
```

**New Integration:**
```go
// cmd/bot/main.go (NEW - GOOD)
func main() {
    // ... initialization ...
    
    // Initialize shutdown handler
    shutdownHandler := bot.GetShutdownHandler()
    shutdownHandler.SetLogger(logger)
    
    // Register all services for graceful shutdown
    shutdownHandler.RegisterService("logger", loggerCloser{logger})
    shutdownHandler.RegisterService("rpcClient", rpcClient)
    shutdownHandler.RegisterService("taskManager", taskManager)
    shutdownHandler.RegisterService("botService", botService)
    if ui != nil {
        shutdownHandler.RegisterService("ui", ui)
    }
    
    // Start bot
    go func() {
        if err := runner.Run(); err != nil {
            logger.Fatal("Bot failed", zap.Error(err))
        }
    }()
    
    // Wait for shutdown
    shutdownHandler.WaitForShutdown()
}

// Helper wrapper
type loggerCloser struct{ *zap.Logger }
func (l loggerCloser) Close() error {
    return l.Sync()
}
```

### 1.2 Replace File Writers

**Files to Update:**
- `internal/logger/logger.go`
- `internal/monitor/session.go`
- Any place using `os.Create()` or `csv.NewWriter()`

**Current Pattern to Replace:**
```go
// OLD - UNSAFE
file, err := os.Create("trades.csv")
writer := csv.NewWriter(file)
writer.Write(record)
writer.Flush()
```

**New Pattern:**
```go
// NEW - SAFE
writer, err := logger.NewSafeCSVWriter("trades.csv", logger)
if err != nil {
    return err
}
defer writer.Close()

// Register with shutdown handler
bot.GetShutdownHandler().RegisterService("tradeWriter", writer)

// Use it
writer.WriteRecord(record) // Auto-flush, thread-safe
```

### 1.3 Integrate LogBuffer

**Update `internal/logger/logger.go`:**
```go
// Add to CreateLogger function
func CreateLogger(config Config) (*zap.Logger, error) {
    // ... existing code ...
    
    // Initialize global log buffer
    buffer, err := NewLogBuffer(1000, logger) // 1000 entries
    if err != nil {
        return nil, err
    }
    
    // Create buffered writer hook
    hook := zapcore.NewCore(
        zapcore.NewJSONEncoder(cfg.EncoderConfig),
        zapcore.AddSync(buffer),
        cfg.Level,
    )
    
    // Combine with existing cores
    logger = zap.New(zapcore.NewTee(
        core,  // existing core
        hook,  // buffer hook
    ))
    
    // Register buffer for shutdown
    bot.GetShutdownHandler().RegisterService("logBuffer", buffer)
    
    return logger, nil
}
```

### 1.4 Testing Phase 1
```bash
# Test graceful shutdown
go run cmd/bot/main.go
# Press Ctrl+C and verify:
# - "Graceful shutdown initiated" message
# - All services close in order
# - No data loss in files

# Test with race detector
go test -race ./internal/bot -run TestShutdown
go test -race ./internal/logger -run TestSafeWriter
```

---

## 📋 PHASE 2: Monitoring Integration (Day 2)

### 2.1 Initialize GlobalBus and GlobalCache

**Add to main.go after logger creation:**
```go
// Initialize global communication
ui.InitBus(logger)
state.InitCache(logger)

// Register with shutdown
bot.GetShutdownHandler().RegisterService("globalBus", ui.GetGlobalBus())
bot.GetShutdownHandler().RegisterService("globalCache", state.GetGlobalCache())
```

### 2.2 Integrate PriceThrottler in MonitorService

**Update `internal/monitor/service.go`:**
```go
type MonitorService struct {
    // ... existing fields ...
    priceThrottler *PriceThrottler // ADD THIS
}

func NewMonitorService(...) *MonitorService {
    ms := &MonitorService{
        // ... existing initialization ...
    }
    
    // Initialize price throttler
    ms.priceThrottler = NewPriceThrottler(
        150*time.Millisecond,  // O3 recommendation
        ui.GetGlobalBus().GetChannel(), // Use GlobalBus
        logger,
    )
    
    // Start throttler flush goroutine
    go ms.runThrottlerFlush()
    
    return ms
}

func (ms *MonitorService) runThrottlerFlush() {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for range ticker.C {
        ms.priceThrottler.FlushPending()
    }
}
```

**Replace Direct Updates:**
```go
// OLD - In updateTokenPrice()
ms.eventCh <- PriceUpdateEvent{...} // REMOVE THIS

// NEW - Use throttler
ms.priceThrottler.SendPriceUpdate(PriceUpdate{
    TokenMint: position.TokenMint,
    Current:   currentPrice,
    Entry:     position.EntryPrice,
    Percent:   pnlPercent,
})
```

### 2.3 Integrate TradeHistory

**Update `internal/bot/worker.go`:**
```go
// Add to Worker struct
type Worker struct {
    // ... existing fields ...
    tradeHistory *monitor.TradeHistory // ADD THIS
}

// In NewWorker
func NewWorker(...) *Worker {
    // Initialize trade history
    history, err := monitor.NewTradeHistory("./logs", 1000, logger)
    if err != nil {
        logger.Error("Failed to create trade history", zap.Error(err))
        // Continue without history
    }
    
    return &Worker{
        // ... existing fields ...
        tradeHistory: history,
    }
}

// In executeSnipeBuy() - After successful buy
if w.tradeHistory != nil {
    trade := monitor.Trade{
        ID:          fmt.Sprintf("snipe_%d_%d", task.ID, time.Now().Unix()),
        Timestamp:   time.Now(),
        WalletAddr:  wallet.PublicKey.String(),
        TokenMint:   tokenMint,
        TokenSymbol: task.TokenSymbol,
        Action:      "buy",
        AmountSOL:   amountInSol,
        AmountToken: float64(tokenBalance),
        Price:       tokenPrice,
        TxSignature: txSig,
        Success:     true,
        DEX:         task.DEX,
    }
    w.tradeHistory.LogTrade(trade)
}
```

### 2.4 Connect UI to GlobalCache

**Update `internal/ui/screen/monitor.go`:**
```go
// In Update method
case domain.EventPositionUpdate:
    // OLD - Direct update
    // m.updatePosition(msg.Data) // REMOVE
    
    // NEW - Update via GlobalCache
    pos := msg.Data.(MonitoredPosition)
    state.GetGlobalCache().UpdatePosition(state.Position{
        ID:           fmt.Sprintf("pos_%d", pos.ID),
        TokenMint:    pos.TokenMint,
        TokenSymbol:  pos.TokenSymbol,
        EntryPrice:   pos.EntryPrice,
        CurrentPrice: pos.CurrentPrice,
        PnLPercent:   pos.PnLPercent,
        LastUpdate:   pos.LastUpdate,
    })
    
    // Get all positions from cache for display
    m.positions = m.convertCachePositions(state.GetGlobalCache().GetAllPositions())
```

---

## 📋 PHASE 3: Feature Integration (Day 3)

### 3.1 Enable AlertManager

**Update `internal/monitor/session.go`:**
```go
type Session struct {
    // ... existing fields ...
    alertManager *AlertManager // ADD THIS
}

func NewSession(...) *Session {
    // Initialize alert manager
    alertConfig := DefaultAlertConfig()
    alertManager := NewAlertManager(alertConfig, logger)
    
    // Add handlers
    alertManager.AddHandler(func(alert Alert) {
        // Send to UI
        ui.GetGlobalBus().Publish(ui.AlertMessage{
            Type:     string(alert.Type),
            Message:  alert.Message,
            Severity: alert.Severity,
        })
        
        // Log critical alerts
        if alert.Severity == "critical" {
            logger.Error("Critical alert triggered",
                zap.String("type", string(alert.Type)),
                zap.String("message", alert.Message),
            )
        }
    })
    
    return &Session{
        // ... existing fields ...
        alertManager: alertManager,
    }
}

// In price update loop
func (s *Session) monitorPosition(pos *Position) {
    // ... existing price check ...
    
    // Check for alerts
    alerts := s.alertManager.CheckPosition(monitor.Position{
        TokenMint:   pos.TokenMint,
        TokenSymbol: pos.TokenSymbol,
        InitialSOL:  pos.InitialInvestment,
        CurrentSOL:  currentValue,
        PnL:         pnl,
        PnLPercent:  pnlPercent,
        UpdatedAt:   time.Now(),
    })
}
```

### 3.2 Wrap UI with Recovery

**Update `internal/ui/app.go` or where UI is created:**
```go
// OLD - Direct UI creation
program := tea.NewProgram(initialModel)

// NEW - Wrapped with recovery
uiManager := NewUIManager(logger)
program := uiManager.WrapProgram(
    tea.NewProgram(initialModel),
    func() tea.Model {
        return NewInitialModel() // Factory for restart
    },
)

// Register with shutdown
bot.GetShutdownHandler().RegisterService("uiManager", uiManager)
```

### 3.3 Enable Export Functionality

**Add new command to UI or create CLI command:**
```go
// internal/ui/screen/monitor_handlers.go
case "e", "E": // Export
    exporter := export.NewTradeExporter(m.logger)
    
    // Get trades from history
    trades := m.tradeHistory.GetAllTrades()
    
    // Export with current date
    options := export.ExportOptions{
        Format:    export.FormatCSV,
        StartTime: time.Now().Add(-24 * time.Hour),
        EndTime:   time.Now(),
        OutputDir: "./exports",
    }
    
    outputPath, err := exporter.ExportTrades(trades, options)
    if err != nil {
        m.showError(fmt.Sprintf("Export failed: %v", err))
    } else {
        m.showSuccess(fmt.Sprintf("Exported to: %s", outputPath))
    }
```

---

## 🗑️ OLD CODE TO REMOVE

### After Phase 1 Success:
1. Remove simple signal handling in main.go
2. Remove direct `os.Create()` calls
3. Remove manual `csv.Writer` usage
4. Remove direct logger `Sync()` calls

### After Phase 2 Success:
1. Remove direct channel sends for price updates
2. Remove manual position tracking in UI
3. Remove simple price monitoring loops

### After Phase 3 Success:
1. Remove manual alert checking
2. Remove basic UI error handling
3. Remove manual trade logging

---

## ✅ VERIFICATION CHECKLIST

### Phase 1 Verification:
- [ ] Bot shuts down gracefully with status messages
- [ ] All files are properly closed and flushed
- [ ] No data loss during shutdown
- [ ] Race detector passes

### Phase 2 Verification:
- [ ] Price updates are throttled (not flooding UI)
- [ ] Trades appear in history CSV
- [ ] GlobalCache has all active positions
- [ ] No race conditions in monitoring

### Phase 3 Verification:
- [ ] Alerts trigger on price movements
- [ ] UI recovers from crashes
- [ ] Export generates valid files
- [ ] All features work together

---

## 🚨 ROLLBACK PLAN

If integration causes issues:
1. Keep old code in `_deprecated` folders
2. Use feature flags for gradual rollout
3. Test each phase thoroughly before proceeding
4. Have monitoring to detect issues early

```go
// Feature flag example
var enableNewSafety = os.Getenv("ENABLE_NEW_SAFETY") == "true"

if enableNewSafety {
    // Use new code
} else {
    // Use old code
}
```

---

## 📊 SUCCESS METRICS

1. **Zero data loss** during high-volume trading
2. **UI stability** - no freezes or crashes
3. **Alert accuracy** - all conditions detected
4. **Performance** - no degradation vs old code
5. **Maintainability** - easier to debug and extend

---

## 🎯 FINAL NOTES

1. **Test each phase thoroughly** before moving to next
2. **Keep backups** of working version
3. **Monitor logs** for new error patterns
4. **Document any deviations** from this plan
5. **Celebrate** when it's working! 🎉

Total estimated time: 3 days for full integration

---

## 📝 INTEGRATION EXAMPLES

### Example 1: BotService Integration
```go
// internal/bot/service.go - Add safety features
type BotService struct {
    // ... existing fields ...
    tradeHistory *monitor.TradeHistory    // ADD
    alertManager *monitor.AlertManager     // ADD
    priceThrottler *monitor.PriceThrottler // ADD
}

// In NewBotService
func NewBotService(parentCtx context.Context, config *BotServiceConfig) (*BotService, error) {
    // ... existing initialization ...
    
    // Initialize safety components if enabled
    if config.EnableSafety {
        // Trade history
        tradeHistory, err := monitor.NewTradeHistory("./logs", 1000, logger)
        if err != nil {
            logger.Warn("Trade history disabled", zap.Error(err))
        }
        
        // Alert manager
        alertManager := monitor.NewAlertManager(monitor.DefaultAlertConfig(), logger)
        
        // Price throttler for UI
        var outputCh chan tea.Msg
        if ui.GetGlobalBus() != nil {
            outputCh = ui.GetGlobalBus().GetChannel()
        }
        priceThrottler := monitor.NewPriceThrottler(150*time.Millisecond, outputCh, logger)
    }
}
```

### Example 2: Worker Integration
```go
// internal/bot/worker.go - Log trades
func (w *Worker) executeSnipeBuy(ctx context.Context, task *task.Task, wallet *task.Wallet) error {
    // ... existing buy logic ...
    
    // Log successful trade
    if w.botService.tradeHistory != nil && success {
        trade := monitor.Trade{
            ID:          fmt.Sprintf("snipe_%d_%d", task.ID, time.Now().Unix()),
            Timestamp:   time.Now(),
            WalletAddr:  wallet.PublicKey.String(),
            TokenMint:   tokenMint,
            TokenSymbol: task.TokenSymbol,
            Action:      "buy",
            AmountSOL:   amountInSol,
            AmountToken: float64(tokenBalance),
            Price:       tokenPrice,
            TxSignature: txSig,
            Success:     true,
        }
        w.botService.tradeHistory.LogTrade(trade)
    }
}
```

### Example 3: Monitor Session Integration
```go
// internal/monitor/session.go - Use throttler
func (s *Session) updatePositionPrice(pos *Position) {
    // ... calculate new price ...
    
    // OLD: Direct send
    // s.eventBus.Publish(PriceUpdateEvent{...})
    
    // NEW: Through throttler
    if s.priceThrottler != nil {
        s.priceThrottler.SendPriceUpdate(monitor.PriceUpdate{
            TokenMint: pos.TokenMint,
            Current:   currentPrice,
            Entry:     pos.EntryPrice,
            Percent:   pnlPercent,
        })
    }
    
    // Check alerts
    if s.alertManager != nil {
        alerts := s.alertManager.CheckPosition(convertPosition(pos))
        // Handle alerts...
    }
}
```

---

## 🔍 VERIFICATION SCRIPTS

### Check Integration Status
```bash
#!/bin/bash
# check_integration.sh

echo "Checking integration status..."

# Check if new components are imported
echo "=== Checking imports ==="
grep -r "monitor.TradeHistory" internal/bot/
grep -r "logger.SafeFileWriter" internal/
grep -r "bot.GetShutdownHandler" cmd/bot/

# Check if components are initialized
echo "=== Checking initialization ==="
grep -r "NewTradeHistory" internal/
grep -r "InitBus" cmd/bot/
grep -r "InitCache" cmd/bot/

# Check for old patterns that should be replaced
echo "=== Checking for old patterns ==="
grep -r "os.Create" internal/ | grep -v test
grep -r "csv.NewWriter" internal/ | grep -v SafeCSVWriter
grep -r "defer.*Sync()" cmd/bot/main.go

# Count test coverage
echo "=== Test coverage ==="
go test -cover ./internal/monitor/
go test -cover ./internal/logger/
go test -cover ./internal/bot/
```

### Performance Test
```bash
#!/bin/bash
# test_performance.sh

echo "Testing performance with new safety features..."

# Run with old code
ENABLE_NEW_SAFETY=false go run cmd/bot/main.go &
OLD_PID=$!
sleep 30
kill $OLD_PID

# Run with new code
ENABLE_NEW_SAFETY=true go run cmd/bot/main.go &
NEW_PID=$!
sleep 30
kill $NEW_PID

# Compare logs
echo "Check logs for performance metrics"
```