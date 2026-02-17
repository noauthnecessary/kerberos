package balancer

import (
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
		inst := b.Select("echo")
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

	inst := b.Select("nonexistent")
	if inst != nil {
		t.Errorf("expected nil for unknown service, got %v", inst)
	}
}

func TestBalancer_Select_SingleInstance(t *testing.T) {
	r := registry.New()
	r.Register("single", registry.Instance{ID: "only", Addr: "http://only"})
	b := New(RoundRobin, r)

	for i := 0; i < 3; i++ {
		inst := b.Select("single")
		if inst == nil || inst.ID != "only" {
			t.Errorf("Select %d: want only instance, got %v", i, inst)
		}
	}
}
