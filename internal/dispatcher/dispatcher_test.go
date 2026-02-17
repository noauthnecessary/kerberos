package dispatcher

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
	"kerberos/internal/registry"
)

func TestDispatcher_Forward_NoInstancesReturns503(t *testing.T) {
	r := registry.New()
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(http.DefaultClient, circuitbreaker.DefaultSettings())
	disp := New(b, cb)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := disp.Forward("nonexistent", req)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestDispatcher_Forward_ProxiesToBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/foo" {
			t.Errorf("backend got path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	r := registry.New()
	r.Register("svc", registry.Instance{ID: "1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(backend.Client(), circuitbreaker.DefaultSettings())
	disp := New(b, cb)

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	resp, err := disp.Forward("svc", req)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(body))
	}
}

func TestDispatcher_Forward_PreservesMethodAndBody(t *testing.T) {
	var gotMethod string
	var gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	r := registry.New()
	r.Register("svc", registry.Instance{ID: "1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(backend.Client(), circuitbreaker.DefaultSettings())
	disp := New(b, cb)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	resp, err := disp.Forward("svc", req)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodPost {
		t.Errorf("backend got method %s", gotMethod)
	}
	if gotBody != "hello" {
		t.Errorf("backend got body %q", gotBody)
	}
}
