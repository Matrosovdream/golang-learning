# Step 62 — Deployment & Operations · 🔴 Hard

Examples **18–26**. Scaling and security in Kubernetes, the other two deploy targets (a **PaaS** and
**systemd**), rollback, and a full manifest-set capstone. Reference files (validated where tooling allows).

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

---

## 18. Horizontal Pod Autoscaler

`🔴 hard` · *scaling*

An **HPA** scales the replica count automatically based on a metric — usually CPU. When average CPU across pods exceeds the target, it adds pods (up to `maxReplicas`); when load drops, it removes them (down to `minReplicas`). Requires resource **`requests`** (example 14) so there's a baseline to measure against.

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: app }
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: app
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target: { type: Utilization, averageUtilization: 70 }
```

**Verify:** YAML-validated (`autoscaling/v2`). Above 70% average CPU the HPA adds pods toward `maxReplicas: 10`; below, it scales back to `minReplicas: 2`. It reads CPU from the pods' `requests`, so those must be set.

---

## 19. Ingress

`🔴 hard` · *networking*

A **Service** is internal; an **Ingress** routes external HTTP(S) traffic to it — host/path rules plus TLS termination — via an ingress controller (nginx, Traefik, a cloud LB). One Ingress can front many services by host or path.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app
  annotations: { cert-manager.io/cluster-issuer: letsencrypt }
spec:
  tls:
    - hosts: [api.acme.com]
      secretName: app-tls
  rules:
    - host: api.acme.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: app
                port: { number: 80 }
```

**Verify:** YAML-validated. Requests to `https://api.acme.com/` route to the `app` Service on port 80; the `cert-manager` annotation + `tls` block provision a certificate into the `app-tls` Secret.

---

## 20. Pod security hardening

`🔴 hard` · *security*

The image is hardened at build time ([step 60](../60-build-package/)); enforce it again at the pod level with a **`securityContext`**. Run as **nonroot**, with a **read-only root filesystem**, no privilege escalation, and **all Linux capabilities dropped** — defense in depth ([step 57](../57-web-security/)). A static Go service needs none of what you're removing.

```yaml
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 65532
    fsGroup: 65532
  containers:
    - name: app
      image: ghcr.io/acme/app:1.4.2
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities: { drop: ["ALL"] }
      # a read-only root FS needs an explicit writable mount for any temp files:
      volumeMounts: [{ name: tmp, mountPath: /tmp }]
  volumes: [{ name: tmp, emptyDir: {} }]
```

**Verify:** YAML-validated. `runAsNonRoot` + `readOnlyRootFilesystem` + `drop: ["ALL"]` is the baseline every pod should have; the `emptyDir` at `/tmp` covers the one place a read-only-root app still needs to write.

---

## 21. Zero-downtime shutdown in Kubernetes

`🔴 hard` · *lifecycle*

The full drain choreography. When a pod is deleted, two things happen **in parallel**: it's removed from Service endpoints *and* it gets SIGTERM. There's a race — the LB may still send requests for a moment. The fix: a **`preStop`** hook that sleeps (so endpoint removal propagates first) and a **`terminationGracePeriodSeconds`** longer than `preStop` + your drain, plus the app's readiness flip (easy #5).

```yaml
spec:
  terminationGracePeriodSeconds: 40   # must exceed preStop + drain time
  containers:
    - name: app
      image: ghcr.io/acme/app:1.4.2
      readinessProbe:
        httpGet: { path: /readyz, port: 8080 }
      lifecycle:
        preStop:
          exec:
            # let endpoint removal propagate before we start draining
            command: ["sleep", "10"]
```

**The sequence:** pod marked Terminating → (a) removed from endpoints, (b) `preStop sleep 10` runs → SIGTERM sent → app flips `/readyz` to 503 and drains in-flight requests (easy #3/#5) → exits → SIGKILL only if it overruns the 40s grace period.

**Verify:** YAML-validated. `terminationGracePeriodSeconds` (40) > `preStop` (10) + your max drain — otherwise SIGKILL cuts requests off. This closes the endpoint-removal race that a bare SIGTERM handler alone still loses.

---

## 22. Deploy to Cloud Run (PaaS)

`🔴 hard` · *paas*

A **PaaS** trades control for simplicity: hand it the image from [step 60](../60-build-package/) and it runs, scales (to zero), and terminates TLS for you. **Google Cloud Run** deploys with one command, or declaratively via a Knative `Service`. It sets `$PORT` (easy #8) and scales on concurrency.

```bash
gcloud run deploy app \
  --image ghcr.io/acme/app:1.4.2 \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars LOG_LEVEL=info \
  --cpu 1 --memory 512Mi --concurrency 80 --min-instances 0 --max-instances 10
```

```yaml
# service.yaml — the declarative (Knative) equivalent
apiVersion: serving.knative.dev/v1
kind: Service
metadata: { name: app }
spec:
  template:
    spec:
      containerConcurrency: 80
      containers:
        - image: ghcr.io/acme/app:1.4.2
          ports: [{ containerPort: 8080 }]
          env: [{ name: LOG_LEVEL, value: info }]
          resources: { limits: { cpu: "1", memory: 512Mi } }
```

**Verify:** the `service.yaml` is YAML-validated; the `gcloud run deploy` command is the imperative form. Cloud Run injects `$PORT`, scales to zero when idle (`min-instances 0`), and sends SIGTERM on scale-down — so your graceful shutdown (easy #3) still matters.

---

## 23. Deploy to Fly.io (PaaS)

`🔴 hard` · *paas*

**Fly.io** runs your container close to users with a small `fly.toml`. `fly launch` scaffolds it and `fly deploy` ships it; the `[http_service]` block wires the port, and `[[http_service.checks]]` points at your `/healthz`.

```toml
# fly.toml
app = "acme-app"
primary_region = "iad"

[build]
  image = "ghcr.io/acme/app:1.4.2"

[env]
  LOG_LEVEL = "info"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = true      # scale to zero when idle
  min_machines_running = 0

  [[http_service.checks]]
    method = "GET"
    path = "/healthz"
    interval = "10s"
    timeout = "2s"
```

**Verify:** reviewed reference (TOML — no validator was available in this environment). `fly deploy` builds/pushes and rolls it out; the health check targets `/healthz` (easy #4), and `auto_stop_machines` scales to zero — again relying on graceful shutdown.

---

## 24. Bare binary with systemd

`🔴 hard` · *systemd*

The simplest deploy of all: Go's single static binary needs no runtime, so you can run it directly on a VM under **systemd** — no container. The unit restarts on failure, loads config from an env file, runs as a dedicated nonroot user, and adds sandboxing directives.

```ini
# /etc/systemd/system/app.service
[Unit]
Description=acme app
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/app
EnvironmentFile=/etc/app/app.env
User=app
Group=app
Restart=on-failure
RestartSec=2
# sandboxing (defense in depth)
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
# graceful shutdown: systemd sends SIGTERM, then SIGKILL after this
TimeoutStopSec=30

[Install]
WantedBy=multi-user.target
```

**Verify:** reviewed reference. Install then `systemctl enable --now app`; `systemctl status app` shows it running, `journalctl -u app -f` tails logs. `TimeoutStopSec=30` gives the graceful shutdown (easy #3) time to drain; `Restart=on-failure` recovers from crashes.

---

## 25. Rollouts and rollback

`🔴 hard` · *operations*

Deploying is only half the job — you need to **watch** a rollout and **undo** a bad one fast. Kubernetes keeps a revision history, so a rollback is a single command (no rebuild). `kubectl rollout status` gates a deploy; `undo` reverts to the previous known-good ReplicaSet.

```bash
kubectl set image deployment/app app=ghcr.io/acme/app:1.5.0   # trigger a rollout
kubectl rollout status deployment/app --timeout=120s          # wait; non-zero = failed
kubectl rollout history deployment/app                        # list revisions

# something's wrong -> revert instantly to the previous revision
kubectl rollout undo deployment/app
kubectl rollout undo deployment/app --to-revision=3           # or a specific one
```

**Verify:** these are the standard `kubectl rollout` commands. `rollout status` returns non-zero if the new pods never become ready (readiness gates the roll — example 13/17), which lets CI [step 61](../61-ci-github-actions.md) fail the deploy and trigger `undo` automatically.

---

## 26. Capstone: a full Kubernetes manifest set

`🔴 hard` · *capstone*

Everything for one service in a single applyable file: **ConfigMap** + **Secret** for config, a **Deployment** with the [step 60](../60-build-package/) image — probes, resources tied to `GOMEMLIMIT`, rolling-update strategy, nonroot security context, graceful-shutdown wiring — a **Service**, and an **HPA**. This is the production manifest set the [step 61](../61-ci-github-actions.md) pipeline deploys.

```yaml
apiVersion: v1
kind: ConfigMap
metadata: { name: app-config }
data: { LOG_LEVEL: "info", TIMEOUT_MS: "5000" }
---
apiVersion: v1
kind: Secret
metadata: { name: app-secrets }
type: Opaque
stringData: { JWT_SECRET: "change-me" }
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: app, labels: { app: app } }
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate: { maxSurge: 1, maxUnavailable: 0 }
  selector: { matchLabels: { app: app } }
  template:
    metadata: { labels: { app: app } }
    spec:
      terminationGracePeriodSeconds: 40
      securityContext: { runAsNonRoot: true, runAsUser: 65532 }
      containers:
        - name: app
          image: ghcr.io/acme/app:1.4.2
          ports: [{ containerPort: 8080 }]
          envFrom:
            - configMapRef: { name: app-config }
          env:
            - name: JWT_SECRET
              valueFrom: { secretKeyRef: { name: app-secrets, key: JWT_SECRET } }
            - name: GOMEMLIMIT
              value: "450MiB"
          resources:
            requests: { cpu: "250m", memory: "128Mi" }
            limits: { cpu: "1", memory: "512Mi" }
          livenessProbe:
            httpGet: { path: /healthz, port: 8080 }
            initialDelaySeconds: 5
          readinessProbe:
            httpGet: { path: /readyz, port: 8080 }
            initialDelaySeconds: 3
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
          lifecycle:
            preStop: { exec: { command: ["sleep", "10"] } }
          volumeMounts: [{ name: tmp, mountPath: /tmp }]
      volumes: [{ name: tmp, emptyDir: {} }]
---
apiVersion: v1
kind: Service
metadata: { name: app }
spec:
  selector: { app: app }
  ports: [{ port: 80, targetPort: 8080 }]
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata: { name: app }
spec:
  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: app }
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource: { name: cpu, target: { type: Utilization, averageUtilization: 70 } }
```

**Verify:** YAML-validated (five documents). This single `kubectl apply -f app.yaml` stands up config, secrets, a hardened rolling Deployment with probes + runtime limits, a Service, and autoscaling — the complete deploy for the image built in [step 60](../60-build-package/) and pushed by CI in [step 61](../61-ci-github-actions.md).

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
