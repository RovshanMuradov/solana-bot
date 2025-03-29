// =============================================
// File: internal/task/config.go
// =============================================
package task

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config defines complete application configuration with all fields from config.json
type Config struct {
	License        string        `mapstructure:"license"`
	RPCList        []string      `mapstructure:"rpc_list"`
	WebSocketURL   string        `mapstructure:"websocket_url"`
	MonitorDelay   time.Duration `mapstructure:"-"` // Converted from milliseconds
	MonitorDelayMS int           `mapstructure:"monitor_delay"`
	RPCDelay       time.Duration `mapstructure:"-"` // Converted from milliseconds
	RPCDelayMS     int           `mapstructure:"rpc_delay"`
	PriceDelay     time.Duration `mapstructure:"-"` // Converted from milliseconds
	PriceDelayMS   int           `mapstructure:"price_delay"`
	DebugLogging   bool          `mapstructure:"debug_logging"`
	TPSLogging     bool          `mapstructure:"tps_logging"`
	Retries        int           `mapstructure:"retries"`
	WebhookURL     string        `mapstructure:"webhook_url"`
	Workers        int           `mapstructure:"workers"`
}

// LoadConfig reads and validates configuration from the provided path
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Set defaults
	v.SetDefault("debug_logging", true)
	v.SetDefault("tps_logging", false)
	v.SetDefault("price_delay", 500)
	v.SetDefault("monitor_delay", 1000)
	v.SetDefault("rpc_delay", 100)
	v.SetDefault("retries", 3)
	v.SetDefault("workers", 1)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config error: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	// Convert milliseconds to durations
	cfg.MonitorDelay = time.Duration(cfg.MonitorDelayMS) * time.Millisecond
	cfg.RPCDelay = time.Duration(cfg.RPCDelayMS) * time.Millisecond
	cfg.PriceDelay = time.Duration(cfg.PriceDelayMS) * time.Millisecond

	// Basic validation
	if len(cfg.RPCList) == 0 {
		return nil, fmt.Errorf("rpc_list is empty")
	}

	if cfg.License == "" {
		return nil, fmt.Errorf("license is required")
	}

	if cfg.WebSocketURL == "" {
		return nil, fmt.Errorf("websocket_url is required")
	}

	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}

	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}

	return &cfg, nil
}

// ValidateLicense is a placeholder for license validation
func ValidateLicense(license string) bool {
	// Placeholder for license validation logic
	return license != ""
}
