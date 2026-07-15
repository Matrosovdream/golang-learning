# 57 — Web Application Security (OWASP for Go)

> Part of **Part 12 — Production Web App Concerns**, the front-door pair with [56 — Authentication & Sessions](56-authentication-sessions.md). Where 56 answers *who are you*, this lesson hardens *everything the request touches*. Builds on [21 — REST API](21-rest-api.md), [12 — Errors](12-errors.md), [22 — database/sql](22-database.md), and the identity/authz work in [43](43-authorization-rbac-multitenancy.md). Complements the `/security-review` skill. Thesis: **most web vulnerabilities are the same handful of mistakes — trusting input as code, trusting the client, leaking internals — and Go's stdlib already ships the right defense for each; the skill is knowing which one to reach for and applying it by default.**

## Goals
- Kill **injection**: parameterized SQL, `os/exec` with argument slices (no shell), and path-traversal-safe file access.
- Prevent **XSS** with `html/template`'s **contextual auto-escaping** — and recognise the `text/template` / `template.HTML` traps that reopen it.
- Lock down the HTTP surface: **security headers** (CSP/HSTS/nosniff/frame-options), correct **CORS**, **body-size limits**, **server timeouts**, and hardened **TLS**.
- Validate on an **allowlist**, reject **unknown JSON fields** (overposting), block **SSRF** and **open redirects**, and enforce **object-level authorization** (IDOR).
- Handle failure safely: **generic error responses** (no internal leakage), **redacted secrets** in logs, **constant-time** verification of webhooks, **rate limiting**, safe **file uploads**, and password-strength policy.

## Concepts

- **Injection = untrusted input treated as code.** The universal fix is to keep data and code separate:
  - **SQL** — always **parameterize** (`db.Query("… WHERE name = $1", input)`); never build SQL by string concatenation. The driver sends the query and the value on separate channels, so input can't become SQL.
  - **OS commands** — `exec.Command(prog, arg1, arg2…)` passes args directly to the process; there is **no shell**, so `;`, `|`, `$()` aren't interpreted. Never build a string and hand it to `sh -c`.
  - **Paths** — validate that a joined path stays under the base dir (`filepath.Rel` + a `..` check), or use Go 1.24's **`os.Root`/`os.OpenRoot`**, which enforces containment at the syscall level.
- **XSS defense is `html/template`'s contextual escaping.** It escapes the *same* value differently in HTML-body vs attribute vs `<script>` vs URL context — automatically. Two traps: **`text/template` does not escape** (never use it to build HTML), and **`template.HTML`/`template.JS`** mark a value as trusted (don't escape it) — wrapping *user* input in them reopens XSS.
- **Don't trust the client's shape of the request.** Reject **unknown JSON fields** with `Decoder.DisallowUnknownFields()` (defends against overposting/mass-assignment and typos), cap the **request body** with `http.MaxBytesReader`, and **validate every field on an allowlist** (a strict regexp or a known set) — deny by default, never blocklist "bad" characters.
- **Set the security headers.** A small middleware adds **CSP** (`default-src 'self'` — the strongest XSS backstop), **HSTS** (force HTTPS), **`X-Content-Type-Options: nosniff`** (stop MIME sniffing), **`X-Frame-Options: DENY`** (anti-clickjacking; or CSP `frame-ancestors`), and a **Referrer-Policy**.
- **CORS is an allowlist, not `*`.** Reflect the request's `Origin` **only if it's on your list**, add `Vary: Origin`, and never combine `Access-Control-Allow-Origin: *` with credentials (the browser forbids it, and it's dangerous).
- **SSRF: validate outbound URLs, and guard the dial.** Require `http`/`https`, reject IP-literal hosts in **loopback/private/link-local** ranges (e.g. the cloud metadata endpoint `169.254.169.254`). Because a hostname can resolve to an internal IP (**DNS rebinding**), also re-check the **resolved address** with a `net.Dialer.Control` hook. Same shape for **open redirects**: only allow local paths (no scheme, no host, no `//`).
- **Set timeouts and TLS floors.** The zero-value `http.Server` has **no timeouts** — a slow client (Slowloris) can pin connections; set `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` and `MaxHeaderBytes`. For TLS, set `MinVersion: tls.VersionTLS12` (prefer 1.3).
- **Fail without leaking.** Return a **generic** message + correct status to the client and log the detail server-side — never surface stack traces, DSNs, or SQL. **Redact secrets in logs** (implement `slog.LogValuer` to print `REDACTED`). Verify signatures/tokens (webhooks, MACs) with **`hmac.Equal`** (constant-time). Enforce **object-level authorization** — scope every fetch by the caller's ownership so a mismatch is a 404, not a leak (**IDOR**).
- **Go removes some classes for free.** `net/http` **sanitizes header values** (CRLF can't inject a second header), and `regexp` uses **RE2** (linear time — no catastrophic backtracking / **ReDoS**). Know these so you don't re-solve them, and know their edges (still cap input length; still guard user-supplied regexes with a size limit + timeout).
- **Round it out** with **rate limiting** (token bucket per client), **safe file uploads** (size cap + sniff the real content type with `http.DetectContentType`, don't trust the filename), and a **password policy** (length + variety; ideally a breach-list check via HIBP k-anonymity), plus **`govulncheck`** in CI to catch vulnerable dependencies.

## Exercises
1. Show a concatenated SQL string being subverted by `x' OR '1'='1`; contrast the parameterized form.
2. Render hostile input through `html/template` (escaped) and `text/template` (not) — see the difference.
3. Render the same value in four template contexts (body/attr/JS/URL) and observe the different escaping; then show the `template.HTML` bypass.
4. Run `exec.Command("echo", userInput)` and confirm shell metacharacters aren't interpreted.
5. Write `safeJoin(base, userPath)` that rejects `../../etc/passwd`; note `os.Root` as the modern approach.
6. Cap a request body with `http.MaxBytesReader` (413 on overflow); reject unknown JSON fields with `DisallowUnknownFields`.
7. Write a security-headers middleware and a correct CORS middleware (allowlisted origin only).
8. Write `validateURL` (SSRF) blocking loopback/private/link-local + non-http schemes, and `safeRedirect` (open-redirect) allowing only local paths.
9. Validate a username against a strict regexp and a role against a set; configure `http.Server` timeouts and a hardened `tls.Config`.
10. Return a generic 500 that doesn't leak an internal error; redact a token in `slog` via `LogValuer`.
11. Show IDOR: an insecure by-id fetch vs an ownership-scoped one. Show RE2 not backtracking on `^(a+)+$`.
12. Verify a webhook HMAC with `hmac.Equal`; add a token-bucket rate limiter; validate a file upload by sniffed content type; enforce a password policy.
13. Capstone: one hardened endpoint combining headers + body cap + strict decode + validation + generic errors + object-level authz, driven with `httptest`.

## Best Practices & Pitfalls
- **Parameterize every query.** No exceptions, no "it's just an int". If you must interpolate an identifier (table/column), use a strict allowlist — never user text.
- **Use `html/template` for HTML, always.** `text/template` is for non-HTML output only. Treat `template.HTML`/`JS`/`URL` as "I personally vouch this is safe" — never for user input.
- **Pitfall — validating with a blocklist.** You'll miss an encoding. Allowlist (strict pattern / known set), deny by default.
- **Pitfall — the zero-value `http.Server`.** No timeouts = Slowloris exposure. Set them, and cap request bodies + headers.
- **Pitfall — SSRF via hostname.** Blocking IP literals isn't enough; a name can resolve to `169.254.169.254`. Re-check the resolved IP at dial time.
- **Pitfall — leaking internals in errors.** `http.Error(w, err.Error(), 500)` can expose a DSN or SQL. Return a generic message; log the detail.
- **Pitfall — IDOR.** Authenticated ≠ authorized. Scope every object fetch by the caller; a not-owned resource should look like "not found".
- **Compare secrets in constant time** (`hmac.Equal`/`subtle`), and **redact secrets** before they reach logs.
- **Run `govulncheck` in CI** and keep `golang.org/x/crypto` and other deps current.

## Checklist
- [ ] I parameterize all SQL, use `exec.Command` with arg slices, and keep file access inside a base dir.
- [ ] I use `html/template` for HTML and understand contextual escaping + the `template.HTML`/`text/template` traps.
- [ ] I cap request bodies, reject unknown JSON fields, and validate inputs on an allowlist.
- [ ] I set security headers, correct CORS, server timeouts, and a TLS floor.
- [ ] I block SSRF (URL + resolved-IP) and open redirects, and enforce object-level authorization (no IDOR).
- [ ] I return generic errors, redact secrets in logs, and verify signatures with `hmac.Equal`.
- [ ] I rate-limit, validate uploads by sniffed type, enforce password strength, and run `govulncheck`.

## Resources
- OWASP Top 10 & Cheat Sheet Series: https://owasp.org/www-project-top-ten/ · https://cheatsheetseries.owasp.org/
- `html/template` (contextual escaping): https://pkg.go.dev/html/template · Go security policy & `govulncheck`: https://go.dev/security/ · https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck
- `net/http` (MaxBytesReader, Server timeouts): https://pkg.go.dev/net/http · `crypto/tls`: https://pkg.go.dev/crypto/tls · `os.Root`: https://pkg.go.dev/os#Root
- Go blog — "Robust generic functions on slices"/security posts; Filippo Valsorda's Go crypto writing: https://words.filippo.io/
- Examples: [examples/57-web-security](examples/57-web-security/).
- Related in this plan: authentication in [56](56-authentication-sessions.md); authorization/RBAC/overposting in [43](43-authorization-rbac-multitenancy.md); error style in [12](12-errors.md); the `/security-review` skill for reviewing your diffs.
