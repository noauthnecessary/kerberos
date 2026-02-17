package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"kerberos/internal/balancer"
	"kerberos/internal/circuitbreaker"
	"kerberos/internal/dispatcher"
	"kerberos/internal/gateway"
	"kerberos/internal/registry"
)

func main() {
	reg := registry.New()
	reg.Register("echo", registry.Instance{ID: "echo-1", Addr: "http://localhost:8081"})
	reg.Register("echo", registry.Instance{ID: "echo-2", Addr: "http://localhost:8082"})

	strategy := balancerStrategy()
	b := balancer.New(strategy, reg)
	cb := circuitbreaker.New(http.DefaultClient, circuitbreaker.DefaultSettings())
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

	log.Printf("Kerberos gateway listening on :8080 (strategy: %s)", strategy)
	if err := gw.Start(); err != nil {
		log.Fatal(err)
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
