// internal/protocol/registry.go
package protocol

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// Registry manages protocol registrations.
type Registry struct {
	mu        sync.RWMutex
	protocols map[string]Protocol
	byType    map[Type][]Protocol
	logger    *zap.Logger
}

// NewRegistry creates a new protocol registry.
func NewRegistry(logger *zap.Logger) *Registry {
	return &Registry{
		protocols: make(map[string]Protocol),
		byType:    make(map[Type][]Protocol),
		logger:    logger.Named("protocol_registry"),
	}
}

// Register adds a protocol to the registry.
func (r *Registry) Register(name string, p Protocol) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.protocols[name]; exists {
		return fmt.Errorf("protocol %s already registered", name)
	}

	r.protocols[name] = p

	// Also index by type
	pType := p.GetType()
	r.byType[pType] = append(r.byType[pType], p)

	r.logger.Info("Protocol registered",
		zap.String("name", name),
		zap.String("type", string(pType)))

	return nil
}

// Get retrieves a protocol by name.
func (r *Registry) Get(name string) (Protocol, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.protocols[name]
	if !exists {
		return nil, fmt.Errorf("protocol %s not found", name)
	}

	return p, nil
}

// GetByType retrieves all protocols of a specific type.
func (r *Registry) GetByType(t Type) []Protocol {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid race conditions
	protocols := r.byType[t]
	result := make([]Protocol, len(protocols))
	copy(result, protocols)

	return result
}

// List returns all registered protocol names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.protocols))
	for name := range r.protocols {
		names = append(names, name)
	}

	return names
}

// Unregister removes a protocol from the registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.protocols[name]
	if !exists {
		return fmt.Errorf("protocol %s not found", name)
	}

	delete(r.protocols, name)

	// Remove from type index
	pType := p.GetType()
	typeProtocols := r.byType[pType]
	for i, proto := range typeProtocols {
		if proto.GetName() == name {
			r.byType[pType] = append(typeProtocols[:i], typeProtocols[i+1:]...)
			break
		}
	}

	r.logger.Info("Protocol unregistered", zap.String("name", name))

	return nil
}

// Clear removes all protocols from the registry.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.protocols = make(map[string]Protocol)
	r.byType = make(map[Type][]Protocol)

	r.logger.Info("Registry cleared")
}

// DefaultRegistry is the global protocol registry.
var DefaultRegistry = &Registry{
	protocols: make(map[string]Protocol),
	byType:    make(map[Type][]Protocol),
}

// Register adds a protocol to the default registry.
func Register(name string, p Protocol) error {
	return DefaultRegistry.Register(name, p)
}

// Get retrieves a protocol from the default registry.
func Get(name string) (Protocol, error) {
	return DefaultRegistry.Get(name)
}

// GetByType retrieves protocols by type from the default registry.
func GetByType(t Type) []Protocol {
	return DefaultRegistry.GetByType(t)
}
