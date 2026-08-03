# Step 62 — Deployment & Operations · 🟢 Easy

Examples **1–8**. Making the app **deploy-ready** — config, shutdown, health, and the container
runtime knobs. All runnable Go with a real **Output**.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Next tier: [🟡 medium](2-medium.md)

---

## 1. Configuration from the environment

`🟢 easy` · *config*

A deployable service reads its config from the **environment** (the 12-factor rule): one source of truth, no config files in the image, trivially overridden per environment. Read each var with a typed default so the app runs with zero config in dev and full config in prod.

**Steps:**

1. `getenv(key, default)` returns the env var or a fallback.
2. Parse into typed fields (int port, `time.Duration`).
3. The platform sets `PORT`; unset vars use defaults.

```go
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is loaded from the environment (12-factor): one source of truth, no config
// files baked into the image, trivially overridden per environment.
type Config struct {
	Port     int
	LogLevel string
	Timeout  time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func load() Config {
	port, _ := strconv.Atoi(getenv("PORT", "8080"))
	ms, _ := strconv.Atoi(getenv("TIMEOUT_MS", "5000"))
	return Config{Port: port, LogLevel: getenv("LOG_LEVEL", "info"), Timeout: time.Duration(ms) * time.Millisecond}
}

func main() {
	os.Setenv("PORT", "9090") // normally set by the platform, not the program
	cfg := load()
	fmt.Printf("port=%d log=%s timeout=%s\n", cfg.Port, cfg.LogLevel, cfg.Timeout)
}
```

**Output:**

```
port=9090 log=info timeout=5s
```

---

## 2. Fail fast on missing config

`🟢 easy` · *config*

Required config (a database URL, a signing secret) should be checked **at startup**, not discovered later when it's first used and the app crashes mid-request. Validate up front and abort with a clear message listing everything that's missing.

**Steps:**

1. `requireEnv(keys...)` collects the missing ones.
2. A non-empty list is a fatal startup error.
3. Once all are set, the check passes.

```go
package main

import (
	"fmt"
	"os"
	"strings"
)

// requireEnv fails FAST at startup with a clear message, instead of crashing later
// when a missing DATABASE_URL is first used.
func requireEnv(keys ...string) error {
	var missing []string
	for _, k := range keys {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func main() {
	os.Setenv("DATABASE_URL", "postgres://...")
	// JWT_SECRET deliberately unset -> startup should abort.
	if err := requireEnv("DATABASE_URL", "JWT_SECRET"); err != nil {
		fmt.Println("FATAL:", err)
	}
	os.Setenv("JWT_SECRET", "s3cr3t")
	fmt.Println("after setting it:", requireEnv("DATABASE_URL", "JWT_SECRET"))
}
```

**Output:**

```
FATAL: missing required config: JWT_SECRET
after setting it: <nil>
```

---

## 3. Graceful shutdown

`🟢 easy` · *shutdown*

Docker and Kubernetes send **SIGTERM** to stop a container (then SIGKILL after a grace period). Trap it, stop accepting new connections, and **drain** in-flight requests with `http.Server.Shutdown` before exiting — so a deploy never cuts a request off mid-flight.

**Steps:**

1. `signal.NotifyContext(ctx, SIGINT, SIGTERM)` cancels `ctx` on the signal.
2. Block on `<-ctx.Done()`, then `srv.Shutdown(timeoutCtx)` drains.
3. (The example sends itself SIGTERM to simulate the platform.)

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Trap SIGINT/SIGTERM (what Docker/k8s send on stop). NotifyContext cancels ctx
	// when a signal arrives, so we can drain in-flight requests before exiting.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: "127.0.0.1:0"}
	go func() { _ = srv.ListenAndServe() }()
	fmt.Println("server started")

	// Simulate the platform sending SIGTERM (normally `docker stop` / pod deletion).
	go func() { time.Sleep(50 * time.Millisecond); syscall.Kill(syscall.Getpid(), syscall.SIGTERM) }()

	<-ctx.Done()
	fmt.Println("signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Println("shutdown error:", err)
	}
	fmt.Println("stopped cleanly")
}
```

**Output:**

```
server started
signal received, draining...
stopped cleanly
```

---

## 4. Liveness: /healthz

`🟢 easy` · *health*

A **liveness** probe answers "is the process alive and not deadlocked?". Keep it **cheap** and **dependency-free**: it must not check the database, because a DB blip would make the orchestrator kill and restart an otherwise-healthy pod — turning a small outage into a restart storm.

**Steps:**

1. `/healthz` returns 200 unconditionally (the process is up).
2. No dependency checks here — those belong in readiness (example 5).
3. Kubernetes restarts the pod if this fails repeatedly.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

// Liveness: "is the process alive and not deadlocked?" Keep it CHEAP and DON'T check
// dependencies — a flaky DB shouldn't make k8s kill and restart the pod.
func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func main() {
	srv := httptest.NewServer(http.HandlerFunc(healthz))
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/healthz")
	fmt.Println("GET /healthz ->", resp.StatusCode)
	resp.Body.Close()
}
```

**Output:**

```
GET /healthz -> 200
```

---

## 5. Readiness: /readyz

`🟢 easy` · *health*

A **readiness** probe answers "can I serve traffic **right now**?". Unlike liveness, it *may* check dependencies — and critically, it must **flip to not-ready during shutdown** so the load balancer stops routing to a pod while it drains. Back it with an `atomic.Bool` you toggle.

**Steps:**

1. `/readyz` returns 200 only when `ready` is true, else 503.
2. Flip to ready once dependencies are connected.
3. Flip to not-ready when shutdown begins → the LB stops sending traffic.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// Readiness: "can I serve traffic right now?" It MAY check deps, and MUST flip to
// not-ready during shutdown so the load balancer stops sending requests while draining.
type Server struct{ ready atomic.Bool }

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

func main() {
	s := &Server{}
	srv := httptest.NewServer(http.HandlerFunc(s.readyz))
	defer srv.Close()
	get := func() int {
		resp, err := http.Get(srv.URL)
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	fmt.Println("before ready:", get()) // 503
	s.ready.Store(true)                 // deps connected -> ready
	fmt.Println("serving:     ", get()) // 200
	s.ready.Store(false)                // shutdown begins -> stop taking traffic
	fmt.Println("draining:    ", get()) // 503
}
```

**Output:**

```
before ready: 503
serving:      200
draining:     503
```

---

## 6. GOMAXPROCS in containers

`🟢 easy` · *runtime*

**`GOMAXPROCS`** sets how many OS threads run Go code in parallel. It **defaults to the host's core count** — which is wrong in a container: on a 64-core node with a `500m` CPU limit, Go spins up 64 workers for half a core, causing scheduler contention and CPU throttling. Match it to the CPU limit.

**Steps:**

1. `runtime.GOMAXPROCS(0)` queries the current value (defaults to `NumCPU`).
2. In production, set it to the CPU limit — via the `GOMAXPROCS` env var or `go.uber.org/automaxprocs` (reads the cgroup quota).
3. `runtime.GOMAXPROCS(2)` sets it explicitly.

```go
package main

import (
	"fmt"
	"runtime"
)

func main() {
	// GOMAXPROCS = OS threads running Go code in parallel. It defaults to the HOST core
	// count — WRONG in a container with a CPU limit (e.g. a 64-core node, 500m limit),
	// causing scheduler contention and CPU throttling.
	fmt.Println("defaults to NumCPU:", runtime.GOMAXPROCS(0) == runtime.NumCPU())

	// In a container, match it to the CPU limit: the GOMAXPROCS env var, or import
	// go.uber.org/automaxprocs which reads the cgroup quota automatically.
	runtime.GOMAXPROCS(2) // e.g. a "2 CPU" limit
	fmt.Println("after GOMAXPROCS(2):", runtime.GOMAXPROCS(0))
}
```

**Output:**

```
defaults to NumCPU: true
after GOMAXPROCS(2): 2
```

---

## 7. GOMEMLIMIT in containers

`🟢 easy` · *runtime*

**`GOMEMLIMIT`** is a **soft** memory limit: as the heap approaches it, the GC works harder to stay under — avoiding the **OOM-kill** that hits a memory-limited container whose heap grows past its cap. Set it a little *below* the container's memory limit (headroom for goroutine stacks and off-heap memory).

**Steps:**

1. `debug.SetMemoryLimit(bytes)` sets the soft limit (env form: `GOMEMLIMIT=450MiB`).
2. It returns the previous limit — unlimited (`math.MaxInt64`) by default.
3. `SetMemoryLimit(-1)` queries without changing.

```go
package main

import (
	"fmt"
	"runtime/debug"
)

func main() {
	// GOMEMLIMIT is a SOFT memory limit: the GC works harder as the heap nears it,
	// avoiding OOM-kills in a memory-limited container. Set it a bit BELOW the container
	// limit (headroom for stacks/off-heap). Env form: GOMEMLIMIT=450MiB.
	prev := debug.SetMemoryLimit(450 << 20) // 450 MiB
	cur := debug.SetMemoryLimit(-1)         // -1 queries without changing
	fmt.Println("default was unlimited:", prev == 9223372036854775807)
	fmt.Printf("new soft limit: %d MiB\n", cur>>20)
}
```

**Output:**

```
default was unlimited: true
new soft limit: 450 MiB
```

---

## 8. Bind to $PORT and log startup

`🟢 easy` · *config*

A container must bind to **`0.0.0.0`** (all interfaces), not `localhost`, or it's unreachable from outside — on the **`$PORT`** the platform assigns. Emit a single **structured** startup line (addr, version) so ops can confirm what came up.

**Steps:**

1. Read `$PORT` (default `8080`); `net.JoinHostPort("0.0.0.0", port)`.
2. Log a structured `slog` startup line.
3. Confirm the bind succeeds.

```go
package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
)

func main() {
	// Bind to 0.0.0.0:$PORT (all interfaces) so the container is reachable, and log one
	// structured startup line so ops can see addr/version at a glance.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := net.JoinHostPort("0.0.0.0", port)

	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ // strip time for a stable demo
		ReplaceAttr: func(g []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	log := slog.New(h)
	log.Info("starting", "addr", addr, "version", "1.4.2")

	ln, err := net.Listen("tcp", "127.0.0.1:0") // demo bind (ephemeral, no conflict)
	fmt.Println("bind ok:", err == nil)
	if ln != nil {
		ln.Close()
	}
}
```

**Output:**

```
level=INFO msg=starting addr=0.0.0.0:8080 version=1.4.2
bind ok: true
```

---

> Next tier: [🟡 medium](2-medium.md) · Back to the [index](README.md)
