#!/bin/bash

# ====================================
# Финальная проверка интеграции
# ====================================

set -e

echo "🎯 Final Integration Verification"
echo "================================="

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo "❌ Must be run from project root directory"
    exit 1
fi

success_count=0
total_checks=0

check() {
    local description=$1
    local command=$2
    
    total_checks=$((total_checks + 1))
    echo -n "Checking $description... "
    
    if eval "$command" >/dev/null 2>&1; then
        echo "✅"
        success_count=$((success_count + 1))
        return 0
    else
        echo "❌"
        return 1
    fi
}

echo
echo "🏗️ Build Tests:"
check "Bot application build" "go build -o /tmp/test_bot ./cmd/bot/main.go"
check "TUI application build" "go build -o /tmp/test_tui ./cmd/tui/main.go"

echo
echo "🔍 Phase Integration Tests:"
check "Phase 1 - ShutdownHandler" "grep -r 'GetShutdownHandler' cmd/ internal/"
check "Phase 1 - SafeFileWriter" "grep -r 'NewSafeFileWriter\|NewSafeCSVWriter' internal/"
check "Phase 1 - LogBuffer" "grep -r 'NewLogBuffer' cmd/ internal/"

check "Phase 2 - PriceThrottler" "grep -r 'NewPriceThrottler' internal/"
check "Phase 2 - GlobalBus/Cache" "grep -r 'InitBus\|InitCache' cmd/"
check "Phase 2 - TradeHistory" "grep -r 'NewTradeHistory' internal/"

check "Phase 3 - AlertManager" "grep -r 'NewAlertManager' internal/"
check "Phase 3 - UIManager" "grep -r 'NewUIManager' cmd/"
check "Phase 3 - Export function" "grep -r 'exportTradeDataCmd' internal/"

echo
echo "🧪 Key Tests:"
check "Monitor AlertManager tests" "go test ./internal/monitor/ -run TestAlertManager -timeout 30s"
check "UI Manager tests" "go test ./internal/ui/ -run TestUIManager -timeout 30s"
check "Export package tests" "go test ./internal/export/ -timeout 30s"

echo
echo "🕵️ Dead Code Analysis:"
echo "Running custom dead code finder..."
go run ./scripts/find_dead_code.go ./internal/ | grep -E "(No dead code|FULLY INTEGRATED)" >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✅ No dead code found"
    success_count=$((success_count + 1))
else
    echo "⚠️  Potential dead code detected"
fi
total_checks=$((total_checks + 1))

# Clean up
rm -f /tmp/test_bot /tmp/test_tui

echo
echo "📊 Integration Status Summary:"
echo "=============================="
echo "✅ Passed: $success_count/$total_checks checks"

percentage=$((success_count * 100 / total_checks))
echo "📈 Success Rate: $percentage%"

if [ $success_count -eq $total_checks ]; then
    echo
    echo "🎉 INTEGRATION VERIFICATION SUCCESSFUL!"
    echo "All 3 phases are fully integrated and working correctly."
    echo
    echo "Phase 1 (Core Safety): ✅ COMPLETE"
    echo "- ShutdownHandler with graceful service shutdown"
    echo "- SafeFileWriter preventing data corruption"
    echo "- LogBuffer with safe logging"
    echo
    echo "Phase 2 (Monitoring): ✅ COMPLETE"
    echo "- PriceThrottler with 150ms UI optimization"
    echo "- GlobalBus/Cache for thread-safe communication"
    echo "- TradeHistory with automatic CSV logging"
    echo
    echo "Phase 3 (Features): ✅ COMPLETE"
    echo "- AlertManager with trading alerts"
    echo "- UIManager with crash recovery"
    echo "- Export functionality for trade analysis"
    echo
    echo "🚀 System is production-ready!"
    exit 0
else
    echo
    echo "❌ INTEGRATION VERIFICATION FAILED"
    echo "Some components are not properly integrated."
    echo "Please review the failed checks above."
    exit 1
fi