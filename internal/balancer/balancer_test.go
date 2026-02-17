package balancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kerberos/internal/registry"
)

func TestBalancer_Select_RoundRobin(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a"})
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b"})
	r.Register("echo", registry.Instance{ID: "c", Addr: "http://c"})

	b := New(RoundRobin, r)

	// Call Select 6 times; should cycle a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}
	for i, want := range expected {
		inst := b.Select("echo", nil)
		if inst == nil {
			t.Fatalf("Select %d: got nil", i)
		}
		if inst.ID != want {
			t.Errorf("Select %d: want ID %s, got %s", i, want, inst.ID)
		}
	}
}

func TestBalancer_Select_NoInstancesReturnsNil(t *testing.T) {
	r := registry.New()
	b := New(RoundRobin, r)

	inst := b.Select("nonexistent", nil)
	if inst != nil {
		t.Errorf("expected nil for unknown service, got %v", inst)
	}
}

func TestBalancer_Select_SingleInstance(t *testing.T) {
	r := registry.New()
	r.Register("single", registry.Instance{ID: "only", Addr: "http://only"})
	b := New(RoundRobin, r)

	for i := 0; i < 3; i++ {
		inst := b.Select("single", nil)
		if inst == nil || inst.ID != "only" {
			t.Errorf("Select %d: want only instance, got %v", i, inst)
		}
	}
}

func TestBalancer_Select_Random(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a"})
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b"})
	b := New(Random, r)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		inst := b.Select("echo", nil)
		if inst == nil {
			t.Fatalf("Select %d: got nil", i)
		}
		seen[inst.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("Random should hit both instances over 50 calls, saw %v", seen)
	}
}

func TestBalancer_Select_WeightedRoundRobin(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a", Weight: 2})
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b", Weight: 1})
	b := New(WeightedRoundRobin, r)

	// Over 6 calls: a should appear 4x, b 2x (2:1 ratio)
	counts := make(map[string]int)
	for i := 0; i < 6; i++ {
		inst := b.Select("echo", nil)
		if inst == nil {
			t.Fatalf("Select %d: got nil", i)
		}
		counts[inst.ID]++
	}
	if counts["a"] != 4 || counts["b"] != 2 {
		t.Errorf("weighted round-robin 2:1: want a=4 b=2, got %v", counts)
	}
}

func TestBalancer_Select_WeightedRoundRobin_NoWeightsFallsBackToRoundRobin(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a"}) // Weight 0
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b"}) // Weight 0
	b := New(WeightedRoundRobin, r)

	expected := []string{"a", "b", "a", "b"}
	for i, want := range expected {
		inst := b.Select("echo", nil)
		if inst == nil || inst.ID != want {
			t.Errorf("Select %d: want %s, got %v", i, want, inst)
		}
	}
}

func TestBalancer_Select_WeightedRandom_NoWeightsFallsBackToRandom(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a"})
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b"})
	b := New(WeightedRandom, r)

	seen := make(map[string]bool)
	for i := 0; i < 30; i++ {
		inst := b.Select("echo", nil)
		if inst == nil {
			t.Fatalf("Select %d: got nil", i)
		}
		seen[inst.ID] = true
	}
	if len(seen) < 2 {
		t.Errorf("weighted random with no weights should fall back to random, saw %v", seen)
	}
}

func TestBalancer_Select_IPHash(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "a", Addr: "http://a"})
	r.Register("echo", registry.Instance{ID: "b", Addr: "http://b"})
	b := New(IPHash, r)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	first := b.Select("echo", req)
	if first == nil {
		t.Fatal("Select: got nil")
	}
	// Same IP -> same instance (sticky)
	for i := 0; i < 5; i++ {
		inst := b.Select("echo", req)
		if inst == nil || inst.ID != first.ID {
			t.Errorf("IP hash: same IP should return same instance, got %v", inst)
		}
	}
}
