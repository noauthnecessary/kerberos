package gateway

import (
	"bytes"
	"encoding/json"
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

func gwWithRegistry(t *testing.T) (*Gateway, *registry.Registry, *httptest.Server) {
	t.Helper()
	r := registry.New()
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(http.DefaultClient, circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)
	route := func(req *http.Request) string {
		if strings.HasPrefix(req.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}
	gw := New(Config{
		Registry:   r,
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	return gw, r, srv
}

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

func TestGateway_POST_Register(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend"))
	}))
	defer backend.Close()

	r := registry.New()
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(backend.Client(), circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)
	route := func(req *http.Request) string {
		if strings.HasPrefix(req.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}
	gw := New(Config{
		Registry:   r,
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := registerRequest{
		Service: "echo",
		ID:      "inst-1",
		Addr:    backend.URL,
	}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify instance is registered by proxying
	proxyResp, err := http.Get(srv.URL + "/echo/")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after register, got %d", proxyResp.StatusCode)
	}
}

func TestGateway_DELETE_Register(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	r := registry.New()
	r.Register("echo", registry.Instance{ID: "inst-1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	cb := circuitbreaker.New(backend.Client(), circuitbreaker.DefaultSettings())
	disp := dispatcher.New(b, cb)
	route := func(req *http.Request) string {
		if strings.HasPrefix(req.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}
	gw := New(Config{
		Registry:   r,
		Dispatcher: disp,
		Route:      route,
	})
	srv := httptest.NewServer(gw.Handler())
	defer srv.Close()

	body := unregisterRequest{
		Service: "echo",
		ID:      "inst-1",
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/register", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	// Verify instance is gone - should get 503
	proxyResp, err := http.Get(srv.URL + "/echo/")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after unregister, got %d", proxyResp.StatusCode)
	}
}

func TestGateway_GET_Services(t *testing.T) {
	_, r, srv := gwWithRegistry(t)
	defer srv.Close()

	r.Register("echo", registry.Instance{ID: "1", Addr: "http://a"})
	r.Register("users", registry.Instance{ID: "1", Addr: "http://b"})

	resp, err := http.Get(srv.URL + "/services")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var services []string
	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %v", services)
	}
}

func TestGateway_Register_InvalidJSON(t *testing.T) {
	_, _, srv := gwWithRegistry(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader([]byte("{")))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGateway_Register_MissingFields(t *testing.T) {
	_, _, srv := gwWithRegistry(t)
	defer srv.Close()

	body := registerRequest{Service: "echo"}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
