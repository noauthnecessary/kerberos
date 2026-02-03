# Kerberos

A Go gateway with service registry, load balancer, and circuit breaker. Receives HTTP requests and dispatches them to backend services.

## Features

- **Service Registry** – In-memory registry for services and instances
- **Load Balancer** – Round-robin selection across instances
- **Circuit Breaker** – Per-backend circuit breaker to prevent cascading failures
- **HTTP Gateway** – Single entry point that routes by path prefix

## Project Structure

```
kerberos/
├── main.go                 # Entry point, wiring
├── internal/
│   ├── registry/           # Service registry
│   ├── balancer/           # Load balancer (round-robin)
│   ├── circuitbreaker/     # Circuit breaker wrapper
│   ├── dispatcher/         # Request forwarding
│   └── gateway/            # HTTP server
└── README.md
```

## Usage

### Run the gateway

```bash
go run .
```

The gateway listens on `:8080`. Routes are configured in `main.go` – by default, `/echo/*` is routed to the `echo` service.

### Register services

Edit `main.go` to register your services:

```go
reg.Register("myservice", registry.Instance{ID: "inst-1", Addr: "http://localhost:9001"})
reg.Register("myservice", registry.Instance{ID: "inst-2", Addr: "http://localhost:9002"})
```

### Routing

Implement a `RouteFunc` that maps requests to service names. Example (path prefix):

```go
route := func(r *http.Request) string {
    if strings.HasPrefix(r.URL.Path, "/api/users") {
        return "users-service"
    }
    if strings.HasPrefix(r.URL.Path, "/api/orders") {
        return "orders-service"
    }
    return ""
}
```

## Try it

1. Start a simple echo server on 8081 and 8082 (e.g. `python -m http.server 8081`)
2. Run the gateway: `go run .`
3. `curl http://localhost:8080/echo/` – requests will be load-balanced across backends

## Circuit Breaker

Each backend has its own circuit breaker. After 5 consecutive failures, the circuit opens and requests fail fast. After 30 seconds, it moves to half-open and allows a few probe requests.
