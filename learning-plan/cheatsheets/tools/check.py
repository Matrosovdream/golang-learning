#!/usr/bin/env python3
"""Verify that every stdlib symbol named in a cheatsheet actually exists.

Pulls `pkg.Symbol` tokens out of the fenced code blocks and asks `go doc` about
each one. Anything `go doc` cannot find is printed as UNKNOWN — usually a typo,
a symbol from a non-stdlib package, or an API that was renamed.

    python3 tools/check.py 13-15-concurrency.md [more.md ...]
"""
import re, subprocess, sys, concurrent.futures

# short package name -> import path
PKGS = {
    "sync": "sync", "atomic": "sync/atomic", "context": "context", "runtime": "runtime",
    "time": "time", "signal": "os/signal", "debug": "runtime/debug", "pprof": "runtime/pprof",
    "trace": "runtime/trace", "os": "os", "fmt": "fmt", "strings": "strings", "strconv": "strconv",
    "bytes": "bytes", "slices": "slices", "maps": "maps", "cmp": "cmp", "sort": "sort",
    "errors": "errors", "http": "net/http", "httptest": "net/http/httptest", "json": "encoding/json",
    "sql": "database/sql", "slog": "log/slog", "io": "io", "bufio": "bufio", "net": "net",
    "tls": "crypto/tls", "rand": ["crypto/rand", "math/rand/v2", "math/rand"], "template": "html/template", "url": "net/url",
    "csv": "encoding/csv", "heap": "container/heap", "list": "container/list", "ring": "container/ring",
    "testing": "testing", "reflect": "reflect", "iter": "iter", "unicode": "unicode",
    "filepath": "path/filepath", "exec": "os/exec", "signalctx": "os/signal", "httputil": "net/http/httputil",
    "flag": "flag", "log": "log", "math": "math", "regexp": "regexp", "utf8": "unicode/utf8",
    "hmac": "crypto/hmac", "sha256": "crypto/sha256", "base64": "encoding/base64", "hex": "encoding/hex",
    "subtle": "crypto/subtle", "synctest": "testing/synctest", "quick": "testing/quick", "weak": "weak", "unsafe": "unsafe", "syscall": "syscall",
}
# packages that are real but outside the stdlib -> not checkable with `go doc`
SKIP = {"errgroup", "semaphore", "singleflight", "proto", "protojson", "bufconn", "otel", "asynq", "redis", "bcrypt", "argon2", "jwt", "quic"}

# Fully-qualified names that a short package name resolves to wrongly, because a
# non-stdlib package shares the name. Checked by hand; keep the reason attached.
IGNORE = {
    "runtime/trace.SpanFromContext",  # OpenTelemetry's trace package, not the runtime's
}

SYM = re.compile(r'\b([a-z][a-z0-9]{1,10})\.([A-Z][A-Za-z0-9]*)\b')


def symbols(path):
    text = open(path).read()
    found = set()
    for block in re.findall(r'```[a-z]*\n(.*?)```', text, re.S):
        for pkg, sym in SYM.findall(block):
            if pkg in SKIP or pkg not in PKGS:
                continue
            paths = PKGS[pkg]
            found.add((tuple(paths) if isinstance(paths, list) else (paths,), sym))
    return sorted(found)


def check(item):
    """A short package name can be ambiguous (rand, template, ...) — any hit counts."""
    paths, sym = item
    for pkg in paths:
        r = subprocess.run(["go", "doc", f"{pkg}.{sym}"], capture_output=True, text=True)
        if r.returncode == 0:
            return item, True
    return item, False


# Everything the sheets legitimately use beyond ASCII.
ALLOWED_NON_ASCII = set("—–·≥≤≠±²³×→←↔’“”é")  # em dash, en dash (ranges), ...


def structure(path):
    """Fence balance and code-block width — code blocks do not wrap in the PDF."""
    problems = []
    lines = open(path).read().split("\n")
    fences = [i for i, l in enumerate(lines) if l.startswith("```")]
    if len(fences) % 2:
        problems.append("unbalanced code fences")
    inside = False
    for i, l in enumerate(lines, 1):
        if l.startswith("```"):
            inside = not inside
            continue
        if inside and len(l) > 88:
            problems.append(f"line {i} is {len(l)} cols wide (max 88)")
        stray = {ch for ch in l if ord(ch) > 127} - ALLOWED_NON_ASCII
        if stray:
            problems.append(f"line {i} has unexpected characters: {''.join(sorted(stray))}")
    return problems


def main(paths):
    bad = 0
    for path in paths:
        for p in structure(path):
            print(f"{path}: {p}")
            bad += 1
        items = symbols(path)
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as ex:
            results = list(ex.map(check, items))
        missing = [f"{p[0]}.{s}" for (p, s), ok in results
                   if not ok and f"{p[0]}.{s}" not in IGNORE]
        bad += len(missing)
        status = "OK" if not missing else "UNKNOWN: " + ", ".join(missing)
        print(f"{path}: {len(items)} stdlib symbols checked -> {status}")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:] or ["13-15-concurrency.md"]))
