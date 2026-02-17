package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
	"kerberos/internal/dispatcher"
	"kerberos/internal/registry"
)

func TestGateway_NotFoundForUnknownPath(t *testing.T) {
	r := registry.New()
	r.Register("echo", registry.Instance{ID: "1", Addr: "http://localhost:8081"})
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(http.DefaultClient, circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)

	route := func(r *http.Request) string {
		if strings.HasPrefix(r.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}

	gw := New(Config{
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/unknown")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGateway_ProxiesToBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response"))
	}))
	defer backend.Close()

	r := registry.New()
	r.Register("echo", registry.Instance{ID: "1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(backend.Client(), circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)

	route := func(r *http.Request) string {
		if strings.HasPrefix(r.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}

	gw := New(Config{
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/echo/foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "backend response" {
		t.Errorf("expected 'backend response', got %q", string(body))
	}
}

func TestGateway_Returns503WhenNoInstances(t *testing.T) {
	r := registry.New()
	// No instances registered
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(http.DefaultClient, circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)

	route := func(r *http.Request) string {
		return "empty-service"
	}

	gw := New(Config{
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/any")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}
