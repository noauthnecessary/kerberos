package balancer

import (
	"hash/fnv"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"kerberos/internal/registry"
)

// Strategy defines how to select an instance from a list.
type Strategy string

const (
	RoundRobin       Strategy = "round-robin"
	Random           Strategy = "random"
	WeightedRoundRobin Strategy = "weighted-round-robin"
	WeightedRandom   Strategy = "weighted-random"
	IPHash           Strategy = "ip-hash"
)

// Balancer selects service instances for forwarding.
type Balancer struct {
	mu        sync.Mutex
	indexes   map[string]*uint64
	strategy  Strategy
	registry  *registry.Registry
	rand      *rand.Rand
}

// New creates a load balancer using the given strategy and registry.
func New(strategy Strategy, reg *registry.Registry) *Balancer {
	return &Balancer{
		indexes:  make(map[string]*uint64),
		strategy: strategy,
		registry: reg,
		rand:     rand.New(rand.NewSource(rand.Int63())),
	}
}

// Select returns the next instance for the given service.
// req may be nil for strategies that don't need it (RoundRobin, Random, Weighted*).
// For IPHash, req is used to extract client IP.
func (b *Balancer) Select(serviceName string, req *http.Request) *registry.Instance {
	instances := b.registry.GetInstances(serviceName)
	if len(instances) == 0 {
		return nil
	}

	switch b.strategy {
	case RoundRobin:
		return b.selectRoundRobin(serviceName, instances)
	case Random:
		return b.selectRandom(instances)
	case WeightedRoundRobin:
		if hasValidWeights(instances) {
			return b.selectWeightedRoundRobin(serviceName, instances)
		}
		return b.selectRoundRobin(serviceName, instances)
	case WeightedRandom:
		if hasValidWeights(instances) {
			return b.selectWeightedRandom(instances)
		}
		return b.selectRandom(instances)
	case IPHash:
		return b.selectIPHash(instances, req)
	default:
		return &instances[0]
	}
}

func hasValidWeights(instances []registry.Instance) bool {
	for _, inst := range instances {
		if inst.Weight < 1 {
			return false
		}
	}
	return true
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

func (b *Balancer) selectRandom(instances []registry.Instance) *registry.Instance {
	b.mu.Lock()
	i := b.rand.Intn(len(instances))
	b.mu.Unlock()
	return &instances[i]
}

func (b *Balancer) selectWeightedRoundRobin(serviceName string, instances []registry.Instance) *registry.Instance {
	total := 0
	for _, inst := range instances {
		total += inst.Weight
	}
	if total <= 0 {
		return &instances[0]
	}

	b.mu.Lock()
	idxPtr, ok := b.indexes[serviceName]
	if !ok {
		var n uint64
		idxPtr = &n
		b.indexes[serviceName] = idxPtr
	}
	n := atomic.AddUint64(idxPtr, 1)
	b.mu.Unlock()

	slot := int((n - 1) % uint64(total))
	for i := range instances {
		slot -= instances[i].Weight
		if slot < 0 {
			return &instances[i]
		}
	}
	return &instances[len(instances)-1]
}

func (b *Balancer) selectWeightedRandom(instances []registry.Instance) *registry.Instance {
	total := 0
	for _, inst := range instances {
		total += inst.Weight
	}
	if total <= 0 {
		return &instances[0]
	}
	b.mu.Lock()
	r := b.rand.Intn(total)
	b.mu.Unlock()
	for i := range instances {
		r -= instances[i].Weight
		if r < 0 {
			return &instances[i]
		}
	}
	return &instances[len(instances)-1]
}

func (b *Balancer) selectIPHash(instances []registry.Instance, req *http.Request) *registry.Instance {
	ip := clientIP(req)
	h := fnv.New32a()
	h.Write([]byte(ip))
	hash := h.Sum32()
	i := int(hash % uint32(len(instances)))
	if i < 0 {
		i = -i
	}
	return &instances[i]
}

func clientIP(req *http.Request) string {
	if req == nil {
		return ""
	}
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For: client, proxy1, proxy2 — take first (original client)
		if idx := strings.Index(xff, ","); idx >= 0 {
			xff = xff[:idx]
		}
		return trimIP(xff)
	}
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	if host != "" {
		return host
	}
	return req.RemoteAddr
}

func trimIP(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
