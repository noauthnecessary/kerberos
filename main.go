package main

import (
	"log"
	"net/http"
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

	b := balancer.New(balancer.RoundRobin, reg)
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
		Dispatcher: disp,
		Route:      route,
	})

	log.Println("Kerberos gateway listening on :8080")
	if err := gw.Start(); err != nil {
		log.Fatal(err)
	}
}
