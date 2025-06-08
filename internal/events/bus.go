// internal/events/bus.go
package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Bus is an in-memory event bus implementation.
type Bus struct {
	mu         sync.RWMutex
	handlers   map[EventType]map[string]Handler
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	eventChan  chan Event
	bufferSize int
}

// NewBus creates a new event bus.
func NewBus(logger *zap.Logger, bufferSize int) *Bus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &Bus{
		handlers:   make(map[EventType]map[string]Handler),
		logger:     logger.Named("event_bus"),
		ctx:        ctx,
		cancel:     cancel,
		eventChan:  make(chan Event, bufferSize),
		bufferSize: bufferSize,
	}

	// Start the event processing goroutine
	bus.wg.Add(1)
	go bus.processEvents()

	return bus
}

// Subscribe registers a handler for a specific event type.
func (b *Bus) Subscribe(eventType EventType, handler Handler) Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := uuid.New().String()

	if b.handlers[eventType] == nil {
		b.handlers[eventType] = make(map[string]Handler)
	}

	b.handlers[eventType][id] = handler

	b.logger.Debug("Handler subscribed",
		zap.String("event_type", string(eventType)),
		zap.String("subscription_id", id))

	return &subscription{
		id:       id,
		eventBus: b,
		typ:      eventType,
	}
}

// SubscribeFunc is a convenience method for subscribing with a function.
func (b *Bus) SubscribeFunc(eventType EventType, fn func(context.Context, Event) error) Subscription {
	return b.Subscribe(eventType, HandlerFunc(fn))
}

// Publish sends an event to all registered handlers asynchronously.
func (b *Bus) Publish(event Event) error {
	select {
	case <-b.ctx.Done():
		return fmt.Errorf("event bus is shutting down")
	case b.eventChan <- event:
		return nil
	default:
		// Channel is full, log and drop the event
		b.logger.Warn("Event channel full, dropping event",
			zap.String("event_type", string(event.Type())))
		return fmt.Errorf("event channel full")
	}
}

// PublishSync sends an event to all registered handlers synchronously.
func (b *Bus) PublishSync(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.Type()]
	// Make a copy to avoid holding the lock during handler execution
	handlersCopy := make(map[string]Handler, len(handlers))
	for id, h := range handlers {
		handlersCopy[id] = h
	}
	b.mu.RUnlock()

	if len(handlersCopy) == 0 {
		return nil
	}

	var errs []error
	for id, handler := range handlersCopy {
		if err := handler.Handle(ctx, event); err != nil {
			b.logger.Error("Handler error",
				zap.String("event_type", string(event.Type())),
				zap.String("handler_id", id),
				zap.Error(err))
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("handlers failed: %v", errs)
	}

	return nil
}

// processEvents is the main event processing loop.
func (b *Bus) processEvents() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			// Drain remaining events
			for {
				select {
				case event := <-b.eventChan:
					_ = b.PublishSync(context.Background(), event)
				default:
					return
				}
			}
		case event := <-b.eventChan:
			// Process event in a separate goroutine to avoid blocking
			b.wg.Add(1)
			go func(e Event) {
				defer b.wg.Done()
				if err := b.PublishSync(b.ctx, e); err != nil {
					b.logger.Error("Failed to process event",
						zap.String("event_type", string(e.Type())),
						zap.Error(err))
				}
			}(event)
		}
	}
}

// unsubscribe removes a handler subscription.
func (b *Bus) unsubscribe(id string, eventType EventType) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if handlers, ok := b.handlers[eventType]; ok {
		delete(handlers, id)
		if len(handlers) == 0 {
			delete(b.handlers, eventType)
		}
	}

	b.logger.Debug("Handler unsubscribed",
		zap.String("event_type", string(eventType)),
		zap.String("subscription_id", id))
}

// Shutdown gracefully shuts down the event bus.
func (b *Bus) Shutdown(ctx context.Context) error {
	b.logger.Info("Shutting down event bus")

	// Signal shutdown
	b.cancel()

	// Wait for all goroutines to finish or context to expire
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		b.logger.Info("Event bus shutdown complete")
		return nil
	case <-ctx.Done():
		b.logger.Warn("Event bus shutdown timeout")
		return ctx.Err()
	}
}

// Stats returns statistics about the event bus.
func (b *Bus) Stats() map[string]interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["buffer_size"] = b.bufferSize
	stats["pending_events"] = len(b.eventChan)
	stats["event_types"] = len(b.handlers)

	handlerCounts := make(map[string]int)
	for eventType, handlers := range b.handlers {
		handlerCounts[string(eventType)] = len(handlers)
	}
	stats["handlers_per_type"] = handlerCounts

	return stats
}
