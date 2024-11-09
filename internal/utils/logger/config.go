// internal/utils/logger/config.go
package logger

type Config struct {
	LogFile     string
	MaxSize     int  // мегабайты
	MaxAge      int  // дни
	MaxBackups  int  // количество файлов
	Compress    bool // сжимать ротированные файлы
	Development bool
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		LogFile:     "bot.log",
		MaxSize:     100,  // 100 MB
		MaxAge:      7,    // 7 дней
		MaxBackups:  3,    // 3 файла
		Compress:    true, // сжимать старые логи
		Development: false,
	}
}
