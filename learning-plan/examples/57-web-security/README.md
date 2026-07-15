# Step 57 — Web Application Security · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. **Stdlib-only** (`html/template`, `net/http`, `httptest`, `crypto/*`, `os/exec`, `regexp`); no external dependencies. Attacks are demonstrated against `httptest` servers so everything runs offline.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. SQL injection vs parameterized queries](1-easy.md#1-sql-injection-vs-parameterized-queries)
- [2. XSS: html/template vs text/template](1-easy.md#2-xss-htmltemplate-vs-texttemplate)
- [3. Contextual output escaping](1-easy.md#3-contextual-output-escaping)
- [4. The template.HTML bypass trap](1-easy.md#4-the-templatehtml-bypass-trap)
- [5. Command injection and os/exec](1-easy.md#5-command-injection-and-osexec)
- [6. Path traversal](1-easy.md#6-path-traversal)
- [7. Limit the request body size](1-easy.md#7-limit-the-request-body-size)
- [8. Reject unknown JSON fields](1-easy.md#8-reject-unknown-json-fields)

### 🟡 [Medium](2-medium.md)

- [9. Security headers middleware](2-medium.md#9-security-headers-middleware)
- [10. CORS done right](2-medium.md#10-cors-done-right)
- [11. SSRF: validate outbound URLs](2-medium.md#11-ssrf-validate-outbound-urls)
- [12. Open redirect defense](2-medium.md#12-open-redirect-defense)
- [13. Allowlist input validation](2-medium.md#13-allowlist-input-validation)
- [14. Server timeouts](2-medium.md#14-server-timeouts)
- [15. Harden TLS](2-medium.md#15-harden-tls)
- [16. Don't leak internal errors](2-medium.md#16-dont-leak-internal-errors)
- [17. Redact secrets in logs](2-medium.md#17-redact-secrets-in-logs)

### 🔴 [Hard](3-hard.md)

- [18. IDOR: object-level authorization](3-hard.md#18-idor-object-level-authorization)
- [19. ReDoS and Go's RE2 engine](3-hard.md#19-redos-and-gos-re2-engine)
- [20. Stop MIME sniffing on downloads](3-hard.md#20-stop-mime-sniffing-on-downloads)
- [21. HTTP header injection is blocked](3-hard.md#21-http-header-injection-is-blocked)
- [22. Verify a webhook signature](3-hard.md#22-verify-a-webhook-signature)
- [23. Token-bucket rate limiting](3-hard.md#23-token-bucket-rate-limiting)
- [24. Validate a file upload](3-hard.md#24-validate-a-file-upload)
- [25. Password strength policy](3-hard.md#25-password-strength-policy)
- [26. Capstone: a hardened endpoint](3-hard.md#26-capstone-a-hardened-endpoint)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
