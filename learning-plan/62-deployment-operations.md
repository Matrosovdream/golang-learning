# 62 — Deployment & Operations

> Part of **Part 13 — CI/CD & Deployment**, the closing lesson: [60 — Build & Package](60-build-package.md) → [61 — CI](61-ci-github-actions.md) → **62 deploy & operate**. Builds on [21 — REST API](21-rest-api.md) (graceful shutdown), [23 — Config, Logging & Observability](23-config-logging.md), [36 — Resilience](36-resilience-patterns.md), and the container-runtime concerns from [47](47-low-latency-gc-contention.md)/[48](48-low-latency-lockfree-tail.md) (`GOMEMLIMIT`/`GOMAXPROCS`); it deploys the image from [60](60-build-package.md) that CI ([61](61-ci-github-actions.md)) pushes. Thesis: **a deployable service is one that reads its config from the environment, exposes health and readiness, shuts down gracefully on SIGTERM, and tells the Go runtime about its container's CPU/memory limits. Get those right and it runs the same on Compose, Kubernetes, a PaaS, or a bare VM — the manifests just differ.**

## Goals
- Make the app **operable**: **12-factor config** from env, **fail-fast** validation, **graceful shutdown** on SIGTERM, **liveness** vs **readiness** endpoints, and `$PORT`/`0.0.0.0` binding with structured startup logging.
- Tell the runtime about the container: **`GOMAXPROCS`** (match the CPU limit) and **`GOMEMLIMIT`** (a soft memory limit that prevents OOM-kills).
- Deploy to the four targets: **Docker Compose** (production), **Kubernetes** (Deployment/Service/probes/resources/ConfigMap/Secret/HPA/Ingress/rollouts/security context), a **PaaS** (Cloud Run, Fly.io), and a **bare binary under systemd**.
- Run it safely: **rolling updates**, **zero-downtime shutdown** (readiness flip + preStop + grace period), and **rollback**.

## Concepts

- **Config comes from the environment (12-factor).** One source of truth, no config files baked into the image, overridden per environment. Read env vars with typed defaults; **fail fast at startup** with a clear message if a *required* var (DB URL, secret) is missing — don't crash later when it's first used.
- **Handle SIGTERM — that's the stop signal.** Docker and Kubernetes send **SIGTERM** to ask a container to stop, then SIGKILL after a grace period. `signal.NotifyContext(ctx, SIGINT, SIGTERM)` cancels a context on the signal; then `http.Server.Shutdown(ctx)` **drains** in-flight requests before exiting. Without this, requests are cut off mid-flight on every deploy.
- **Liveness ≠ readiness — and the difference matters.**
  - **Liveness** (`/healthz`): "is the process alive / not deadlocked?" Keep it **cheap** and **dependency-free** — a failing DB must *not* fail liveness, or the orchestrator kills and restarts a healthy pod uselessly.
  - **Readiness** (`/readyz`): "can I serve traffic **right now**?" It *may* check dependencies, and **must flip to not-ready during shutdown** so the load balancer stops routing to a pod that's draining.
- **Tell the Go runtime about the container's limits.** By default **`GOMAXPROCS`** = host core count — wrong on a 64-core node with a `500m` CPU limit (scheduler contention + throttling). Set it to the CPU limit (the `GOMAXPROCS` env var, or `go.uber.org/automaxprocs` which reads the cgroup quota). **`GOMEMLIMIT`** is a **soft** memory limit that makes the GC work harder near the ceiling, avoiding OOM-kills; set it a little *below* the container's memory limit for headroom.
- **Docker Compose for production** adds `restart: unless-stopped`, resource limits, an `env_file`, log rotation, and a **`stop_grace_period`** so your graceful shutdown has time to drain.
- **Kubernetes is a set of declarative objects.** A **Deployment** manages replicas of a pod (and rolling updates); a **Service** gives them a stable virtual IP/DNS name; **probes** (`livenessProbe`/`readinessProbe` → your `/healthz`//`/readyz`) drive restart and routing; **resource `requests`/`limits`** schedule the pod and cap it (and should drive `GOMAXPROCS`/`GOMEMLIMIT`); a **ConfigMap** holds non-secret config and a **Secret** holds credentials (both surfaced as env vars or files); an **HPA** autoscales on CPU/metrics; an **Ingress** routes external traffic; a **securityContext** runs nonroot with a read-only root FS and dropped capabilities ([57](57-web-security.md)/[60](60-build-package.md)).
- **Zero-downtime is a choreography.** A **rolling update** (`maxSurge`/`maxUnavailable`) replaces pods gradually; new pods only receive traffic once **ready**. On shutdown: the pod is removed from endpoints, a **`preStop`** hook + **`terminationGracePeriodSeconds`** give your app time to flip readiness → drain → exit before SIGKILL. `kubectl rollout status`/`undo` monitor and **roll back**.
- **A PaaS trades control for simplicity.** **Cloud Run** and **Fly.io** take your container image (from [60](60-build-package.md)) and run it — you provide a small service/`fly.toml` config (port, env, scaling, health check) and they handle the orchestration, TLS, and autoscaling.
- **A bare binary + systemd** is the simplest deploy: Go's single static binary needs no runtime, so a **systemd unit** (with `Restart=on-failure`, env, and hardening directives like `NoNewPrivileges`/`ProtectSystem`) runs it on a VM with no container at all.

## Exercises
1. Load config from env with defaults; fail fast if a required var is missing.
2. Implement graceful shutdown: trap SIGTERM, `Shutdown` the server, drain.
3. Add `/healthz` (liveness, cheap) and `/readyz` (readiness, flips to 503 on shutdown).
4. Read `GOMAXPROCS`/`NumCPU`; set a `GOMEMLIMIT`; log a structured startup line bound to `$PORT`.
5. Write a production **Compose** file (restart, limits, `stop_grace_period`) and validate it.
6. Write a Kubernetes **Deployment** + **Service** with **liveness/readiness probes** and **resource requests/limits**.
7. Add a **ConfigMap**, a **Secret**, a **rolling update** strategy, an **HPA**, and an **Ingress**.
8. Add a **securityContext** (nonroot, read-only FS, drop caps) and a **zero-downtime shutdown** setup (preStop + grace period + readiness flip).
9. Deploy the same image to **Cloud Run** and **Fly.io**; write a **systemd** unit for a bare-VM deploy; practice `kubectl rollout status`/`undo`.
10. Capstone: a complete manifest set (Deployment+Service+probes+resources+ConfigMap+Secret+HPA) for the [60](60-build-package.md) image.

## Best Practices & Pitfalls
- **Always handle SIGTERM + drain.** Otherwise every deploy drops in-flight requests. Pair it with the readiness flip so the LB stops sending new traffic first.
- **Keep liveness dependency-free.** Checking the DB in `/healthz` turns a DB blip into a restart storm. Dependencies belong in `/readyz`.
- **Set `GOMAXPROCS`/`GOMEMLIMIT` to the container's limits.** The #1 Go-in-containers footgun: the runtime sees the *node's* resources, not the pod's. Use `automaxprocs` + `GOMEMLIMIT`.
- **Set resource `requests` and `limits`.** No requests → bad scheduling; no memory limit → a leak takes down the node; a too-low CPU limit → throttling.
- **Pitfall — readiness that never fails.** A `/readyz` hardcoded to 200 can't drain and can't shed a broken dependency. Make it reflect real state.
- **Pitfall — grace period too short.** If `terminationGracePeriodSeconds` (or Compose `stop_grace_period`) is shorter than your drain, SIGKILL cuts requests off. Size it to your longest request.
- **Pitfall — secrets in the image or in Git.** Config via env/ConfigMap; **secrets** via a Secret/secret manager, never baked into the image or committed.
- **Run nonroot with a read-only root FS.** The image is hardened ([60](60-build-package.md)); enforce it again at the pod level with a `securityContext`.

## Checklist
- [ ] My app reads config from env (fail-fast), handles SIGTERM with a drain, and exposes cheap `/healthz` + real `/readyz`.
- [ ] I set `GOMAXPROCS` and `GOMEMLIMIT` to the container's CPU/memory limits.
- [ ] I can write a production Compose file (restart, limits, `stop_grace_period`).
- [ ] I can write a Deployment + Service with probes and resource requests/limits.
- [ ] I use ConfigMaps/Secrets, rolling updates, an HPA, an Ingress, and a securityContext.
- [ ] I understand zero-downtime shutdown (readiness flip + preStop + grace period) and rollback.
- [ ] I can deploy the same image to a PaaS (Cloud Run / Fly.io) and to a bare VM via systemd.

## Resources
- 12-factor app: https://12factor.net/ · graceful shutdown: `net/http` `Server.Shutdown` https://pkg.go.dev/net/http#Server.Shutdown · `os/signal` `NotifyContext` https://pkg.go.dev/os/signal#NotifyContext
- `GOMAXPROCS`/`automaxprocs`: https://pkg.go.dev/go.uber.org/automaxprocs · `GOMEMLIMIT`/`SetMemoryLimit`: https://pkg.go.dev/runtime/debug#SetMemoryLimit · the GC guide: https://go.dev/doc/gc-guide
- Kubernetes: pod lifecycle & probes https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/ · Deployments https://kubernetes.io/docs/concepts/workloads/controllers/deployment/ · resources https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
- PaaS: Cloud Run https://cloud.google.com/run/docs · Fly.io https://fly.io/docs/ · systemd unit files https://www.freedesktop.org/software/systemd/man/systemd.service.html
- Examples: [examples/62-deployment-operations](examples/62-deployment-operations/).
- Related in this plan: graceful shutdown in [21](21-rest-api.md); config/health in [23](23-config-logging.md); `GOMEMLIMIT`/`GOMAXPROCS` in [47](47-low-latency-gc-contention.md)/[48](48-low-latency-lockfree-tail.md); the image & CI in [60](60-build-package.md)/[61](61-ci-github-actions.md); pod security in [57](57-web-security.md).
