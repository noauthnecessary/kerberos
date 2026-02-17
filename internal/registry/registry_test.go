package registry

import (
	"testing"
)

func TestRegistry_Register_GetInstances(t *testing.T) {
	r := New()

	r.Register("echo", Instance{ID: "inst-1", Addr: "http://localhost:8081"})
	r.Register("echo", Instance{ID: "inst-2", Addr: "http://localhost:8082"})

	instances := r.GetInstances("echo")
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}

	addrMap := make(map[string]string)
	for _, inst := range instances {
		addrMap[inst.ID] = inst.Addr
	}
	if addrMap["inst-1"] != "http://localhost:8081" {
		t.Errorf("inst-1 addr: got %s", addrMap["inst-1"])
	}
	if addrMap["inst-2"] != "http://localhost:8082" {
		t.Errorf("inst-2 addr: got %s", addrMap["inst-2"])
	}
}

func TestRegistry_GetInstances_EmptyReturnsNil(t *testing.T) {
	r := New()

	instances := r.GetInstances("nonexistent")
	if instances != nil {
		t.Errorf("expected nil for unknown service, got %v", instances)
	}
}

func TestRegistry_Register_UpdatesExisting(t *testing.T) {
	r := New()

	r.Register("echo", Instance{ID: "inst-1", Addr: "http://localhost:8081"})
	r.Register("echo", Instance{ID: "inst-1", Addr: "http://localhost:9999"})

	instances := r.GetInstances("echo")
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance after update, got %d", len(instances))
	}
	if instances[0].Addr != "http://localhost:9999" {
		t.Errorf("expected updated addr, got %s", instances[0].Addr)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := New()

	r.Register("echo", Instance{ID: "inst-1", Addr: "http://localhost:8081"})
	r.Register("echo", Instance{ID: "inst-2", Addr: "http://localhost:8082"})

	r.Unregister("echo", "inst-1")

	instances := r.GetInstances("echo")
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance after unregister, got %d", len(instances))
	}
	if instances[0].ID != "inst-2" {
		t.Errorf("expected inst-2, got %s", instances[0].ID)
	}
}

func TestRegistry_Unregister_UnknownInstanceNoOp(t *testing.T) {
	r := New()

	r.Register("echo", Instance{ID: "inst-1", Addr: "http://localhost:8081"})
	r.Unregister("echo", "nonexistent")

	instances := r.GetInstances("echo")
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance (unregister unknown should be no-op), got %d", len(instances))
	}
}

func TestRegistry_ListServices(t *testing.T) {
	r := New()

	if names := r.ListServices(); len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}

	r.Register("echo", Instance{ID: "1", Addr: "http://a"})
	r.Register("users", Instance{ID: "1", Addr: "http://b"})

	names := r.ListServices()
	if len(names) != 2 {
		t.Fatalf("expected 2 services, got %d", len(names))
	}
	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	if !seen["echo"] || !seen["users"] {
		t.Errorf("expected echo and users, got %v", names)
	}
}
