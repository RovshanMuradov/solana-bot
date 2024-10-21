// internal/eventlistener/handler.go

// Этот файл содержит логику обработки событий.
package eventlistener

import (
	"go.uber.org/zap"
)

func HandleEvent(event Event, logger *zap.Logger) {
	switch event.Type {
	case "NewPool":
		logger.Info("Обнаружен новый пул",
			zap.String("poolId", event.PoolID),
			zap.String("tokenA", event.TokenA),
			zap.String("tokenB", event.TokenB))

		// Здесь можно добавить логику для передачи информации снайперу
		// например, вызов функции sniper.NotifyNewPool(event)
	case "PriceChange":
		logger.Info("Изменение цены",
			zap.String("poolId", event.PoolID),
			zap.String("tokenA", event.TokenA),
			zap.String("tokenB", event.TokenB))
		// Добавить обработку изменения цены
	default:
		logger.Debug("Получено нерелевантное событие",
			zap.String("type", event.Type))
	}
}
