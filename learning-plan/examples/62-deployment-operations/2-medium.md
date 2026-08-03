# Step 62 — Deployment & Operations · 🟡 Medium

Examples **9–17**. Production **Docker Compose**, then **Kubernetes** basics — Deployment, Service,
probes, resources, config, secrets, rollouts. Complete reference files (validated).

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟢 easy](1-easy.md) · Next: [🔴 hard](3-hard.md)

---

## 9. Docker Compose for production

`🟡 medium` · *compose*

The dev Compose from [step 60](../60-build-package/) grows a few production concerns: **`restart`** (survive crashes/reboots), **resource limits**, an **`env_file`** for config, and **log rotation** so a chatty container doesn't fill the disk.

```yaml
services:
  app:
    image: ghcr.io/acme/app:1.4.2
    restart: unless-stopped          # auto-restart on crash / after reboot
    ports: ["8080:8080"]
    env_file: [.env]                 # 12-factor config, kept out of the image
    deploy:
      resources:
        limits: { cpus: "1.0", memory: 512M }
    logging:
      driver: json-file
      options: { max-size: "10m", max-file: "3" }   # cap log disk usage
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 10s
      timeout: 3s
      retries: 3
```

**Verify:** validated with `docker compose config` (parses + normalizes). `restart: unless-stopped` keeps it running across reboots; the healthcheck hits the `/healthz` from easy #4.

---

## 10. Graceful stop in Compose

`🟡 medium` · *compose*

On `docker compose down`/`stop`, Docker sends **SIGTERM**, waits **`stop_grace_period`**, then SIGKILL. Set the grace period longer than your worst-case request drain so the graceful shutdown from easy #3 can finish. `stop_signal` overrides the signal if your app expects a different one.

```yaml
services:
  app:
    image: ghcr.io/acme/app:1.4.2
    stop_grace_period: 30s     # time to drain before SIGKILL (default is 10s)
    stop_signal: SIGTERM       # the signal Docker sends (default; shown for clarity)
    ports: ["8080:8080"]
```

**Verify:** `docker compose config` accepts it. On `docker compose stop`, the container gets SIGTERM and up to 30s to drain in-flight requests (easy #3) before being killed — pair the two so no request is cut off.

---

## 11. A Kubernetes Deployment

`🟡 medium` · *kubernetes*

A **Deployment** declares the desired state: N replicas of a pod running your image. Kubernetes keeps that many running and manages rolling updates. The `selector` must match the pod template `labels` — that's how the Deployment finds its pods.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  labels: { app: app }
spec:
  replicas: 3
  selector:
    matchLabels: { app: app }
  template:
    metadata:
      labels: { app: app }
    spec:
      containers:
        - name: app
          image: ghcr.io/acme/app:1.4.2
          ports:
            - containerPort: 8080
```

**Verify:** YAML-validated. `kubectl apply -f deployment.yaml` creates 3 pods; `kubectl scale deployment/app --replicas=5` changes the count; the controller reconciles to match `spec.replicas`.

---

## 12. A Kubernetes Service

`🟡 medium` · *kubernetes*

Pods are ephemeral (new IP on every restart), so you never talk to them directly. A **Service** gives a stable virtual IP + DNS name (`app.default.svc.cluster.local`) and load-balances across the pods its `selector` matches. `ClusterIP` (the default) is internal; `LoadBalancer`/`Ingress` expose it externally.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: app
spec:
  type: ClusterIP
  selector: { app: app }     # routes to pods with this label
  ports:
    - port: 80               # the Service port
      targetPort: 8080       # the container port
```

**Verify:** YAML-validated. The `selector` matches the Deployment's pod labels; other pods reach it at `http://app:80`. `kubectl get endpoints app` lists the pod IPs behind it.

---

## 13. Liveness and readiness probes

`🟡 medium` · *kubernetes*

Probes wire Kubernetes to the endpoints from easy #4–5. The **livenessProbe** restarts a pod that stops responding; the **readinessProbe** removes a pod from the Service's endpoints until it's ready (and again during shutdown). `initialDelaySeconds`/`periodSeconds`/`failureThreshold` tune the timing.

```yaml
spec:
  containers:
    - name: app
      image: ghcr.io/acme/app:1.4.2
      ports: [{ containerPort: 8080 }]
      livenessProbe:            # restart if this fails (cheap, no deps)
        httpGet: { path: /healthz, port: 8080 }
        initialDelaySeconds: 5
        periodSeconds: 10
        failureThreshold: 3
      readinessProbe:           # remove from LB until ready / while draining
        httpGet: { path: /readyz, port: 8080 }
        initialDelaySeconds: 3
        periodSeconds: 5
```

**Verify:** YAML-validated. Liveness → `/healthz` (restart on failure); readiness → `/readyz` (routing gate). Keep liveness dependency-free so a DB blip doesn't trigger a restart loop.

---

## 14. Resource requests and limits

`🟡 medium` · *kubernetes*

**`requests`** are what the pod is guaranteed (used for scheduling); **`limits`** are the hard cap (exceed memory → OOM-killed; exceed CPU → throttled). Crucially, these should drive the Go runtime: set **`GOMAXPROCS`** to the CPU limit and **`GOMEMLIMIT`** just below the memory limit (easy #6–7).

```yaml
spec:
  containers:
    - name: app
      image: ghcr.io/acme/app:1.4.2
      resources:
        requests: { cpu: "250m", memory: "128Mi" }   # guaranteed / scheduled on
        limits:   { cpu: "1000m", memory: "512Mi" }   # hard cap
      env:
        - name: GOMEMLIMIT
          value: "450MiB"       # a bit below the 512Mi limit (headroom)
        - name: GOMAXPROCS
          value: "1"            # match the CPU limit (or use automaxprocs)
```

**Verify:** YAML-validated. The `env` vars tie the container limits to the Go runtime — the single most important fix for "Go uses too much CPU/memory in Kubernetes" (easy #6–7).

---

## 15. Config with a ConfigMap

`🟡 medium` · *kubernetes*

Non-secret config lives in a **ConfigMap**, surfaced to the container as env vars (or mounted files) — keeping [step 60](../60-build-package/)'s image generic and the config per-environment. `envFrom` injects every key as an env var.

```yaml
apiVersion: v1
kind: ConfigMap
metadata: { name: app-config }
data:
  LOG_LEVEL: "info"
  TIMEOUT_MS: "5000"
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: app }
spec:
  selector: { matchLabels: { app: app } }
  template:
    metadata: { labels: { app: app } }
    spec:
      containers:
        - name: app
          image: ghcr.io/acme/app:1.4.2
          envFrom:
            - configMapRef: { name: app-config }   # inject all keys as env vars
```

**Verify:** YAML-validated (two documents separated by `---`). The ConfigMap's keys arrive as `LOG_LEVEL`/`TIMEOUT_MS` env vars — read by the config loader from easy #1.

---

## 16. Secrets

`🟡 medium` · *kubernetes*

Credentials go in a **Secret**, never a ConfigMap or the image. Kubernetes stores them base64-encoded (use `stringData` to write plaintext) and can encrypt them at rest; reference them as env vars via `secretKeyRef`. In production, back Secrets with an external manager (Vault, cloud secret stores).

```yaml
apiVersion: v1
kind: Secret
metadata: { name: app-secrets }
type: Opaque
stringData:                       # plaintext in; stored base64-encoded
  JWT_SECRET: "super-secret-value"
  DATABASE_URL: "postgres://app:pw@db:5432/app"
---
# in the Deployment's container spec:
#   env:
#     - name: JWT_SECRET
#       valueFrom:
#         secretKeyRef: { name: app-secrets, key: JWT_SECRET }
```

**Verify:** YAML-validated. `stringData` lets you write plaintext (kubectl encodes it); the container reads `JWT_SECRET` via `secretKeyRef`. Never commit real secrets — this belongs in a secret manager, not Git.

---

## 17. Rolling updates

`🟡 medium` · *kubernetes*

A **RollingUpdate** replaces pods gradually so there's no downtime: **`maxSurge`** allows extra pods during the roll, **`maxUnavailable`** caps how many can be down. New pods receive traffic only once **ready** (example 13), so a broken image never takes the service down.

```yaml
spec:
  replicas: 4
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1            # up to 1 extra pod during the update
      maxUnavailable: 0      # never drop below the desired count (safest)
  minReadySeconds: 5        # a pod must stay ready 5s before it "counts"
  selector: { matchLabels: { app: app } }
  template:
    metadata: { labels: { app: app } }
    spec:
      containers: [{ name: app, image: "ghcr.io/acme/app:1.4.2" }]  # quote: ':' in a flow mapping
```

**Verify:** YAML-validated. `maxUnavailable: 0` + `maxSurge: 1` rolls one pod at a time with no capacity dip; `minReadySeconds` guards against a pod that passes readiness then immediately crashes.

---

> Next tier: [🔴 hard](3-hard.md) · Prev: [🟢 easy](1-easy.md) · Back to the [index](README.md)
