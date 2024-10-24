package eventlistener

import (
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Event struct {
	Type   string `json:"type"`
	PoolID string `json:"pool_id,omitempty"`
	TokenA string `json:"token_a,omitempty"`
	TokenB string `json:"token_b,omitempty"`
}

type eventValidator struct {
	validTypes map[string]bool
}

type EventListener struct {
	conn      net.Conn
	logger    *zap.Logger
	wsURL     string
	mu        sync.Mutex
	closeOnce sync.Once
	done      chan struct{}
	validator *eventValidator
}

const (
	initialBackoff = 200 * time.Millisecond
	maxBackoff     = 2 * time.Second
	maxAttempts    = 5
	readTimeout    = 5 * time.Second
	writeTimeout   = 5 * time.Second
)
