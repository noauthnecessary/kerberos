package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
	"kerberos/internal/dispatcher"
	"kerberos/internal/gateway"
	"kerberos/internal/registry"
	"kerberos/internal/retry"
)

func main() {
	reg := registry.New()
	reg.Register("echo", registry.Instance{ID: "echo-1", Addr: "http://localhost:8081"})
	reg.Register("echo", registry.Instance{ID: "echo-2", Addr: "http://localhost:8082"})

	strategy := balancerStrategy()
	b := balancer.New(strategy, reg)

	// HTTP client with timeout for forwarded requests
	requestTimeout := requestTimeout()
	httpClient := &http.Client{Timeout: requestTimeout}

	// Circuit breaker with retry
	cbSettings := circuitbreaker.DefaultSettings()
	cbSettings.Retry = retryConfig()
	cb := circuitbreaker.New(httpClient, cbSettings)
	disp := dispatcher.New(b, cb)

	// Route by path prefix: /echo/* -> echo service
	route := func(r *http.Request) string {
		if strings.HasPrefix(r.URL.Path, "/echo") {
			return "echo"
		}
		return ""
	}

	gw := gateway.New(gateway.Config{
		Addr:       ":8080",
		Registry:   reg,
		Dispatcher: disp,
		Route:      route,
	})

	log.Printf("Kerberos gateway listening on :8080 (strategy: %s, timeout: %v)", strategy, requestTimeout)

	// Graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- gw.Start()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("Received %v, shutting down gracefully...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := gw.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	case err := <-done:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}

func balancerStrategy() balancer.Strategy {
	s := os.Getenv("BALANCER_STRATEGY")
	switch s {
	case "random":
		return balancer.Random
	case "weighted-round-robin":
		return balancer.WeightedRoundRobin
	case "weighted-random":
		return balancer.WeightedRandom
	case "ip-hash":
		return balancer.IPHash
	default:
		return balancer.RoundRobin
	}
}

func requestTimeout() time.Duration {
	s := os.Getenv("REQUEST_TIMEOUT")
	if s == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(s)
	if err != nil || sec <= 0 {
		return 30 * time.Second
	}
	return time.Duration(sec) * time.Second
}

func retryConfig() retry.Config {
	cfg := retry.DefaultConfig()
	s := os.Getenv("RETRY_MAX")
	if s != "" {
		n, err := strconv.Atoi(s)
		if err == nil && n >= 0 {
			cfg.MaxRetries = n
		}
	}
	return cfg
}
