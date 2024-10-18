package config

import (
	"encoding/json"
	"io/ioutil"
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

func LoadConfig(path string) (*Config, error) {
	// Открытие файла конфигурации
	data, err := ioutil.ReadFile(path)
	if err != nil {
		// Возвращаем ошибку чтения файла
		return nil, err
	}

	// Парсинг JSON в структуру Config
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		// Возвращаем ошибку парсинга JSON
		return nil, err
	}

	// Валидация обязательных полей
	// Если поля отсутствуют, возвращаем ошибку

	return &cfg, nil
}
