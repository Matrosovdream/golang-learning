# 56 — Authentication & Sessions

> Part of **Part 12 — Production Web App Concerns**, the front-door pair with [57 — Web Application Security](57-web-security.md). This lesson is the **authentication** (authN — *who are you?*) complement to [43 — Authorization, RBAC & Multi-Tenancy](43-authorization-rbac-multitenancy.md) (authZ — *what may you do?*). Builds on [12 — Errors](12-errors.md), [15 — Sync & Context](15-sync-context.md) (identity in context), [20](20-http-server.md)–[21 — HTTP/REST](21-rest-api.md), and the JWT/bcrypt seen in the `auth-service-intermediate` project. Thesis: **authentication is a small number of primitives done exactly right — hash passwords with a slow KDF, mint unguessable tokens, put the identity in the request context, and know the precise trade-off between a server-side session and a stateless JWT.**

## Goals
- Store passwords correctly: a **slow, salted KDF** (`bcrypt` / `argon2id`), never a fast hash, always a **constant-time** verify.
- Mint and manage credentials: cryptographically-random **session IDs / tokens** (`crypto/rand`), **secure cookies** (`HttpOnly`/`Secure`/`SameSite`), and a server-side **session store**.
- Understand **JWTs** from the bytes up — build, sign (HMAC-SHA256), and **verify** one by hand, check `exp`/`nbf`, and defend against the **alg-confusion** attack.
- Run the token lifecycle: **access + refresh** tokens, **rotation with reuse detection**, and **revocation** (denylist) — and articulate **session vs JWT**.
- Wire real flows: an **auth middleware** that puts identity in context, **CSRF** defenses, the **OAuth2 authorization-code** flow with **PKCE**, **OIDC ID-token** validation, **TOTP** 2FA, **login rate-limiting**, and a **password-reset** flow.

## Concepts

- **Passwords need a slow, salted KDF — not SHA-256.** Fast hashes are brute-forceable (billions/sec on a GPU) and, unsalted, leak that two users share a password. Use **`bcrypt`** (`golang.org/x/crypto/bcrypt`, tunable `cost`) or **`argon2id`** (memory-hard, the current OWASP recommendation). Both embed the salt in their output; verification is **constant-time** (`CompareHashAndPassword`).
- **Compare secrets in constant time.** `==` on strings/bytes can short-circuit on the first differing byte, leaking length/prefix via timing. Use `crypto/subtle.ConstantTimeCompare` (or `hmac.Equal`) for tokens, MACs, and hashed IDs.
- **Randomness for credentials must be cryptographic.** Session IDs, API keys, CSRF and reset tokens come from `crypto/rand`, **never `math/rand`**. Encode with `base64.RawURLEncoding` for cookie/URL safety.
- **Cookies carry the session — harden them.** `HttpOnly` (JS can't read it → XSS can't steal it), `Secure` (HTTPS only), `SameSite` (Lax/Strict → CSRF defense), a sane `Path`, and an explicit `MaxAge`. Store the **hash** of a session ID server-side so a DB leak doesn't hand out live sessions.
- **Two ways to remember a logged-in user:**
  - **Server-side session** — the cookie holds an opaque ID; the data lives in a store. **Instantly revocable** (delete the record), but stateful.
  - **Stateless JWT** — a signed `header.payload.signature`; the server trusts the signature and never stores anything. Scales trivially, but **can't be revoked before `exp`** without extra state.
- **A JWT is just base64url and an HMAC.** `base64url(header) + "." + base64url(claims) + "." + base64url(HMAC-SHA256(...))`. Verification = recompute the HMAC over the first two parts and **constant-time compare** to the third, then check `exp`/`nbf`/`iat`. Understanding this by hand is what makes the attacks obvious.
- **The alg-confusion attack is the JWT footgun.** An attacker sets `alg:"none"` (drop the signature) or swaps `RS256→HS256` (sign with the *public* key as an HMAC secret). Defense: **pin the expected algorithm** in your verifier; never let the token's own header choose how it's verified.
- **The token lifecycle:** short-lived **access** token + long-lived **refresh** token; the refresh endpoint mints a new access token without re-login. **Rotate** the refresh token on every use and store it (hashed) so you can detect **reuse of a rotated token** — a signal it was stolen — and revoke the whole family. For logout on stateless JWTs, keep a short-lived **denylist** of revoked `jti`s until they expire.
- **Identity belongs in the request context.** The auth middleware verifies the credential once, then stashes the identity under an **unexported context key** (same pattern as [43](43-authorization-rbac-multitenancy.md)); handlers read the identity, never the raw token.
- **CSRF defends state-changing requests.** Cookies are sent automatically cross-site, so a POST needs proof it came from your page: **double-submit cookie** (token in a readable cookie, echoed in a header — a cross-site attacker can't read the cookie) or a server-side **synchronizer token**. `SameSite` helps but isn't a complete substitute.
- **Delegated auth is OAuth2 + OIDC.** The **authorization-code** flow redirects to a provider with a random `state` (CSRF defense for the redirect), then exchanges the returned `code` for a token. **PKCE** (`code_challenge = base64url(sha256(verifier))`) makes this safe for public clients. **OIDC** adds an **ID token** (a JWT) whose `iss`/`aud`/`exp` and signature you must validate.
- **Second factors and abuse controls.** **TOTP** (RFC 6238) is `HOTP(secret, time/30)` — an HMAC-SHA1 with dynamic truncation to 6 digits. Slow brute force with per-account **rate limiting / lockout**, and make password-reset tokens **random, single-use, expiring, and stored hashed**.

## Exercises
1. Hash a password with `bcrypt` and verify a right/wrong attempt; read the embedded `cost`.
2. Show that plain SHA-256 is unsalted (equal inputs → equal hashes); use `subtle.ConstantTimeCompare` for a token check.
3. Derive and verify an `argon2id` key (fixed salt for a repeatable demo; note the real one is random).
4. Generate two 32-byte tokens with `crypto/rand` + `base64.RawURLEncoding`; confirm they differ.
5. Set a cookie and read it back with `httptest`; then set one with `HttpOnly`/`Secure`/`SameSite`/`MaxAge` and inspect the header.
6. Build a mutex-guarded server-side session store (create/get/delete); store only the **hash** of the ID.
7. Build a JWT by hand (HMAC-SHA256), then verify it and show that tampering breaks the signature.
8. Validate `nbf`/`exp` with an injected `now`; write an auth middleware that puts the user in the context.
9. Implement access+refresh issuance, refresh **rotation with reuse detection**, and a JWT **denylist** for logout.
10. Pin the algorithm to defeat `alg:"none"`; then contrast session vs JWT revocation.
11. Do CSRF two ways (double-submit + synchronizer); run the OAuth2 code flow (with `httptest` as the provider) verifying `state`; compute a **PKCE** challenge.
12. Validate an OIDC ID token (`iss`/`aud`/`exp` + signature); implement **TOTP**; add login lockout and a single-use password-reset token.
13. Capstone: assemble register → login (session cookie) → auth middleware → `/me` → logout, driven end-to-end with `httptest`.

## Best Practices & Pitfalls
- **Never store plaintext or a fast hash of a password.** `bcrypt`/`argon2id` only. Tune the cost so a verify takes ~50–250ms.
- **Uniform login errors.** Return the same "invalid credentials" whether the email is unknown or the password is wrong — otherwise you leak which accounts exist (user enumeration).
- **Pitfall — `math/rand` for tokens.** It's predictable; a leaked seed lets an attacker forge IDs. Always `crypto/rand`.
- **Pitfall — comparing secrets with `==`.** Timing side-channel. Use `subtle.ConstantTimeCompare`/`hmac.Equal`.
- **Pitfall — trusting the JWT header's `alg`.** Pin the expected algorithm; reject `none` and unexpected algorithms. Also always check `exp`.
- **Pitfall — thinking a JWT can be "logged out".** It's valid until `exp` unless you add revocation state. Keep access-token TTLs short and revoke via the refresh token.
- **Pitfall — rotating refresh tokens without reuse detection.** Rotation without detecting replay of an old token misses the exact signal that one was stolen.
- **Set `HttpOnly` + `Secure` + `SameSite` on session cookies**, and scope `Path`. Prefer `SameSite=Lax` unless you have a cross-site reason.
- **Verify `state` before exchanging an OAuth code**, and use **PKCE** for anything that can't keep a client secret (SPAs, mobile, CLIs).

## Checklist
- [ ] I hash passwords with `bcrypt`/`argon2id` and verify in constant time; login errors are uniform.
- [ ] I mint tokens with `crypto/rand` and compare secrets with `subtle`/`hmac.Equal`.
- [ ] I set `HttpOnly`/`Secure`/`SameSite`/`MaxAge` on session cookies and can run a server-side session store.
- [ ] I can build and verify a JWT by hand, check `exp`/`nbf`, and defeat the alg-confusion attack.
- [ ] I can issue access+refresh, rotate with reuse detection, and revoke via a denylist — and I can state session-vs-JWT trade-offs.
- [ ] I put identity in the request context and defend state-changing routes with CSRF.
- [ ] I can run the OAuth2 code flow with `state` + PKCE, validate an OIDC ID token, and add TOTP / lockout / reset tokens.

## Resources
- `golang.org/x/crypto/bcrypt`: https://pkg.go.dev/golang.org/x/crypto/bcrypt · `argon2`: https://pkg.go.dev/golang.org/x/crypto/argon2
- `crypto/rand`, `crypto/subtle`, `crypto/hmac`: https://pkg.go.dev/crypto/rand · https://pkg.go.dev/crypto/subtle · https://pkg.go.dev/crypto/hmac
- `net/http` cookies: https://pkg.go.dev/net/http#Cookie · `golang.org/x/oauth2`: https://pkg.go.dev/golang.org/x/oauth2
- JWT (RFC 7519), PKCE (RFC 7636), TOTP (RFC 6238): https://datatracker.ietf.org/doc/html/rfc7519 · https://datatracker.ietf.org/doc/html/rfc7636 · https://datatracker.ietf.org/doc/html/rfc6238
- OWASP cheat sheets — Password Storage, Session Management, JWT: https://cheatsheetseries.owasp.org/
- Examples: [examples/56-authentication-sessions](examples/56-authentication-sessions/).
- Related in this plan: authorization/RBAC in [43](43-authorization-rbac-multitenancy.md); the broader attack surface in [57 — Web Application Security](57-web-security.md); identity-in-context in [15](15-sync-context.md); rate-limiting/lockout ties to [36 — Resilience](36-resilience-patterns.md).
