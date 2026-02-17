# Kerberos

A Go gateway with service registry, load balancer, and circuit breaker. Receives HTTP requests and dispatches them to backend services.

## Architecture

```mermaid
flowchart LR
    Client([Client]) --> Gateway
    Gateway --> RouteFunc{RouteFunc}
    RouteFunc -->|service name| Dispatcher
    Dispatcher --> Balancer
    Balancer --> Registry[(Registry)]
    Balancer -->|instance| CircuitBreaker
    CircuitBreaker --> Backend1[Backend 1]
    CircuitBreaker --> Backend2[Backend 2]
    CircuitBreaker --> BackendN[Backend N]
```

## Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant G as Gateway
    participant R as RouteFunc
    participant D as Dispatcher
    participant B as Balancer
    participant Reg as Registry
    participant CB as Circuit Breaker
    participant BE as Backend

    C->>G: HTTP Request
    G->>R: Route(request)
    R->>R: Match path → service name
    R-->>G: "echo"
    G->>D: Forward(service, request)
    D->>B: Select(service, request)
    B->>Reg: GetInstances(service)
    Reg-->>B: [inst-1, inst-2]
    Note right of B: Strategy: round-robin,<br/>random, weighted-*, ip-hash
    B-->>D: instance (by strategy)
    D->>CB: Do(addr, request)
    CB->>BE: HTTP forward
    BE-->>CB: Response
    CB-->>D: Response
    D-->>G: Response
    G-->>C: Response
```

## Registration Flow

```mermaid
sequenceDiagram
    participant S as Service
    participant G as Gateway
    participant R as Registry

    Note over S,R: Self-registration (startup)
    S->>G: POST /register<br/>{service, id, addr, weight?}
    G->>R: Register(service, instance)
    R-->>G: OK
    G-->>S: 204 No Content

    Note over S,R: Unregister (shutdown)
    S->>G: DELETE /register<br/>{service, id}
    G->>R: Unregister(service, id)
    R-->>G: OK
    G-->>S: 204 No Content
```

## Component Overview

```mermaid
flowchart TB
    subgraph Gateway
        RouteFunc
        HandleRegister["/register"]
        HandleServices["/services"]
    end

    subgraph Core
        Dispatcher
        Balancer["Balancer<br/>(round-robin, random,<br/>weighted-*, ip-hash)"]
        Registry[(Registry)]
        CircuitBreaker
    end

    subgraph Backends
        B1[Instance 1]
        B2[Instance 2]
    end

    HandleRegister -->|Register/Unregister| Registry
    HandleServices -->|ListServices| Registry
    RouteFunc --> Dispatcher
    Dispatcher --> Balancer
    Balancer --> Registry
    Dispatcher --> CircuitBreaker
    CircuitBreaker --> B1
    CircuitBreaker --> B2
```

## Features

- **Service Registry** – In-memory registry for services and instances
- **HTTP Registration API** – Self-register via POST/DELETE `/register`
- **Load Balancer** – Multiple strategies: round-robin, random, weighted-round-robin, weighted-random, ip-hash
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

## Load Balancing

```mermaid
flowchart TB
    subgraph Strategies
        RR[round-robin]
        RND[random]
        WRR[weighted-round-robin]
        WR[weighted-random]
        IP[ip-hash]
    end

    WRR -->|weight >= 1| Weighted["weighted selection"]
    WR -->|weight >= 1| Weighted
    WRR -->|weight &lt; 1 or omitted| RR
    WR -->|weight &lt; 1 or omitted| RND
```

| Strategy | Env Var | Description |
|----------|---------|-------------|
| `round-robin` | (default) | Cycles through instances sequentially |
| `random` | `BALANCER_STRATEGY=random` | Picks a random instance each time |
| `weighted-round-robin` | `BALANCER_STRATEGY=weighted-round-robin` | Round-robin proportional to weight. If weight &lt; 1 or omitted, falls back to round-robin |
| `weighted-random` | `BALANCER_STRATEGY=weighted-random` | Random selection proportional to weight. If weight &lt; 1 or omitted, falls back to random |
| `ip-hash` | `BALANCER_STRATEGY=ip-hash` | Same client IP → same instance (session affinity) |

Weights are set at registration. Example: `{"service":"echo","id":"inst-1","addr":"http://localhost:8081","weight":3}`. Weight ≥ 1 enables weighted strategies; weight &lt; 1 or omitted uses the unweighted variant.

## Usage

### Run the gateway

```bash
go run .
# Or with a different load balancing strategy:
BALANCER_STRATEGY=ip-hash go run .
```

The gateway listens on `:8080`. Routes are configured in `main.go` – by default, `/echo/*` is routed to the `echo` service.

### Register services

**Option 1: HTTP API (self-registration)**

```bash
# Register an instance (optional: add "weight": 3 for weighted strategies)
curl -X POST http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"service":"echo","id":"inst-1","addr":"http://localhost:8081","weight":2}'

# Unregister an instance
curl -X DELETE http://localhost:8080/register \
  -H "Content-Type: application/json" \
  -d '{"service":"echo","id":"inst-1"}'

# List registered services
curl http://localhost:8080/services
```

**Option 2: Programmatic (in `main.go`)**

```go
reg.Register("myservice", registry.Instance{ID: "inst-1", Addr: "http://localhost:9001"})
reg.Register("myservice", registry.Instance{ID: "inst-2", Addr: "http://localhost:9002", Weight: 2})
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
