// =================================
// File: internal/config/config.go
// =================================
package config

import (
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type Config struct {
	License      string   `mapstructure:"license"`
	RPCList      []string `mapstructure:"rpc_list"`
	WebSocketURL string   `mapstructure:"websocket_url"`
	MonitorDelay int      `mapstructure:"monitor_delay"`
	RPCDelay     int      `mapstructure:"rpc_delay"`
	PriceDelay   int      `mapstructure:"price_delay"`
	DebugLogging bool     `mapstructure:"debug_logging"`
	TPSLogging   bool     `mapstructure:"tps_logging"`
	Retries      int      `mapstructure:"retries"`
	WebhookURL   string   `mapstructure:"webhook_url"`
	Workers      int      `mapstructure:"workers"`
	PostgresURL  string   `mapstructure:"postgres_url"`
}

const (
	DefaultMonitorDelay = 1000
	DefaultRPCDelay     = 100
	DefaultPriceDelay   = 500
	DefaultWorkers      = 5
	DefaultRetries      = 3
)

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	defaults := map[string]interface{}{
		"monitor_delay": DefaultMonitorDelay,
		"rpc_delay":     DefaultRPCDelay,
		"price_delay":   DefaultPriceDelay,
		"workers":       DefaultWorkers,
		"retries":       DefaultRetries,
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if err := loadEnvironmentVariables(v, &cfg); err != nil {
		return nil, err
	}

	return &cfg, validateConfig(&cfg)
}

func validateConfig(cfg *Config) error {
	if cfg.License == "" {
		return errors.New("missing license in configuration")
	}
	if len(cfg.RPCList) == 0 {
		return errors.New("rpc_list is empty")
	}
	if cfg.WebSocketURL != "" {
		if err := validateURLWithCache(cfg.WebSocketURL, "ws"); err != nil {
			return errors.New("invalid WebSocket URL protocol")
		}
	}
	for _, rpcURL := range cfg.RPCList {
		if err := validateURLWithCache(rpcURL, "http"); err != nil {
			return errors.New("invalid RPC URL protocol")
		}
	}
	if err := validateNumericParams(cfg); err != nil {
		return err
	}
	if cfg.WebhookURL != "" {
		if err := validateURLWithCache(cfg.WebhookURL, "https"); err != nil {
			return errors.New("webhook URL must use HTTPS")
		}
	}
	return nil
}

func validateNumericParams(cfg *Config) error {
	if cfg.MonitorDelay <= 0 {
		return errors.New("invalid monitor_delay")
	}
	if cfg.Workers < 0 {
		return errors.New("invalid workers count")
	}
	if cfg.RPCDelay <= 0 {
		return errors.New("invalid rpc_delay")
	}
	if cfg.PriceDelay <= 0 {
		return errors.New("invalid price_delay")
	}
	if cfg.Retries < 0 {
		return errors.New("invalid retries count")
	}
	return nil
}

var urlCache sync.Map

func validateURLWithCache(rawURL string, protocol string) error {
	if _, ok := urlCache.Load(rawURL); ok {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL format")
	}
	if !strings.HasPrefix(parsed.Scheme, protocol) {
		return errors.New("invalid URL protocol")
	}
	urlCache.Store(rawURL, parsed)
	return nil
}

func loadEnvironmentVariables(v *viper.Viper, cfg *Config) error {
	v.AutomaticEnv()
	v.SetEnvPrefix("SOLANA_BOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	envLicense := v.GetString("LICENSE")
	if envLicense != "" {
		cfg.License = envLicense
	}

	envRPCList := v.GetString("RPC_LIST")
	if envRPCList != "" {
		rpcs := strings.Split(envRPCList, ",")
		var cleanRPCs []string
		for _, rpc := range rpcs {
			clean := strings.TrimSpace(rpc)
			if clean != "" {
				cleanRPCs = append(cleanRPCs, clean)
			}
		}
		if len(cleanRPCs) > 0 {
			cfg.RPCList = cleanRPCs
		}
	}
	return nil
}
