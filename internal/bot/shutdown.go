package bot

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Closer represents a service that can be closed
type Closer interface {
	Close() error
}

// CloseFunc allows using a function as a Closer
type CloseFunc func() error

func (f CloseFunc) Close() error {
	return f()
}

// ShutdownHandler manages graceful shutdown of multiple services
type ShutdownHandler struct {
	logger          *zap.Logger
	services        []namedService
	mu              sync.Mutex
	timeout         time.Duration
	shutdownChan    chan struct{}
	shutdownOnce    sync.Once
	shutdownStarted bool
}

type namedService struct {
	name   string
	closer io.Closer
}

// NewShutdownHandler creates a new shutdown handler
func NewShutdownHandler(logger *zap.Logger, timeout time.Duration) *ShutdownHandler {
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	return &ShutdownHandler{
		logger:       logger,
		timeout:      timeout,
		shutdownChan: make(chan struct{}),
	}
}

// Add registers a service for shutdown
func (sh *ShutdownHandler) Add(name string, closer io.Closer) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	sh.services = append(sh.services, namedService{
		name:   name,
		closer: closer,
	})

	if sh.logger != nil {
		sh.logger.Debug("Registered service for shutdown", zap.String("service", name))
	}
}

// AddFunc registers a shutdown function
func (sh *ShutdownHandler) AddFunc(name string, fn func() error) {
	sh.Add(name, CloseFunc(fn))
}

// SetLogger sets the logger for the shutdown handler
func (sh *ShutdownHandler) SetLogger(logger *zap.Logger) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.logger = logger
}

// RegisterService registers a service for shutdown (alias for Add)
func (sh *ShutdownHandler) RegisterService(name string, closer io.Closer) {
	sh.Add(name, closer)
}

// InitiateShutdown triggers a graceful shutdown programmatically
func (sh *ShutdownHandler) InitiateShutdown() {
	sh.shutdownOnce.Do(func() {
		sh.mu.Lock()
		sh.shutdownStarted = true
		sh.mu.Unlock()
		close(sh.shutdownChan)
	})
}

// ShutdownInitiated returns a channel that will be closed when shutdown is initiated
func (sh *ShutdownHandler) ShutdownInitiated() <-chan struct{} {
	return sh.shutdownChan
}

// WaitForShutdown waits for shutdown to complete
func (sh *ShutdownHandler) WaitForShutdown() {
	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), sh.timeout)
	defer cancel()

	sh.Shutdown(ctx)
}

// HandleShutdown listens for shutdown signals and gracefully closes all services
func (sh *ShutdownHandler) HandleShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	if sh.logger != nil {
		sh.logger.Info("Shutdown signal received", zap.String("signal", sig.String()))
	}

	// Trigger shutdown signal
	sh.InitiateShutdown()

	// Create context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), sh.timeout)
	defer cancel()

	sh.Shutdown(ctx)
}

// Shutdown closes all registered services
func (sh *ShutdownHandler) Shutdown(ctx context.Context) {
	sh.mu.Lock()
	services := make([]namedService, len(sh.services))
	copy(services, sh.services)
	sh.mu.Unlock()

	if sh.logger != nil {
		sh.logger.Info("Starting graceful shutdown", zap.Int("services", len(services)))
	}

	// Close services in reverse order (LIFO)
	var wg sync.WaitGroup
	errors := make(chan error, len(services))

	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		wg.Add(1)

		go func(s namedService) {
			defer wg.Done()

			// Create a channel to signal completion
			done := make(chan error, 1)

			go func() {
				if sh.logger != nil {
					sh.logger.Info("Shutting down service", zap.String("service", s.name))
				}
				err := s.closer.Close()
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					if sh.logger != nil {
						sh.logger.Error("Failed to shutdown service",
							zap.String("service", s.name),
							zap.Error(err))
					}
					errors <- fmt.Errorf("%s: %w", s.name, err)
				} else {
					if sh.logger != nil {
						sh.logger.Info("Service shutdown complete",
							zap.String("service", s.name))
					}
				}
			case <-ctx.Done():
				if sh.logger != nil {
					sh.logger.Error("Shutdown timeout for service",
						zap.String("service", s.name))
				}
				errors <- fmt.Errorf("%s: shutdown timeout", s.name)
			}
		}(svc)
	}

	// Wait for all services to complete or timeout
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		if sh.logger != nil {
			sh.logger.Info("All services shutdown complete")
		}
	case <-ctx.Done():
		if sh.logger != nil {
			sh.logger.Error("Shutdown timeout exceeded")
		}
	}

	// Collect errors
	close(errors)
	var shutdownErrors []error
	for err := range errors {
		shutdownErrors = append(shutdownErrors, err)
	}

	if len(shutdownErrors) > 0 {
		if sh.logger != nil {
			sh.logger.Error("Shutdown completed with errors",
				zap.Int("errorCount", len(shutdownErrors)))
			for _, err := range shutdownErrors {
				sh.logger.Error("Shutdown error", zap.Error(err))
			}
		}
	} else {
		if sh.logger != nil {
			sh.logger.Info("Graceful shutdown completed successfully")
		}
	}
}

// HandleShutdownWithServices is a convenience function that creates a handler,
// registers services, and handles shutdown
func HandleShutdownWithServices(logger *zap.Logger, services ...io.Closer) {
	handler := NewShutdownHandler(logger, 30*time.Second)

	for i, svc := range services {
		name := fmt.Sprintf("service_%d", i+1)
		handler.Add(name, svc)
	}

	handler.HandleShutdown()
}

// ShutdownManager provides a global shutdown handler for the application
type ShutdownManager struct {
	handler *ShutdownHandler
	once    sync.Once
}

var globalShutdownManager = &ShutdownManager{}

// RegisterForShutdown registers a service with the global shutdown manager
func RegisterForShutdown(name string, closer io.Closer, logger *zap.Logger) {
	globalShutdownManager.once.Do(func() {
		globalShutdownManager.handler = NewShutdownHandler(logger, 30*time.Second)

		// Start listening for shutdown in a goroutine
		go globalShutdownManager.handler.HandleShutdown()
	})

	globalShutdownManager.handler.Add(name, closer)
}

// WaitForShutdown blocks until shutdown is triggered
// This should be called in main() after all services are started
func WaitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
}

// Global singleton instance
var globalSingletonHandler *ShutdownHandler
var globalSingletonOnce sync.Once

// GetShutdownHandler returns the global singleton shutdown handler
func GetShutdownHandler() *ShutdownHandler {
	globalSingletonOnce.Do(func() {
		globalSingletonHandler = NewShutdownHandler(nil, 30*time.Second)

		// Start signal handling in background
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			select {
			case sig := <-sigChan:
				if globalSingletonHandler.logger != nil {
					globalSingletonHandler.logger.Info("OS shutdown signal received", zap.String("signal", sig.String()))
				}
				globalSingletonHandler.InitiateShutdown()
			case <-globalSingletonHandler.ShutdownInitiated():
				// Already initiated by program
			}
		}()
	})

	return globalSingletonHandler
}
