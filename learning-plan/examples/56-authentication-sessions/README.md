# Step 56 — Authentication & Sessions · Examples

A library of **26 runnable examples**, split into three files by difficulty. Each is a complete
`package main` program: read the concept and steps, then **retype the code block** into a scratch
folder and run it.

**Run any example:**

```bash
mkdir -p /tmp/go-ex && cd /tmp/go-ex   # once: go mod init scratch
# type the example into main.go, then:
go run .
```

Every example was compiled, `gofmt`-checked, `go vet`-ed, and run before being added — the **Output** under each one is real stdout. Mostly stdlib (`crypto/hmac`, `crypto/rand`, `crypto/subtle`, `net/http`, `httptest`); the password-hashing examples need **`golang.org/x/crypto`** (`go get golang.org/x/crypto/bcrypt golang.org/x/crypto/argon2`). JWT, PKCE, OIDC, and TOTP are **hand-rolled from stdlib crypto** so you see how they work; OAuth2/OIDC use `httptest` as a stand-in provider.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–8 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 9–17 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 18–26 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Just ask and I'll append them to the right tier file.

## Index

### 🟢 [Easy](1-easy.md)

- [1. Hash a password with bcrypt](1-easy.md#1-hash-a-password-with-bcrypt)
- [2. Why not plain SHA-256](1-easy.md#2-why-not-plain-sha-256)
- [3. Argon2id password hashing](1-easy.md#3-argon2id-password-hashing)
- [4. Generate a secure random token](1-easy.md#4-generate-a-secure-random-token)
- [5. Set and read a cookie](1-easy.md#5-set-and-read-a-cookie)
- [6. Harden cookie attributes](1-easy.md#6-harden-cookie-attributes)
- [7. A server-side session store](1-easy.md#7-a-server-side-session-store)
- [8. Hash session IDs at rest](1-easy.md#8-hash-session-ids-at-rest)

### 🟡 [Medium](2-medium.md)

- [9. Build a JWT by hand](2-medium.md#9-build-a-jwt-by-hand)
- [10. Verify a JWT](2-medium.md#10-verify-a-jwt)
- [11. Check token expiry claims](2-medium.md#11-check-token-expiry-claims)
- [12. Auth middleware with context identity](2-medium.md#12-auth-middleware-with-context-identity)
- [13. Access and refresh tokens](2-medium.md#13-access-and-refresh-tokens)
- [14. Refresh rotation and reuse detection](2-medium.md#14-refresh-rotation-and-reuse-detection)
- [15. Revoke a stateless JWT (denylist)](2-medium.md#15-revoke-a-stateless-jwt-denylist)
- [16. Session vs JWT: the revocation trade-off](2-medium.md#16-session-vs-jwt-the-revocation-trade-off)
- [17. Defend against the alg-confusion attack](2-medium.md#17-defend-against-the-alg-confusion-attack)

### 🔴 [Hard](3-hard.md)

- [18. CSRF: double-submit cookie](3-hard.md#18-csrf-double-submit-cookie)
- [19. CSRF: synchronizer token](3-hard.md#19-csrf-synchronizer-token)
- [20. OAuth2 authorization code flow](3-hard.md#20-oauth2-authorization-code-flow)
- [21. PKCE for public clients](3-hard.md#21-pkce-for-public-clients)
- [22. Validate an OIDC ID token](3-hard.md#22-validate-an-oidc-id-token)
- [23. TOTP two-factor codes](3-hard.md#23-totp-two-factor-codes)
- [24. Login rate limiting and lockout](3-hard.md#24-login-rate-limiting-and-lockout)
- [25. A secure password reset flow](3-hard.md#25-a-secure-password-reset-flow)
- [26. Capstone: a full auth service](3-hard.md#26-capstone-a-full-auth-service)

---
*Global progress: [../../PROGRESS.md](../../PROGRESS.md).*
