// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestConfig содержит тестовые данные
var validConfigJSON = `{
    "license": "test-license-key",
    "rpc_list": [
        "https://api.mainnet-beta.solana.com",
        "https://solana-api.projectserum.com"
    ],
    "websocket_url": "wss://api.mainnet-beta.solana.com",
    "monitor_delay": 1000,
    "rpc_delay": 100,
    "price_delay": 500,
    "debug_logging": true,
    "tps_logging": true,
    "retries": 3,
    "workers": 5,
    "webhook_url": "https://test-webhook.com"
}`

var invalidConfigJSON = `{
    "license": "",
    "rpc_list": [],
    "websocket_url": "",
    "monitor_delay": -1
}`

// func setupTestConfig(t *testing.T, content string) string {
// 	// Создаем временную директорию для тестов
// 	tmpDir, err := os.MkdirTemp("", "config_test")
// 	if err != nil {
// 		t.Fatalf("Failed to create temp dir: %v", err)
// 	}

// 	// Создаем временный конфиг файл с безопасными правами доступа
// 	configPath := filepath.Join(tmpDir, "config.json")
// 	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
// 		os.RemoveAll(tmpDir)
// 		t.Fatalf("Failed to write config file: %v", err)
// 	}

// 	return configPath
// }

func cleanupTestConfig(configPath string) {
	os.RemoveAll(filepath.Dir(configPath))
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*Config) bool
	}{
		{
			name:    "Valid config",
			content: validConfigJSON,
			wantErr: false,
			check: func(cfg *Config) bool {
				return cfg.License == "test-license-key" &&
					len(cfg.RPCList) == 2 &&
					cfg.WebSocketURL == "wss://api.mainnet-beta.solana.com" &&
					cfg.MonitorDelay == 1000
			},
		},
		{
			name:    "Invalid config - empty required fields",
			content: invalidConfigJSON,
			wantErr: true,
			check:   nil,
		},
		{
			name:    "Invalid JSON syntax",
			content: "{invalid json",
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := setupTestConfig(t, tt.content)
			defer cleanupTestConfig(configPath)

			cfg, err := LoadConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.check != nil {
				if !tt.check(cfg) {
					t.Error("LoadConfig() returned invalid configuration")
				}
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "Valid configuration",
			cfg: &Config{
				License:      "test-license",
				RPCList:      []string{"https://test-rpc.com"},
				WebSocketURL: "wss://test-ws.com",
				MonitorDelay: 1000,
				RPCDelay:     100, // Добавляем RPCDelay
				PriceDelay:   500, // Добавляем PriceDelay
				Workers:      5,
				Retries:      3,
			},
			wantErr: false,
		},
		{
			name: "Missing license",
			cfg: &Config{
				RPCList:      []string{"https://test-rpc.com"},
				WebSocketURL: "wss://test-ws.com",
				MonitorDelay: 1000,
				RPCDelay:     100,
				PriceDelay:   500,
			},
			wantErr: true,
		},
		{
			name: "Empty RPC list",
			cfg: &Config{
				License:      "test-license",
				RPCList:      []string{},
				WebSocketURL: "wss://test-ws.com",
				MonitorDelay: 1000,
				RPCDelay:     100,
				PriceDelay:   500,
			},
			wantErr: true,
		},
		{
			name: "Invalid monitor delay",
			cfg: &Config{
				License:      "test-license",
				RPCList:      []string{"https://test-rpc.com"},
				WebSocketURL: "wss://test-ws.com",
				MonitorDelay: -1,
				RPCDelay:     100,
				PriceDelay:   500,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfigEnvironmentVariables(t *testing.T) {
	// Очищаем все переменные окружения перед тестом
	os.Clearenv()

	// Настраиваем тестовое окружение
	t.Setenv("SOLANA_BOT_LICENSE", "env-license-key")
	t.Setenv("SOLANA_BOT_RPC_LIST", "https://env-rpc1.com,https://env-rpc2.com")

	// Базовая конфигурация со всеми необходимыми полями
	configJSON := `{
        "license": "",
        "rpc_list": [],
        "websocket_url": "wss://test.com",
        "monitor_delay": 1000,
        "rpc_delay": 100,
        "price_delay": 500,
        "workers": 5,
        "retries": 3,
        "debug_logging": true,
        "tps_logging": true
    }`

	configPath := setupTestConfig(t, configJSON)
	defer cleanupTestConfig(configPath)

	// Создаем новый инстанс viper для изоляции
	v := viper.New()
	v.SetConfigFile(configPath)
	v.AutomaticEnv()
	v.SetEnvPrefix("SOLANA_BOT")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Проверяем, что значения из переменных окружения имеют приоритет
	if cfg.License != "env-license-key" {
		t.Errorf("Expected license from env var to be 'env-license-key', got %s", cfg.License)
	}

	expectedRPCs := []string{"https://env-rpc1.com", "https://env-rpc2.com"}
	if len(cfg.RPCList) != len(expectedRPCs) {
		t.Errorf("Expected %d RPCs, got %d", len(expectedRPCs), len(cfg.RPCList))
	}
	for i, rpc := range expectedRPCs {
		if cfg.RPCList[i] != rpc {
			t.Errorf("Expected RPC %s, got %s", rpc, cfg.RPCList[i])
		}
	}

	// Проверяем, что другие поля сохранили свои значения
	if cfg.MonitorDelay != 1000 {
		t.Errorf("Expected MonitorDelay to be 1000, got %d", cfg.MonitorDelay)
	}
	if cfg.RPCDelay != 100 {
		t.Errorf("Expected RPCDelay to be 100, got %d", cfg.RPCDelay)
	}
	if cfg.PriceDelay != 500 {
		t.Errorf("Expected PriceDelay to be 500, got %d", cfg.PriceDelay)
	}
}

func TestConfigValidationDetails(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		expectedError string
	}{
		{
			name: "Invalid WebSocket URL",
			config: Config{
				License:      "test-license",
				RPCList:      []string{"https://test.com"},
				WebSocketURL: "invalid-url",
				MonitorDelay: 1000,
			},
			expectedError: "invalid WebSocket URL protocol",
		},
		{
			name: "Invalid RPC URL",
			config: Config{
				License:      "test-license",
				RPCList:      []string{"invalid-url"},
				WebSocketURL: "wss://test.com",
				MonitorDelay: 1000,
			},
			expectedError: "invalid RPC URL protocol",
		},
		{
			name: "Invalid Workers Count",
			config: Config{
				License:      "test-license",
				RPCList:      []string{"https://test.com"},
				WebSocketURL: "wss://test.com",
				MonitorDelay: 1000,
				Workers:      -1,
			},
			expectedError: "invalid workers count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.config)
			if err == nil {
				t.Error("Expected error but got nil")
				return
			}
			if err.Error() != tt.expectedError {
				t.Errorf("Expected error '%s', got '%s'", tt.expectedError, err.Error())
			}
		})
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
	// Минимальная конфигурация
	configJSON := `{
		"license": "test-license",
		"rpc_list": ["https://test.com"],
		"websocket_url": "wss://test.com"
	}`

	configPath := setupTestConfig(t, configJSON)
	defer cleanupTestConfig(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Проверка значений по умолчанию
	if cfg.MonitorDelay != 1000 {
		t.Errorf("Expected default MonitorDelay 1000, got %d", cfg.MonitorDelay)
	}
	if cfg.Workers != 5 {
		t.Errorf("Expected default Workers 5, got %d", cfg.Workers)
	}
	if cfg.Retries != 3 {
		t.Errorf("Expected default Retries 3, got %d", cfg.Retries)
	}
}
