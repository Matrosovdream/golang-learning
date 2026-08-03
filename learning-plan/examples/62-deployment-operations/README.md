# Step 62 — Deployment & Operations · Examples

A library of **26 examples**, split into three files by difficulty.

Two shapes (as in [60](../60-build-package/)/[61](../61-ci-github-actions/)):

- **Runnable Go** (making the app deploy-ready): a real **Output** — compiled/`gofmt`/`vet`ed/run before being added.
- **Reference config** (Compose, Kubernetes manifests, a Cloud Run service, `fly.toml`, a systemd unit): a complete, copy-pasteable file with a **Verify** note. Compose files were validated with `docker compose config`; every Kubernetes/Cloud Run **YAML was syntax-validated** with a YAML loader (no `kubectl` cluster or `kubeval` was available, so manifests are additionally **reviewed against the Kubernetes schema** — apply them with `kubectl apply --dry-run=server` against a real cluster to confirm). The `fly.toml` and systemd unit are reviewed references.

The four deploy targets — **Docker Compose**, **Kubernetes**, a **PaaS** (Cloud Run / Fly.io), and a **bare binary under systemd** — all run the same image from [step 60](../60-build-package/).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — making the app operable (runnable Go)

- [1. Configuration from the environment](1-easy.md#1-configuration-from-the-environment)
- [2. Fail fast on missing config](1-easy.md#2-fail-fast-on-missing-config)
- [3. Graceful shutdown](1-easy.md#3-graceful-shutdown)
- [4. Liveness: /healthz](1-easy.md#4-liveness-healthz)
- [5. Readiness: /readyz](1-easy.md#5-readiness-readyz)
- [6. GOMAXPROCS in containers](1-easy.md#6-gomaxprocs-in-containers)
- [7. GOMEMLIMIT in containers](1-easy.md#7-gomemlimit-in-containers)
- [8. Bind to $PORT and log startup](1-easy.md#8-bind-to-port-and-log-startup)

### 🟡 [Medium](2-medium.md) — Compose & Kubernetes basics

- [9. Docker Compose for production](2-medium.md#9-docker-compose-for-production)
- [10. Graceful stop in Compose](2-medium.md#10-graceful-stop-in-compose)
- [11. A Kubernetes Deployment](2-medium.md#11-a-kubernetes-deployment)
- [12. A Kubernetes Service](2-medium.md#12-a-kubernetes-service)
- [13. Liveness and readiness probes](2-medium.md#13-liveness-and-readiness-probes)
- [14. Resource requests and limits](2-medium.md#14-resource-requests-and-limits)
- [15. Config with a ConfigMap](2-medium.md#15-config-with-a-configmap)
- [16. Secrets](2-medium.md#16-secrets)
- [17. Rolling updates](2-medium.md#17-rolling-updates)

### 🔴 [Hard](3-hard.md) — scaling, PaaS, systemd, capstone

- [18. Horizontal Pod Autoscaler](3-hard.md#18-horizontal-pod-autoscaler)
- [19. Ingress](3-hard.md#19-ingress)
- [20. Pod security hardening](3-hard.md#20-pod-security-hardening)
- [21. Zero-downtime shutdown in Kubernetes](3-hard.md#21-zero-downtime-shutdown-in-kubernetes)
- [22. Deploy to Cloud Run (PaaS)](3-hard.md#22-deploy-to-cloud-run-paas)
- [23. Deploy to Fly.io (PaaS)](3-hard.md#23-deploy-to-flyio-paas)
- [24. Bare binary with systemd](3-hard.md#24-bare-binary-with-systemd)
- [25. Rollouts and rollback](3-hard.md#25-rollouts-and-rollback)
- [26. Capstone: a full Kubernetes manifest set](3-hard.md#26-capstone-a-full-kubernetes-manifest-set)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
