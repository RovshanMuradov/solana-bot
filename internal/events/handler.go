// internal/events/handler.go
package events

import (
	"context"
)

// Handler processes events of a specific type.
type Handler interface {
	// Handle processes an event. Should not block.
	Handle(ctx context.Context, event Event) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as event handlers.
type HandlerFunc func(ctx context.Context, event Event) error

// Handle calls f(ctx, event).
func (f HandlerFunc) Handle(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Subscription represents a subscription to events.
type Subscription interface {
	// Unsubscribe removes the subscription.
	Unsubscribe()
}

// subscription is the internal implementation of Subscription.
type subscription struct {
	id       string
	eventBus *Bus
	typ      EventType
}

// Unsubscribe removes this subscription from the event bus.
func (s *subscription) Unsubscribe() {
	s.eventBus.unsubscribe(s.id, s.typ)
}
