package config

import (
	"errors"

	"github.com/spf13/viper"
)

type Config struct {
	License                    string   `mapstructure:"license"`
	RPCList                    []string `mapstructure:"rpc_list"`
	WebSocketURL               string   `mapstructure:"websocket_url"`
	MonitorDelay               int      `mapstructure:"monitor_delay"`
	RPCDelay                   int      `mapstructure:"rpc_delay"`
	PriceDelay                 int      `mapstructure:"price_delay"`
	DebugLogging               bool     `mapstructure:"debug_logging"`
	TPSLogging                 bool     `mapstructure:"tps_logging"`
	Retries                    int      `mapstructure:"retries"`
	WebhookURL                 string   `mapstructure:"webhook_url"`
	RaydiumSwapInstructionCode uint64   `mapstructure:"raydium_swap_instruction_code"`
	RaydiumAmmProgramID        string   `mapstructure:"raydium_amm_program_id"`
	SerumProgramID             string   `mapstructure:"serum_program_id"`
	Workers                    int      `mapstructure:"workers"`
}

// LoadConfig читает JSON-файл конфигурации и возвращает заполненную структуру Config.
func LoadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.AutomaticEnv() // Чтение переменных окружения

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Валидация конфигурации
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
