package retry

import (
	"math"
	"time"
)

// Config for retry behavior.
type Config struct {
	MaxRetries    int           // Max retry attempts (0 = no retries)
	InitialBackoff time.Duration // Initial backoff between retries
	MaxBackoff    time.Duration // Max backoff cap
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:    2 * time.Second,
	}
}

// Backoff returns the delay for the given attempt (0-based).
// Uses exponential backoff: initial * 2^attempt, capped at MaxBackoff.
func (c Config) Backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := c.InitialBackoff * time.Duration(math.Pow(2, float64(attempt-1)))
	if d > c.MaxBackoff {
		d = c.MaxBackoff
	}
	return d
}
