#!/bin/bash

# Solana Bot Build Script
# Usage: ./build.sh [clean|dist|dev]

set -e

PROJECT_NAME="solana-bot"
VERSION=$(date +%Y%m%d-%H%M%S)
DIST_DIR="distribution"

echo "🚀 Building Solana Trading Bot..."

# Clean function
clean() {
    echo "🧹 Cleaning old builds..."
    rm -f ${PROJECT_NAME}-*
    rm -rf ${DIST_DIR}
    echo "✅ Clean completed"
}

# Development build (current platform only)
dev_build() {
    echo "🔨 Building for development (current platform)..."
    go build -ldflags "-X main.version=${VERSION}" -o ${PROJECT_NAME} cmd/bot/main.go
    echo "✅ Development build completed: ${PROJECT_NAME}"
}

# Distribution build (all platforms)
dist_build() {
    echo "📦 Building distribution for all platforms..."
    
    # Create distribution directory
    mkdir -p ${DIST_DIR}/configs
    
    # Build for different platforms
    echo "Building for Linux x64..."
    GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=${VERSION}" -o ${DIST_DIR}/${PROJECT_NAME}-linux cmd/bot/main.go
    
    echo "Building for Windows x64..."
    GOOS=windows GOARCH=amd64 go build -ldflags "-X main.version=${VERSION}" -o ${DIST_DIR}/${PROJECT_NAME}-windows.exe cmd/bot/main.go
    
    echo "Building for macOS x64..."
    GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=${VERSION}" -o ${DIST_DIR}/${PROJECT_NAME}-macos cmd/bot/main.go
    
    # Copy configuration files
    echo "📋 Copying configuration files..."
    cp -r configs/config.json ${DIST_DIR}/configs/ 2>/dev/null || echo "Using default config"
    
    # Create example configs if they don't exist
    if [ ! -f "${DIST_DIR}/configs/config.json" ]; then
        cat > ${DIST_DIR}/configs/config.json << 'EOF'
{
  "license": "YOUR_LICENSE_KEY_HERE",
  
  "rpc_list": [
    "https://api.mainnet-beta.solana.com",
    "https://solana-api.projectserum.com"
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
EOF
    fi
    
    cat > ${DIST_DIR}/configs/wallets.csv << 'EOF'
name,private_key
main,YOUR_PRIVATE_KEY_HERE
EOF
    
    cat > ${DIST_DIR}/configs/tasks.csv << 'EOF'
task_name,module,wallet,operation,amount_sol,slippage_percent,priority_fee,token_mint,compute_units,percent_to_sell
example_task,snipe,main,snipe,0.001,20.0,0.000001,YOUR_TOKEN_MINT_HERE,200000,25
EOF
    
    # Create README
    cat > ${DIST_DIR}/README.md << 'EOF'
# Solana Trading Bot - Setup Guide

## Quick Start

1. **Get your license key** from the bot owner
2. **Configure your settings:**
   - Edit `configs/config.json` - add your license key
   - Edit `configs/wallets.csv` - add your wallet private key
   - Edit `configs/tasks.csv` - configure your trading tasks

3. **Run the bot:**
   - Linux/macOS: `./solana-bot-linux` or `./solana-bot-macos`
   - Windows: `solana-bot-windows.exe`

## Configuration Files

### configs/config.json
- `license`: Your license key (required)
- `rpc_list`: Solana RPC endpoints (you can use free public ones)
- `workers`: Number of parallel workers (keep at 1 for safety)

### configs/wallets.csv
- Add your wallet private key
- Format: `name,private_key`

### configs/tasks.csv
Trading task configuration:
- `module`: Use `snipe` for automatic DEX selection
- `amount_sol`: Amount to invest in SOL
- `slippage_percent`: Slippage tolerance (10-50%)
- `percent_to_sell`: Percentage to sell when you press Enter (1-99%)

## Important Notes

- Keep your private keys secure
- Start with small amounts to test
- Monitor the bot while it's running
- Press Enter during monitoring to sell tokens

## Support

Contact the bot owner for license keys and technical support.
EOF
    
    # Create archive
    echo "📦 Creating distribution archive..."
    tar -czf ${PROJECT_NAME}-distribution-${VERSION}.tar.gz ${DIST_DIR}/
    
    echo "✅ Distribution build completed!"
    echo "📁 Files created:"
    echo "   - ${DIST_DIR}/${PROJECT_NAME}-linux"
    echo "   - ${DIST_DIR}/${PROJECT_NAME}-windows.exe"
    echo "   - ${DIST_DIR}/${PROJECT_NAME}-macos"
    echo "   - ${PROJECT_NAME}-distribution-${VERSION}.tar.gz"
}

# Run linter and tests
check() {
    echo "🔍 Running code checks..."
    
    echo "Running go fmt..."
    go fmt ./...
    
    echo "Running go vet..."
    go vet ./...
    
    if command -v golangci-lint >/dev/null 2>&1; then
        echo "Running golangci-lint..."
        golangci-lint run
    else
        echo "⚠️  golangci-lint not found, skipping"
    fi
    
    echo "Running tests..."
    go test ./... -v
    
    echo "✅ Code checks completed"
}

# Main script logic
case "${1:-dev}" in
    "clean")
        clean
        ;;
    "dev")
        check
        dev_build
        ;;
    "dist")
        check
        clean
        dist_build
        ;;
    "check")
        check
        ;;
    *)
        echo "Usage: $0 [clean|dev|dist|check]"
        echo "  clean - Remove old builds"
        echo "  dev   - Build for current platform (default)"
        echo "  dist  - Build distribution for all platforms"
        echo "  check - Run linter and tests only"
        exit 1
        ;;
esac

echo "🎉 Build script completed!"