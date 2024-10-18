// internal/utils/logger.go
package utils

import (
	"go.uber.org/zap"
)

func InitLogger(debug bool) (*zap.Logger, error) {
	if debug {
		// Инициализируем логгер в режиме Debug
		return zap.NewDevelopment()
	} else {
		// Инициализируем логгер в режиме Production
		return zap.NewProduction()
	}
}
