# 01 — Introduction to Go

## Goals
- Understand what Go is and what problems it was designed to solve.
- Know Go's core philosophy and how it differs from languages you know.
- Get a mental model of the Go toolchain and ecosystem before writing any code.

## Concepts
- **What is Go?** A statically typed, compiled language from Google (2009), designed for building simple, reliable, efficient software — especially networked servers and CLI tools. Often called "Golang" because of its old domain `golang.org` (now `go.dev`).
- **Why Go exists** — it was a reaction to slow builds, complex dependency graphs, and difficult concurrency in large C++/Java codebases. The design goals: **fast compilation**, **simple syntax**, **built-in concurrency**, and **easy deployment** (a single static binary).
- **Core philosophy:**
  - **Simplicity over features** — the language is deliberately small. There are no classes, no inheritance, no exceptions, no generics until 1.18, and even now generics are used sparingly. "Less is more."
  - **One obvious way to do things** — enforced by tools like `gofmt` (there is no style debate; the formatter decides).
  - **Composition over inheritance** — you build behavior from small pieces (structs + interfaces), not class hierarchies.
  - **Errors are values** — no exceptions; functions return an `error` you check explicitly. This makes failure paths visible.
  - **Concurrency is built in** — goroutines and channels are language features, not libraries.
- **Compiled & statically typed** — `go build` produces a single self-contained binary (no VM, no interpreter, no runtime to install on the server). Types are checked at compile time, which catches a whole class of bugs early.
- **Garbage collected** — you don't manage memory manually like in C, but you also get value types and pointers, so you have more control than in Java/Python.
- **Where Go fits** — backend web services & APIs, microservices, CLI tools, DevOps/cloud-native tooling (Docker, Kubernetes, Terraform, and Prometheus are all written in Go). It's our target: **backend REST APIs**.
- **The ecosystem:**
  - **The toolchain** — `go` is one command that builds, tests, formats, vets, and manages dependencies. No separate build tool like Maven/webpack.
  - **Modules** — `go.mod` declares your module and its dependencies (like `package.json`).
  - **The standard library** — unusually complete: a production HTTP server, JSON, crypto, SQL interface, and testing are all built in.
  - **`pkg.go.dev`** — the package documentation hub (like npmjs.com but docs-first).

## Exercises
1. Read the official "Why Go" framing and tour intro (links below). You don't need to write code yet — focus on the mental model.
2. Open the **Go Playground** (https://go.dev/play/) and run the default "Hello, 世界" program. Change the printed text and run it again to get a feel for the edit-run loop.
3. In the Playground, observe that even a one-line program needs `package main`, an `import`, and a `func main()`. Ask Claude *why* Go requires this structure (vs Python's bare script).
4. Write down (in your own words) three differences between Go and the language you know best. Ask Claude to confirm or correct your understanding.

## Best Practices & Pitfalls
- **Don't fight the language.** If you come from Java/Python/JS, you'll miss features (ternary operator, exceptions, inheritance, `while`). Go leaves them out on purpose. Learn the Go way rather than recreating your old language.
- **Embrace explicitness.** Go favors clear, slightly verbose code over clever one-liners. Returned errors, no hidden control flow — this is a feature.
- **"Gofmt's style is no one's favorite, yet gofmt is everyone's favorite."** Accept the formatter early; never argue about braces or spacing.
- **Pitfall:** expecting OOP. There are no classes. You'll model data with `struct` and behavior with `interface`, composed together — covered in Part 3.

## Checklist
- [ ] I can explain in one sentence what Go is and what it's good at.
- [ ] I can name three pillars of Go's philosophy (simplicity, errors-as-values, built-in concurrency).
- [ ] I understand that `go build` produces a single static binary.
- [ ] I've run a program in the Go Playground.

## Resources
- Go homepage: https://go.dev/
- A Tour of Go (interactive): https://go.dev/tour/
- Effective Go (read later, but bookmark it): https://go.dev/doc/effective_go
- FAQ — language design rationale: https://go.dev/doc/faq
- The Go Playground: https://go.dev/play/
