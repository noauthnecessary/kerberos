package dispatcher

import (
	"bytes"
	"io"
	"net/http"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
)

// Dispatcher forwards incoming HTTP requests to backend services.
type Dispatcher struct {
	balancer *balancer.Balancer
	client   *circuitbreaker.Client
}

// New creates a dispatcher.
func New(b *balancer.Balancer, c *circuitbreaker.Client) *Dispatcher {
	return &Dispatcher{
		balancer: b,
		client:   c,
	}
}

// Forward selects an instance for the service, forwards the request through
// the circuit breaker, and streams the response back.
// Returns the response and error. Caller is responsible for closing the response body.
func (d *Dispatcher) Forward(serviceName string, r *http.Request) (*http.Response, error) {
	instance := d.balancer.Select(serviceName)
	if instance == nil {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	}

	return d.client.Do(instance.Addr, r)
}

// RouteFunc maps an incoming request to a service name.
// Return empty string to indicate no match (404).
type RouteFunc func(r *http.Request) string
