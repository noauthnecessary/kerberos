package registry

import (
	"sync"
)

// Instance represents a single instance of a service.
type Instance struct {
	ID     string // Unique instance identifier
	Addr   string // Address (e.g., "http://localhost:8081")
	Weight int    // Optional. >= 1 enables weighted LB; < 1 or 0 falls back to unweighted
}

// Service represents a named service with one or more instances.
type Service struct {
	Name      string
	Instances []Instance
}

// Registry holds registered services and their instances.
type Registry struct {
	mu       sync.RWMutex
	services map[string][]Instance
}

// New creates a new service registry.
func New() *Registry {
	return &Registry{
		services: make(map[string][]Instance),
	}
}

// Register adds or updates an instance for a service.
// If the instance ID already exists, it replaces the address.
func (r *Registry) Register(serviceName string, instance Instance) {
	r.mu.Lock()
	defer r.mu.Unlock()

	instances := r.services[serviceName]
	for i, inst := range instances {
		if inst.ID == instance.ID {
			instances[i] = instance
			return
		}
	}
	r.services[serviceName] = append(instances, instance)
}

// Unregister removes an instance from a service.
func (r *Registry) Unregister(serviceName string, instanceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	instances := r.services[serviceName]
	for i, inst := range instances {
		if inst.ID == instanceID {
			r.services[serviceName] = append(instances[:i], instances[i+1:]...)
			return
		}
	}
}

// GetInstances returns all instances for a service, or nil if not found.
func (r *Registry) GetInstances(serviceName string) []Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()

	instances := r.services[serviceName]
	if len(instances) == 0 {
		return nil
	}
	// Return a copy to avoid races
	result := make([]Instance, len(instances))
	copy(result, instances)
	return result
}

// ListServices returns the names of all registered services.
func (r *Registry) ListServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.services))
	for name := range r.services {
		names = append(names, name)
	}
	return names
}
