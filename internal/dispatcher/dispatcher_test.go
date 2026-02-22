package dispatcher

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
	"kerberos/internal/registry"
	"kerberos/internal/retry"
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

func TestDispatcher_Forward_RequestTimeout(t *testing.T) {
	// Backend that sleeps longer than client timeout
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Client with very short timeout
	httpClient := &http.Client{Timeout: 50 * time.Millisecond}

	r := registry.New()
	r.Register("svc", registry.Instance{ID: "1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	cbSettings := circuitbreaker.DefaultSettings()
	cbSettings.Retry = retry.Config{MaxRetries: 0} // no retries
	cb := circuitbreaker.New(httpClient, cbSettings)
	disp := New(b, cb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.Background())

	_, err := disp.Forward("svc", req)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !isTimeoutError(err) {
		t.Errorf("expected timeout error, got %v", err)
	}
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	// Also check for context.DeadlineExceeded
	return strings.Contains(err.Error(), "deadline exceeded") ||
		strings.Contains(err.Error(), "timeout")
}

func TestDispatcher_Forward_RetriesOnConnectionError(t *testing.T) {
	attempt := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt <= 2 {
			// Close connection before responding - forces retryable error
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					conn.Close()
					return
				}
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	cbSettings := circuitbreaker.DefaultSettings()
	cbSettings.Retry = retry.Config{
		MaxRetries:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:    50 * time.Millisecond,
	}
	cb := circuitbreaker.New(backend.Client(), cbSettings)

	r := registry.New()
	r.Register("svc", registry.Instance{ID: "1", Addr: backend.URL})
	b := balancer.New(balancer.RoundRobin, r)
	disp := New(b, cb)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := disp.Forward("svc", req)
	if err != nil {
		t.Fatalf("Forward: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if attempt < 3 {
		t.Errorf("expected at least 3 attempts (fail twice then succeed), got %d", attempt)
	}
}
