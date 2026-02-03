package circuitbreaker

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/sony/gobreaker"
)

// Client wraps an HTTP client with per-target circuit breakers.
type Client struct {
	httpClient *http.Client
	breakers   map[string]*gobreaker.CircuitBreaker
	mu         sync.RWMutex
}

// Settings for creating a new breaker client.
type Settings struct {
	MaxRequests uint32  // Max requests when half-open
	Interval    int64   // Time window for counting failures (seconds)
	Timeout     int64   // How long circuit stays open (seconds)
	ReadyToTrip func(counts gobreaker.Counts) bool
}

// DefaultSettings returns sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		MaxRequests: 3,
		Interval:    60,
		Timeout:     30,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}
}

// New creates a circuit-breaker-wrapped HTTP client.
func New(httpClient *http.Client, s Settings) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient: httpClient,
		breakers:   make(map[string]*gobreaker.CircuitBreaker),
	}
}

func (c *Client) getBreaker(target string) *gobreaker.CircuitBreaker {
	c.mu.RLock()
	cb, ok := c.breakers[target]
	c.mu.RUnlock()

	if ok {
		return cb
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok = c.breakers[target]; ok {
		return cb
	}

	cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        target,
		MaxRequests: 3,
		Interval:    60,
		Timeout:     30,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})
	c.breakers[target] = cb
	return cb
}

// Do executes the request through the circuit breaker for the target.
// target should uniquely identify the backend (e.g., "http://localhost:8081").
func (c *Client) Do(target string, req *http.Request) (*http.Response, error) {
	cb := c.getBreaker(target)

	result, err := cb.Execute(func() (interface{}, error) {
		forwardURL, err := buildForwardURL(target, req.URL.Path, req.URL.RawQuery)
		if err != nil {
			return nil, err
		}
		reqCopy, err := http.NewRequestWithContext(req.Context(), req.Method, forwardURL, req.Body)
		if err != nil {
			return nil, err
		}
		// Copy headers
		for k, v := range req.Header {
			reqCopy.Header[k] = v
		}
		return c.httpClient.Do(reqCopy)
	})

	if err != nil {
		return nil, err
	}
	return result.(*http.Response), nil
}

func buildForwardURL(base, path, rawQuery string) (string, error) {
	base = strings.TrimSuffix(base, "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path
	u.RawQuery = rawQuery
	return u.String(), nil
}
