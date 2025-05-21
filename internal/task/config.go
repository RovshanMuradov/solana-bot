// =============================================
// File: internal/task/config.go
// =============================================
package task

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds application settings loaded from config.json.
type Config struct {
	License      string        `mapstructure:"license"`
	RPCList      []string      `mapstructure:"rpc_list"`
	WebSocketURL string        `mapstructure:"websocket_url"`
	MonitorDelay time.Duration `mapstructure:"-"` // Converted from monitor_delay (ms)
	RPCDelay     time.Duration `mapstructure:"-"` // Converted from rpc_delay (ms)
	PriceDelay   time.Duration `mapstructure:"-"` // Converted from price_delay (ms)
	DebugLogging bool          `mapstructure:"debug_logging"`
	TPSLogging   bool          `mapstructure:"tps_logging"`
	Retries      int           `mapstructure:"retries"`
	WebhookURL   string        `mapstructure:"webhook_url"`
	Workers      int           `mapstructure:"workers"`
}

// LoadConfig reads configuration from the specified file path and performs validation.
func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	// Defaults
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

	// Convert ms values to Duration
	cfg.MonitorDelay = time.Duration(v.GetInt("monitor_delay")) * time.Millisecond
	cfg.RPCDelay = time.Duration(v.GetInt("rpc_delay")) * time.Millisecond
	cfg.PriceDelay = time.Duration(v.GetInt("price_delay")) * time.Millisecond

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate checks required fields and applies defaults if necessary.
func (c *Config) validate() error {
	if len(c.RPCList) == 0 {
		return fmt.Errorf("rpc_list must contain at least one RPC endpoint")
	}
	if c.License == "" {
		return fmt.Errorf("license is required")
	}
	if c.WebSocketURL == "" {
		return fmt.Errorf("websocket_url is required")
	}
	if c.Workers <= 0 {
		c.Workers = 1
	}
	if c.Retries <= 0 {
		c.Retries = 3
	}
	return nil
}

// ValidateLicense returns true if the provided license string meets basic criteria.
func ValidateLicense(license string) bool {
	return license != ""
}
