package config

import (
	"encoding/json"
	"errors"
	"os"
)

type Config struct {
	License      string   `json:"license"`
	RPCList      []string `json:"rpc_list"`
	WebSocketURL string   `json:"websocket_url"`
	MonitorDelay int      `json:"monitor_delay"`
	RPCDelay     int      `json:"rpc_delay"`
	PriceDelay   int      `json:"price_delay"`
	DebugLogging bool     `json:"debug_logging"`
	TPSLogging   bool     `json:"tps_logging"`
	Retries      int      `json:"retries"`
	WebhookURL   string   `json:"webhook_url"`
}

// LoadConfig читает JSON-файл конфигурации и возвращает заполненную структуру Config.
func LoadConfig(path string) (*Config, error) {
	// Открытие файла конфигурации
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Декодирование JSON в структуру Config
	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	// Валидация обязательных полей
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validateConfig проверяет наличие и корректность обязательных полей конфигурации.
func validateConfig(cfg *Config) error {
	if cfg.License == "" {
		return errors.New("missing license in configuration")
	}
	if len(cfg.RPCList) == 0 {
		return errors.New("RPC list is empty in configuration")
	}
	if cfg.WebSocketURL == "" {
		return errors.New("missing WebSocket URL in configuration")
	}
	if cfg.MonitorDelay <= 0 {
		return errors.New("invalid monitor delay in configuration")
	}
	// Добавьте дополнительные проверки для других важных полей

	return nil
}
