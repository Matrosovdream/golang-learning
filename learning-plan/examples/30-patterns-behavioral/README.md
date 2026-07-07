# Step 30 — Behavioral Patterns (the Go way) · Examples

A library of **16 runnable examples**, split into three files by difficulty. Every example is a
complete `package main` program you **retype** and run with `go run .`. They reinforce
[30-patterns-behavioral.md](../../30-patterns-behavioral.md): strategy & command as function values,
the Template-Method embedding trap, observer, state machines, range-over-func iterators, chain of
responsibility, and visitor.

## One-time setup

```bash
mkdir -p /tmp/behavioral-ex && cd /tmp/behavioral-ex
go mod init scratch
```

For each example, put the code in **`main.go`** (replacing the previous one) and run it:

```bash
go run .
```

Every example was compiled, `go vet`-ed, and run before being added; the **Output** shown under each
one is real stdout. Standard-library only — no `go get`. Examples 12, 13, and 16 use **range-over-func
iterators**, which need **Go 1.23+**.

| Tier | File | Examples |
|------|------|----------|
| 🟢 Easy | [1-easy.md](1-easy.md) | 1–6 |
| 🟡 Medium | [2-medium.md](2-medium.md) | 7–11 |
| 🔴 Hard | [3-hard.md](3-hard.md) | 12–16 |

> Progress tracker: [PROGRESS.md](PROGRESS.md). Want more examples? Ask and I'll append them.

## Index

### 🟢 [Easy](1-easy.md) — strategy, command, observer, state
- [1. Strategy as a function value](1-easy.md#1-strategy-as-a-function-value)
- [2. Strategy via slices.SortFunc](1-easy.md#2-strategy-via-slicessortfunc)
- [3. Command as a closure queue](1-easy.md#3-command-as-a-closure-queue)
- [4. Observer: a callback slice](1-easy.md#4-observer-a-callback-slice)
- [5. State machine: a transition table](1-easy.md#5-state-machine-a-transition-table)
- [6. Chain of responsibility](1-easy.md#6-chain-of-responsibility)

### 🟡 [Medium](2-medium.md) — interface strategy, undo, template method, visitor
- [7. Strategy as an interface](2-medium.md#7-strategy-as-an-interface)
- [8. Command with undo + history](2-medium.md#8-command-with-undo--history)
- [9. The Template-Method embedding trap](2-medium.md#9-the-template-method-embedding-trap)
- [10. Template Method, fixed by injection](2-medium.md#10-template-method-fixed-by-injection)
- [11. Visitor via a type switch](2-medium.md#11-visitor-via-a-type-switch)

### 🔴 [Hard](3-hard.md) — iterators, pub/sub, state objects, capstone
- [12. Range-over-func iterator (iter.Seq)](3-hard.md#12-range-over-func-iterator-iterseq)
- [13. Key/value iterator (iter.Seq2)](3-hard.md#13-keyvalue-iterator-iterseq2)
- [14. Observer via channels (pub/sub)](3-hard.md#14-observer-via-channels-pubsub)
- [15. State as objects](3-hard.md#15-state-as-objects)
- [16. Capstone: undoable stack calculator](3-hard.md#16-capstone-undoable-stack-calculator)
