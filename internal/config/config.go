// =================================
// File: internal/config/config.go
// =================================
package config

import (
	"errors"

	"github.com/spf13/viper"
)

type Config struct {
	RPCList      []string `mapstructure:"rpc_list"`
	WebSocketURL string   `mapstructure:"websocket_url"`
	PostgresURL  string   `mapstructure:"postgres_url"`

	// Add other small flags if necessary
	DebugLogging bool `mapstructure:"debug_logging"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Defaults
	v.SetDefault("debug_logging", true)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Minimal validation
	if len(cfg.RPCList) == 0 {
		return nil, errors.New("rpc_list is empty, please specify at least one Solana RPC")
	}
	if cfg.PostgresURL == "" {
		return nil, errors.New("postgres_url is required")
	}

	return &cfg, nil
}
