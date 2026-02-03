package balancer

import (
	"sync"
	"sync/atomic"

	"kerberos/internal/registry"
)

// Strategy defines how to select an instance from a list.
type Strategy string

const (
	RoundRobin Strategy = "round-robin"
)

// Balancer selects service instances for forwarding.
type Balancer struct {
	mu        sync.Mutex
	indexes   map[string]*uint64 // service name -> next index for round-robin
	strategy  Strategy
	registry  *registry.Registry
}

// New creates a load balancer using the given strategy and registry.
func New(strategy Strategy, reg *registry.Registry) *Balancer {
	return &Balancer{
		indexes:  make(map[string]*uint64),
		strategy: strategy,
		registry: reg,
	}
}

// Select returns the next instance for the given service.
// Returns nil if the service has no instances.
func (b *Balancer) Select(serviceName string) *registry.Instance {
	instances := b.registry.GetInstances(serviceName)
	if len(instances) == 0 {
		return nil
	}

	switch b.strategy {
	case RoundRobin:
		return b.selectRoundRobin(serviceName, instances)
	default:
		return &instances[0]
	}
}

func (b *Balancer) selectRoundRobin(serviceName string, instances []registry.Instance) *registry.Instance {
	b.mu.Lock()
	idxPtr, ok := b.indexes[serviceName]
	if !ok {
		var n uint64
		idxPtr = &n
		b.indexes[serviceName] = idxPtr
	}
	b.mu.Unlock()

	n := atomic.AddUint64(idxPtr, 1)
	i := int(n-1) % len(instances)
	return &instances[i]
}
