package circuitbreaker

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"kerberos/internal/retry"
	"github.com/sony/gobreaker"
)

// Client wraps an HTTP client with per-target circuit breakers.
type Client struct {
	httpClient *http.Client
	breakers   map[string]*gobreaker.CircuitBreaker
	mu         sync.RWMutex
	retry      retry.Config
}

// Settings for creating a new breaker client.
type Settings struct {
	MaxRequests uint32  // Max requests when half-open
	Interval    int64   // Time window for counting failures (seconds)
	Timeout     int64   // How long circuit stays open (seconds)
	ReadyToTrip func(counts gobreaker.Counts) bool
	Retry       retry.Config // Optional; MaxRetries 0 disables retries
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
		retry:      s.Retry,
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
// Retries with exponential backoff on failure (if Retry configured).
func (c *Client) Do(target string, req *http.Request) (*http.Response, error) {
	cb := c.getBreaker(target)

	result, err := cb.Execute(func() (interface{}, error) {
		return c.doWithRetry(target, req)
	})

	if err != nil {
		return nil, err
	}
	return result.(*http.Response), nil
}

func (c *Client) doWithRetry(target string, req *http.Request) (*http.Response, error) {
	forwardURL, err := buildForwardURL(target, req.URL.Path, req.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}

	var lastErr error
	for attempt := 0; attempt <= c.retry.MaxRetries; attempt++ {
		var body io.Reader
		if len(bodyBytes) > 0 {
			body = bytes.NewReader(bodyBytes)
		}
		reqCopy, err := http.NewRequestWithContext(req.Context(), req.Method, forwardURL, body)
		if err != nil {
			return nil, err
		}
		for k, v := range req.Header {
			reqCopy.Header[k] = v
		}

		resp, err := c.httpClient.Do(reqCopy)
		if err != nil {
			lastErr = err
			if attempt < c.retry.MaxRetries {
				time.Sleep(c.retry.Backoff(attempt + 1))
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
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
