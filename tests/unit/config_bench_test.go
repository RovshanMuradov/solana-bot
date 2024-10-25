// internal/config/config_bench_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// testHelper интерфейс для обобщения методов T и B
type testHelper interface {
	Fatalf(format string, args ...interface{})
}

// setupTestConfig теперь принимает интерфейс testHelper
func setupTestConfig(t testHelper, content string) string {
	// Создаем временную директорию для тестов
	tmpDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Создаем временный конфиг файл
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to write config file: %v", err)
	}

	return configPath
}

// Тесты остаются без изменений...

// Бенчмарки
func BenchmarkLoadConfig(b *testing.B) {
	configPath := setupTestConfig(b, validConfigJSON)
	defer cleanupTestConfig(configPath)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cfg, err := LoadConfig(configPath)
		if err != nil {
			b.Fatal(err)
		}
		if cfg == nil {
			b.Fatal("config is nil")
		}
	}
}

func BenchmarkValidateConfig(b *testing.B) {
	cfg := &Config{
		License:      "test-license",
		RPCList:      []string{"https://test-rpc.com"},
		WebSocketURL: "wss://test-ws.com",
		MonitorDelay: 1000,
		RPCDelay:     100,
		PriceDelay:   500,
		Workers:      5,
		Retries:      3,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := validateConfig(cfg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadConfigWithEnvironment(b *testing.B) {
	b.Setenv("SOLANA_BOT_LICENSE", "env-license-key")
	b.Setenv("SOLANA_BOT_RPC_LIST", "https://env-rpc1.com,https://env-rpc2.com")

	configPath := setupTestConfig(b, validConfigJSON)
	defer cleanupTestConfig(configPath)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cfg, err := LoadConfig(configPath)
		if err != nil {
			b.Fatal(err)
		}
		if cfg == nil {
			b.Fatal("config is nil")
		}
	}
}

func BenchmarkValidateConfigParallel(b *testing.B) {
	cfg := &Config{
		License:      "test-license",
		RPCList:      []string{"https://test-rpc.com"},
		WebSocketURL: "wss://test-ws.com",
		MonitorDelay: 1000,
		RPCDelay:     100,
		PriceDelay:   500,
		Workers:      5,
		Retries:      3,
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := validateConfig(cfg); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkURLValidationWithCache(b *testing.B) {
	b.Run("First validation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := validateURLWithCache("https://test.com", "http"); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Cached validation", func(b *testing.B) {
		// Прогреваем кэш
		if err := validateURLWithCache("https://test.com", "http"); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := validateURLWithCache("https://test.com", "http"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
