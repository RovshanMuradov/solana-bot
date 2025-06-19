#!/bin/bash

# ====================================
# Integration and Dead Code Checker
# ====================================
# Этот скрипт проверяет успешность интеграции Phase 1-3 
# и находит неиспользуемый код

set -e

echo "🔍 Starting Integration and Dead Code Analysis..."
echo "================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
ERRORS=0
WARNINGS=0
SUCCESSES=0

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "SUCCESS") echo -e "${GREEN}✅ ${message}${NC}" && ((SUCCESSES++)) ;;
        "WARNING") echo -e "${YELLOW}⚠️  ${message}${NC}" && ((WARNINGS++)) ;;
        "ERROR") echo -e "${RED}❌ ${message}${NC}" && ((ERRORS++)) ;;
        "INFO") echo -e "${BLUE}ℹ️  ${message}${NC}" ;;
    esac
}

# Function to check if pattern exists in files
check_integration_pattern() {
    local pattern=$1
    local description=$2
    local files=$3
    
    if grep -r "$pattern" $files >/dev/null 2>&1; then
        print_status "SUCCESS" "$description found"
        return 0
    else
        print_status "ERROR" "$description NOT found"
        return 1
    fi
}

# Function to check for dead code patterns
check_dead_code_pattern() {
    local pattern=$1
    local description=$2
    local files=$3
    
    local matches=$(grep -r "$pattern" $files 2>/dev/null | grep -v "_test.go" | grep -v "//.*$pattern" | wc -l)
    if [ "$matches" -gt 0 ]; then
        print_status "WARNING" "$description found ($matches occurrences) - possible dead code"
        grep -r "$pattern" $files 2>/dev/null | grep -v "_test.go" | grep -v "//.*$pattern" | head -5
        return 1
    else
        print_status "SUCCESS" "$description not found - good"
        return 0
    fi
}

echo
echo "📋 PHASE 1: Core Safety Integration Check"
echo "=========================================="

# Check Phase 1 integrations
check_integration_pattern "GetShutdownHandler" "ShutdownHandler usage" "cmd/ internal/"
check_integration_pattern "NewSafeFileWriter\|NewSafeCSVWriter" "SafeFileWriter usage" "internal/"
check_integration_pattern "NewLogBuffer" "LogBuffer integration" "cmd/ internal/"
check_integration_pattern "RegisterService.*shutdown" "Service registration for shutdown" "cmd/ internal/"

echo
echo "📋 PHASE 2: Monitoring Integration Check"
echo "========================================"

# Check Phase 2 integrations  
check_integration_pattern "NewPriceThrottler" "PriceThrottler initialization" "internal/"
check_integration_pattern "InitBus.*InitCache" "GlobalBus and GlobalCache initialization" "cmd/"
check_integration_pattern "NewTradeHistory" "TradeHistory integration" "internal/"
check_integration_pattern "SendPriceUpdate" "PriceThrottler usage" "internal/"
check_integration_pattern "GlobalCache.*SetPosition\|UpdatePosition" "GlobalCache position updates" "internal/"

echo
echo "📋 PHASE 3: Feature Integration Check" 
echo "====================================="

# Check Phase 3 integrations
check_integration_pattern "NewAlertManager" "AlertManager initialization" "internal/"
check_integration_pattern "NewUIManager" "UIManager integration" "cmd/"
check_integration_pattern "CheckPosition.*alert" "AlertManager position checking" "internal/"
check_integration_pattern "exportTradeDataCmd\|Export.*trade" "Export functionality" "internal/"

echo
echo "🕵️ DEAD CODE DETECTION"
echo "======================"

# Check for potentially dead code patterns
check_dead_code_pattern "func.*unused\|var.*unused" "Functions/variables with 'unused' in name" "internal/"
check_dead_code_pattern "TODO.*remove\|FIXME.*delete\|DEPRECATED" "Code marked for removal" "internal/ cmd/"

# Check for old patterns that should be replaced
echo
echo "🔄 OLD PATTERN DETECTION"
echo "========================"

check_dead_code_pattern "os\.Create.*csv\|csv\.NewWriter" "Direct CSV writing (should use SafeCSVWriter)" "internal/"
check_dead_code_pattern "signal\.Notify.*SIGINT.*SIGTERM" "Manual signal handling (should use ShutdownHandler)" "cmd/"
check_dead_code_pattern "defer.*Sync()" "Manual logger sync (should use shutdown handler)" "cmd/"

echo
echo "🧪 RUNNING ENHANCED GOLANGCI-LINT"
echo "================================="

# Run golangci-lint with enhanced configuration
if command -v golangci-lint >/dev/null 2>&1; then
    print_status "INFO" "Running golangci-lint with dead code detection..."
    
    # Run linter and capture output
    if golangci-lint run --config .golangci.yml ./... 2>&1 | tee /tmp/lint_output.txt; then
        print_status "SUCCESS" "golangci-lint completed successfully"
    else
        print_status "WARNING" "golangci-lint found issues"
        
        # Check for specific dead code issues
        if grep -E "(unused|deadcode|varcheck|structcheck)" /tmp/lint_output.txt >/dev/null; then
            print_status "WARNING" "Dead code detected by linter:"
            grep -E "(unused|deadcode|varcheck|structcheck)" /tmp/lint_output.txt | head -10
        fi
    fi
else
    print_status "WARNING" "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
fi

echo
echo "🔍 UNUSED IMPORT DETECTION"
echo "=========================="

# Check for unused imports using go mod tidy
print_status "INFO" "Checking for unused dependencies..."
go mod tidy
if git diff --exit-code go.mod go.sum >/dev/null 2>&1; then
    print_status "SUCCESS" "No unused dependencies found"
else
    print_status "WARNING" "go.mod/go.sum changed - there were unused dependencies"
    git diff go.mod go.sum
fi

echo
echo "🏗️ BUILD VERIFICATION"
echo "===================="

# Test that everything builds
print_status "INFO" "Testing build of main applications..."

if go build -o /tmp/bot_test ./cmd/bot/main.go >/dev/null 2>&1; then
    print_status "SUCCESS" "Bot application builds successfully"
else
    print_status "ERROR" "Bot application build failed"
fi

if go build -o /tmp/tui_test ./cmd/tui/main.go >/dev/null 2>&1; then
    print_status "SUCCESS" "TUI application builds successfully" 
else
    print_status "ERROR" "TUI application build failed"
fi

# Clean up test binaries
rm -f /tmp/bot_test /tmp/tui_test /tmp/lint_output.txt

echo
echo "📊 INTEGRATION VERIFICATION SUMMARY"
echo "===================================="

# Check integration completeness based on specific markers
echo "Phase Integration Status:"

# Phase 1 verification
phase1_markers=("GetShutdownHandler" "NewSafeFileWriter" "NewLogBuffer")
phase1_complete=true
for marker in "${phase1_markers[@]}"; do
    if ! grep -r "$marker" cmd/ internal/ >/dev/null 2>&1; then
        phase1_complete=false
        break
    fi
done

if $phase1_complete; then
    print_status "SUCCESS" "Phase 1 (Core Safety) - FULLY INTEGRATED"
else
    print_status "ERROR" "Phase 1 (Core Safety) - INCOMPLETE"
fi

# Phase 2 verification  
phase2_markers=("NewPriceThrottler" "InitBus" "NewTradeHistory")
phase2_complete=true
for marker in "${phase2_markers[@]}"; do
    if ! grep -r "$marker" cmd/ internal/ >/dev/null 2>&1; then
        phase2_complete=false
        break
    fi
done

if $phase2_complete; then
    print_status "SUCCESS" "Phase 2 (Monitoring) - FULLY INTEGRATED"
else
    print_status "ERROR" "Phase 2 (Monitoring) - INCOMPLETE"
fi

# Phase 3 verification
phase3_markers=("NewAlertManager" "NewUIManager" "exportTradeDataCmd")
phase3_complete=true
for marker in "${phase3_markers[@]}"; do
    if ! grep -r "$marker" cmd/ internal/ >/dev/null 2>&1; then
        phase3_complete=false
        break
    fi
done

if $phase3_complete; then
    print_status "SUCCESS" "Phase 3 (Features) - FULLY INTEGRATED"
else
    print_status "ERROR" "Phase 3 (Features) - INCOMPLETE"
fi

echo
echo "📈 FINAL SUMMARY"
echo "================"
echo -e "${GREEN}✅ Successes: $SUCCESSES${NC}"
echo -e "${YELLOW}⚠️  Warnings:  $WARNINGS${NC}"
echo -e "${RED}❌ Errors:    $ERRORS${NC}"
echo

if [ $ERRORS -eq 0 ]; then
    if [ $WARNINGS -eq 0 ]; then
        print_status "SUCCESS" "Integration verification PASSED - No issues found!"
        exit 0
    else
        print_status "WARNING" "Integration verification PASSED with warnings - Review recommended"
        exit 0
    fi
else
    print_status "ERROR" "Integration verification FAILED - Issues must be fixed"
    exit 1
fi