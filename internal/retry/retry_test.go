package retry

import (
	"testing"
	"time"
)

func TestConfig_Backoff(t *testing.T) {
	cfg := Config{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
	}

	if d := cfg.Backoff(0); d != 0 {
		t.Errorf("Backoff(0): want 0, got %v", d)
	}
	if d := cfg.Backoff(1); d != 100*time.Millisecond {
		t.Errorf("Backoff(1): want 100ms, got %v", d)
	}
	if d := cfg.Backoff(2); d != 200*time.Millisecond {
		t.Errorf("Backoff(2): want 200ms, got %v", d)
	}
	if d := cfg.Backoff(5); d != cfg.MaxBackoff {
		t.Errorf("Backoff(5): want capped at MaxBackoff, got %v", d)
	}
}
