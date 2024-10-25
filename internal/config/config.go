// internal/config/config.go
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

// Значения по умолчанию
const (
	DefaultMonitorDelay = 1000
	DefaultRPCDelay     = 100
	DefaultPriceDelay   = 500
	DefaultWorkers      = 5
	DefaultRetries      = 3
)

// Оптимизированная функция LoadConfig
func LoadConfig(path string) (*Config, error) {
	v := viper.New() // Создаем новый экземпляр для изоляции
	v.SetConfigFile(path)

	// Устанавливаем значения по умолчанию одним вызовом
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

	// Проверяем переменные окружения более эффективно
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
		return errors.New("RPC list is empty in configuration")
	}

	// Проверка WebSocket URL с использованием кэша
	if cfg.WebSocketURL != "" {
		if err := validateURLWithCache(cfg.WebSocketURL, "ws"); err != nil {
			return errors.New("invalid WebSocket URL protocol")
		}
	}

	// Проверка RPC URLs с использованием кэша
	for _, rpcURL := range cfg.RPCList {
		if err := validateURLWithCache(rpcURL, "http"); err != nil {
			return errors.New("invalid RPC URL protocol")
		}
	}

	// Проверка числовых параметров одним блоком
	if err := validateNumericParams(cfg); err != nil {
		return err
	}

	// Проверка webhook URL с использованием кэша
	if cfg.WebhookURL != "" {
		if err := validateURLWithCache(cfg.WebhookURL, "https"); err != nil {
			return errors.New("webhook URL must use HTTPS")
		}
	}

	return nil
}

// Выделяем проверку числовых параметров в отдельную функцию
func validateNumericParams(cfg *Config) error {
	if cfg.MonitorDelay <= 0 {
		return errors.New("invalid monitor delay in configuration")
	}
	if cfg.Workers < 0 {
		return errors.New("invalid workers count")
	}
	if cfg.RPCDelay <= 0 {
		return errors.New("invalid RPC delay")
	}
	if cfg.PriceDelay <= 0 {
		return errors.New("invalid price delay")
	}
	if cfg.Retries < 0 {
		return errors.New("invalid retries count")
	}
	return nil
}

// Кэш для URL
var urlCache sync.Map

// Оптимизированная функция проверки URL
func validateURLWithCache(rawURL string, protocol string) error {
	// Проверяем кэш
	if _, ok := urlCache.Load(rawURL); ok {
		return nil
	}

	// Парсим URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL format")
	}

	if !strings.HasPrefix(parsedURL.Scheme, protocol) {
		return errors.New("invalid URL protocol")
	}

	// Сохраняем в кэш
	urlCache.Store(rawURL, parsedURL)
	return nil
}

// Отдельная функция для загрузки переменных окружения
func loadEnvironmentVariables(v *viper.Viper, cfg *Config) error {
	// Устанавливаем настройки для переменных окружения
	v.AutomaticEnv()
	v.SetEnvPrefix("SOLANA_BOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Проверяем лицензию из переменных окружения
	envLicense := v.GetString("LICENSE")
	if envLicense != "" {
		cfg.License = envLicense
	}

	// Проверяем список RPC из переменных окружения
	envRPCList := v.GetString("RPC_LIST")
	if envRPCList != "" {
		// Разделяем строку по запятым и убираем пробелы
		rpcs := strings.Split(envRPCList, ",")
		cleanRPCs := make([]string, 0, len(rpcs))
		for _, rpc := range rpcs {
			cleanRPC := strings.TrimSpace(rpc)
			if cleanRPC != "" {
				cleanRPCs = append(cleanRPCs, cleanRPC)
			}
		}
		if len(cleanRPCs) > 0 {
			cfg.RPCList = cleanRPCs
		}
	}

	return nil
}
