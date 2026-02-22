package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"kerberos/internal/dispatcher"
	"kerberos/internal/registry"
)

// Gateway is the HTTP gateway that receives requests and dispatches them.
type Gateway struct {
	addr       string
	registry   *registry.Registry
	dispatcher *dispatcher.Dispatcher
	route      dispatcher.RouteFunc
	server     *http.Server
}

// Config for the gateway.
type Config struct {
	Addr       string
	Registry   *registry.Registry // optional, enables POST/DELETE /register
	Dispatcher *dispatcher.Dispatcher
	Route      dispatcher.RouteFunc
}

// New creates a new gateway.
func New(cfg Config) *Gateway {
	return &Gateway{
		addr:       cfg.Addr,
		registry:   cfg.Registry,
		dispatcher: cfg.Dispatcher,
		route:      cfg.Route,
	}
}

// registerRequest for POST /register.
type registerRequest struct {
	Service string `json:"service"`
	ID      string `json:"id"`
	Addr    string `json:"addr"`
	Weight  int    `json:"weight,omitempty"` // optional; >= 1 for weighted LB, < 1 falls back to unweighted
}

// unregisterRequest for DELETE /register.
type unregisterRequest struct {
	Service string `json:"service"`
	ID      string `json:"id"`
}

// Handler returns the HTTP handler for the gateway. Useful for testing.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", g.handleRegister)
	mux.HandleFunc("/services", g.handleServices)
	mux.HandleFunc("/", g.handleRequest)
	return mux
}

func (g *Gateway) handleRegister(w http.ResponseWriter, r *http.Request) {
	if g.registry == nil {
		http.Error(w, "registration not enabled", http.StatusNotImplemented)
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Service == "" || req.ID == "" || req.Addr == "" {
			http.Error(w, "service, id, and addr are required", http.StatusBadRequest)
			return
		}
		g.registry.Register(req.Service, registry.Instance{ID: req.ID, Addr: req.Addr, Weight: req.Weight})
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		var req unregisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Service == "" || req.ID == "" {
			http.Error(w, "service and id are required", http.StatusBadRequest)
			return
		}
		g.registry.Unregister(req.Service, req.ID)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) handleServices(w http.ResponseWriter, r *http.Request) {
	if g.registry == nil {
		http.Error(w, "registry not enabled", http.StatusNotImplemented)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services := g.registry.ListServices()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// Start begins listening for HTTP requests. Blocks until the server stops.
func (g *Gateway) Start() error {
	g.server = &http.Server{
		Addr:         g.addr,
		Handler:      g.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return g.server.ListenAndServe()
}

// Shutdown gracefully stops the gateway. Waits for in-flight requests to complete
// up to the context deadline.
func (g *Gateway) Shutdown(ctx context.Context) error {
	if g.server == nil {
		return nil
	}
	return g.server.Shutdown(ctx)
}

func (g *Gateway) handleRequest(w http.ResponseWriter, r *http.Request) {
	serviceName := g.route(r)
	if serviceName == "" {
		http.NotFound(w, r)
		return
	}

	resp, err := g.dispatcher.Forward(serviceName, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
