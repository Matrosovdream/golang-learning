# Step 60 — Building & Packaging · Examples

A library of **26 examples**, split into three files by difficulty.

Because this topic is half Go and half **build/container config**, the examples come in two shapes:

- **Runnable** (Go programs + `go build` commands): each has a real **Output** block — compiled/run/`gofmt`/`vet`ed before being added.
- **Reference config** (Dockerfile, `compose.yaml`, Makefile): a complete, copy-pasteable file with a **Verify** note stating how it was checked (`docker build`, `docker compose config`, etc.).

**Run/verify:** the Go examples work with `go run .` / `go build`; the container examples were validated with a local Docker daemon (`docker build` — the multi-stage image builds to a **~650 KB** `scratch` image — and `docker compose config`).

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md) — the `go build` toolchain

- [1. Compile and run](1-easy.md#1-compile-and-run)
- [2. Build outputs and install](1-easy.md#2-build-outputs-and-install)
- [3. Cross-compile for any OS/arch](1-easy.md#3-cross-compile-for-any-osarch)
- [4. A static binary](1-easy.md#4-a-static-binary)
- [5. Inject a version with -ldflags](1-easy.md#5-inject-a-version-with--ldflags)
- [6. Read build metadata at runtime](1-easy.md#6-read-build-metadata-at-runtime)
- [7. Shrink the binary](1-easy.md#7-shrink-the-binary)
- [8. Build tags](1-easy.md#8-build-tags)

### 🟡 [Medium](2-medium.md) — embedding & Docker

- [9. Embed a file](2-medium.md#9-embed-a-file)
- [10. Embed a directory](2-medium.md#10-embed-a-directory)
- [11. Serve embedded files over HTTP](2-medium.md#11-serve-embedded-files-over-http)
- [12. A naive Dockerfile (anti-pattern)](2-medium.md#12-a-naive-dockerfile-anti-pattern)
- [13. A multi-stage Dockerfile](2-medium.md#13-a-multi-stage-dockerfile)
- [14. A scratch / distroless runtime](2-medium.md#14-a-scratch--distroless-runtime)
- [15. .dockerignore](2-medium.md#15-dockerignore)
- [16. Version the image with a build ARG](2-medium.md#16-version-the-image-with-a-build-arg)
- [17. Reproducible builds](2-medium.md#17-reproducible-builds)

### 🔴 [Hard](3-hard.md) — compose, security, capstone

- [18. Layer caching for fast rebuilds](3-hard.md#18-layer-caching-for-fast-rebuilds)
- [19. Docker Compose for local dev](3-hard.md#19-docker-compose-for-local-dev)
- [20. Compose healthchecks and dependencies](3-hard.md#20-compose-healthchecks-and-dependencies)
- [21. A Makefile](3-hard.md#21-a-makefile)
- [22. Image security hardening](3-hard.md#22-image-security-hardening)
- [23. Multi-arch images](3-hard.md#23-multi-arch-images)
- [24. A /version endpoint](3-hard.md#24-a-version-endpoint)
- [25. Vendoring for reproducible CI](3-hard.md#25-vendoring-for-reproducible-ci)
- [26. Capstone: a production Dockerfile](3-hard.md#26-capstone-a-production-dockerfile)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
