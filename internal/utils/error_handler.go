// internal/utils/error_handler.go
package utils

import (
	"fmt"

	"go.uber.org/zap"
)

func HandleError(logger *zap.Logger, err error, message string) {
	if err != nil {
		logger.Error(message, zap.Error(err))
	}
}

func WrapError(err error, message string) error {
	return fmt.Errorf("%s: %w", message, err)
}
