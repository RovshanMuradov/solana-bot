// internal/eventlistener/handler.go
package eventlistener

import (
	"go.uber.org/zap"
)

func HandleEvent(event Event, logger *zap.Logger) {
	// Проверяем, соответствует ли событие нашим критериям
	// Если да, выполняем необходимые действия
	// Например, уведомляем снайпера о новом пуле
	logger.Info("Получено событие", zap.Any("event", event))
}
