// internal/blockchain/solbc/rpc/enhanced_client.go

package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

const (
	healthCheckInterval = 30 * time.Second
	reconnectDelay      = 5 * time.Second
	maxReconnectTries   = 3
)

// EnhancedClient представляет улучшенный RPC клиент с мониторингом здоровья
type EnhancedClient struct {
	nodes          []*NodeClient
	activeNodes    sync.Map
	healthyChan    chan string // URL здоровых узлов
	logger         *zap.Logger
	metrics        *ClientMetrics
	ctx            context.Context
	cancel         context.CancelFunc
	currentNodeIdx int
	mu             sync.RWMutex
}

// ClientMetrics содержит метрики клиента
type ClientMetrics struct {
	TotalRequests  uint64
	FailedRequests uint64
	ActiveNodes    int32
	LastSuccessful time.Time
	AverageLatency time.Duration
	lastLatencies  []time.Duration
	metricsLock    sync.RWMutex
}

func NewEnhancedClient(urls []string, logger *zap.Logger) (*EnhancedClient, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no RPC URLs provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := &EnhancedClient{
		nodes:       make([]*NodeClient, 0, len(urls)),
		healthyChan: make(chan string, len(urls)),
		logger:      logger.Named("enhanced-rpc"),
		metrics:     &ClientMetrics{},
		ctx:         ctx,
		cancel:      cancel,
	}

	// Инициализация узлов
	for _, url := range urls {
		node, err := client.initNode(url)
		if err != nil {
			logger.Warn("Failed to initialize node", zap.String("url", url), zap.Error(err))
			continue
		}
		client.nodes = append(client.nodes, node)
		client.activeNodes.Store(url, true)
	}

	if len(client.nodes) == 0 {
		return nil, fmt.Errorf("failed to initialize any nodes")
	}

	// Запуск мониторинга здоровья
	go client.healthCheck()

	return client, nil
}

func (c *EnhancedClient) initNode(url string) (*NodeClient, error) {
	rpcClient := solanarpc.New(url)
	return &NodeClient{
		Client: rpcClient,
		URL:    url,
		active: true,
	}, nil
}

func (c *EnhancedClient) healthCheck() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkNodesHealth()
		}
	}
}

func (c *EnhancedClient) checkNodesHealth() {
	var wg sync.WaitGroup
	activeCount := int32(0)

	for _, node := range c.nodes {
		wg.Add(1)
		go func(n *NodeClient) {
			defer wg.Done()

			// Проверяем здоровье с несколькими попытками
			for attempt := 0; attempt < 3; attempt++ {
				if healthy := c.checkNodeHealth(n); healthy {
					atomic.AddInt32(&activeCount, 1)
					c.activeNodes.Store(n.URL, true)
					select {
					case c.healthyChan <- n.URL:
					default:
					}
					return
				}
				time.Sleep(reconnectDelay / 3)
			}

			c.activeNodes.Delete(n.URL)
			c.logger.Warn("Node marked as inactive",
				zap.String("url", n.URL),
				zap.Int("failed_attempts", 3))
		}(node)
	}

	wg.Wait()

	// Обновление метрик
	c.metrics.metricsLock.Lock()
	c.metrics.ActiveNodes = activeCount
	c.metrics.metricsLock.Unlock()

	// Безопасная проверка с учетом возможного переполнения
	nodesCount := len(c.nodes)
	threshold := nodesCount / 2
	if nodesCount > 0 && int(activeCount) < threshold {
		c.logger.Warn("Low number of active nodes, starting aggressive reconnection",
			zap.Int32("active_nodes", activeCount),
			zap.Int("total_nodes", nodesCount))
		go c.aggressiveReconnect()
	}
}

func (c *EnhancedClient) aggressiveReconnect() {
	backoff := reconnectDelay
	maxBackoff := 30 * time.Second

	for {
		nodesCount := len(c.nodes)
		threshold := nodesCount / 2
		if nodesCount > 0 && int(atomic.LoadInt32(&c.metrics.ActiveNodes)) >= threshold {
			return
		}

		c.logger.Info("Attempting aggressive reconnection of inactive nodes")
		reconnected := false

		for _, node := range c.nodes {
			if active, ok := c.activeNodes.Load(node.URL); !ok || !active.(bool) {
				if c.checkNodeHealth(node) {
					c.activeNodes.Store(node.URL, true)
					atomic.AddInt32(&c.metrics.ActiveNodes, 1)
					reconnected = true
					c.logger.Info("Successfully reconnected node during aggressive reconnection",
						zap.String("url", node.URL))
				}
			}
		}

		if !reconnected {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			time.Sleep(backoff)
		} else {
			backoff = reconnectDelay
		}
	}
}

func (c *EnhancedClient) checkNodeHealth(node *NodeClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err := node.Client.GetVersion(ctx)
	duration := time.Since(start)

	if err != nil {
		c.logger.Warn("Node health check failed",
			zap.String("url", node.URL),
			zap.Error(err))
		return false
	}

	// Обновление метрик латентности
	c.updateLatencyMetrics(duration)
	return true
}

func (c *EnhancedClient) updateLatencyMetrics(duration time.Duration) {
	c.metrics.metricsLock.Lock()
	defer c.metrics.metricsLock.Unlock()

	c.metrics.lastLatencies = append(c.metrics.lastLatencies, duration)
	if len(c.metrics.lastLatencies) > 10 {
		c.metrics.lastLatencies = c.metrics.lastLatencies[1:]
	}

	var total time.Duration
	for _, d := range c.metrics.lastLatencies {
		total += d
	}
	c.metrics.AverageLatency = total / time.Duration(len(c.metrics.lastLatencies))
}

func (c *EnhancedClient) GetHealthyNode() (*NodeClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Проверяем все узлы в порядке очереди
	initialIdx := c.currentNodeIdx
	for i := 0; i < len(c.nodes); i++ {
		idx := (initialIdx + i) % len(c.nodes)
		node := c.nodes[idx]

		if active, ok := c.activeNodes.Load(node.URL); ok && active.(bool) {
			c.currentNodeIdx = (idx + 1) % len(c.nodes)
			return node, nil
		}
	}

	// Если нет активных узлов, пробуем переподключиться
	if err := c.reconnectInactiveNodes(); err != nil {
		return nil, fmt.Errorf("no active nodes available and reconnection failed: %w", err)
	}

	// Повторная попытка получить здоровый узел
	for _, node := range c.nodes {
		if active, ok := c.activeNodes.Load(node.URL); ok && active.(bool) {
			return node, nil
		}
	}

	return nil, fmt.Errorf("no active RPC nodes available")
}

func (c *EnhancedClient) reconnectInactiveNodes() error {
	for _, node := range c.nodes {
		if active, ok := c.activeNodes.Load(node.URL); !ok || !active.(bool) {
			c.logger.Info("Attempting to reconnect node", zap.String("url", node.URL))

			// Попытка переподключения
			for i := 0; i < maxReconnectTries; i++ {
				if c.checkNodeHealth(node) {
					c.activeNodes.Store(node.URL, true)
					c.logger.Info("Successfully reconnected node", zap.String("url", node.URL))
					return nil
				}
				time.Sleep(reconnectDelay)
			}
		}
	}
	return fmt.Errorf("failed to reconnect any nodes")
}

func (c *EnhancedClient) ExecuteWithRetry(ctx context.Context, operation func(*NodeClient) error) error {
	var lastErr error
	backoff := RetryDelay

	for attempt := 0; attempt < maxReconnectTries*2; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			node, err := c.GetHealthyNode()
			if err != nil {
				lastErr = err
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
				continue
			}

			start := time.Now()
			err = operation(node)
			duration := time.Since(start)

			if err == nil {
				c.metrics.metricsLock.Lock()
				c.metrics.TotalRequests++
				c.metrics.LastSuccessful = time.Now()
				c.updateLatencyMetrics(duration)
				c.metrics.metricsLock.Unlock()
				return nil
			}

			lastErr = err
			c.metrics.metricsLock.Lock()
			c.metrics.FailedRequests++
			c.metrics.metricsLock.Unlock()

			// Анализируем ошибку для определения стратегии повтора
			if IsRetryableError(err) {
				c.logger.Debug("Retryable error occurred, will retry",
					zap.Error(err),
					zap.Duration("backoff", backoff))
				time.Sleep(backoff)
				backoff *= 2
				continue
			}

			// Для критических ошибок помечаем узел как неактивный
			if IsCriticalError(err) {
				c.activeNodes.Delete(node.URL)
				c.logger.Warn("Node marked as inactive due to critical error",
					zap.String("url", node.URL),
					zap.Error(err))
			}
		}
	}

	return fmt.Errorf("all retry attempts failed: %w", lastErr)
}

// IsRetryableError определяет, можно ли повторить операцию при данной ошибке
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Проверяем конкретные типы ошибок
	var rpcErr *Error
	if errors.As(err, &rpcErr) {
		switch {
		case errors.Is(rpcErr.Err, ErrTimeout),
			errors.Is(rpcErr.Err, ErrRateLimit),
			errors.Is(rpcErr.Err, ErrConnectionFailed):
			return true
		}
	}

	// Проверяем текст ошибки для общих сетевых проблем
	errStr := err.Error()
	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout")
}

// IsCriticalError определяет, является ли ошибка критической
func IsCriticalError(err error) bool {
	if err == nil {
		return false
	}

	var rpcErr *Error
	if errors.As(err, &rpcErr) && errors.Is(rpcErr.Err, ErrInvalidResponse) {
		return true
	}

	errStr := err.Error()
	return strings.Contains(errStr, "invalid request") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden")
}

// GetMetrics возвращает текущие метрики клиента
func (c *EnhancedClient) GetMetrics() *ClientMetrics {
	c.metrics.metricsLock.RLock()
	defer c.metrics.metricsLock.RUnlock()

	return &ClientMetrics{
		TotalRequests:  c.metrics.TotalRequests,
		FailedRequests: c.metrics.FailedRequests,
		ActiveNodes:    c.metrics.ActiveNodes,
		AverageLatency: c.metrics.AverageLatency,
		LastSuccessful: c.metrics.LastSuccessful,
	}
}

func (c *EnhancedClient) Close() {
	c.cancel()
}
