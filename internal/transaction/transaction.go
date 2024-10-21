// internal/transaction/transaction.go
package transaction

import (
	"time"
)

// RetryOperation выполняет операцию с повторными попытками и экспоненциальной задержкой
func RetryOperation(attempts int, sleep time.Duration, operation func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = operation()
		if err == nil {
			return nil
		}
		time.Sleep(sleep)
		sleep *= 2 // Экспоненциальное увеличение задержки
	}
	return err
}
