package gateway

import (
	"io"
	"net/http"

	"kerberos/internal/dispatcher"
)

// Gateway is the HTTP gateway that receives requests and dispatches them.
type Gateway struct {
	addr       string
	dispatcher *dispatcher.Dispatcher
	route      dispatcher.RouteFunc
}

// Config for the gateway.
type Config struct {
	Addr       string
	Dispatcher *dispatcher.Dispatcher
	Route      dispatcher.RouteFunc
}

// New creates a new gateway.
func New(cfg Config) *Gateway {
	return &Gateway{
		addr:       cfg.Addr,
		dispatcher: cfg.Dispatcher,
		route:      cfg.Route,
	}
}

// Handler returns the HTTP handler for the gateway. Useful for testing.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleRequest)
	return mux
}

// Start begins listening for HTTP requests.
func (g *Gateway) Start() error {
	return http.ListenAndServe(g.addr, g.Handler())
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
