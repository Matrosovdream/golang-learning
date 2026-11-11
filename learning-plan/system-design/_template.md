# NN · Task Title

`🟢 easy` · *Topics:* comma, separated, concepts · *Builds on:* [NN Other Task](NN-other.md)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [NN Previous](NN-prev.md) · Next: [NN Next](NN-next.md)

<!-- Copy this file into easy/, medium/ or hard/. Paths above assume one folder below the track root.
     Lesson links from a task file look like ../../22-database.md. -->

<!-- Section map (keep all 14, in this order):
     1 task · 2 questions · 3 requirements · 4 estimation · 5 plan · 6 high-level design · 7 API
     8 data model · 9 deep dives · 10 scaling & failures · 11 trade-offs · 12 check yourself · 13 in Go · 14 extensions
     15 hardware sizing on AWS (see the README's AWS sizing cheat sheet) · 16 database choice and why (see the README's database choice cheat sheet) -->



---

## 1. The task

> "One or two sentences, deliberately vague, the way an interviewer says it."

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| | | |

## 3. Requirements

**Functional**
- 

**Non-functional**
- 

**Out of scope**
- 

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| | | |

What the numbers tell us: 

## 5. Plan

1. 
2. 
3. 

## 6. High-level design

```
ASCII diagram
```

Walk through one write and one read.

## 7. API

```
METHOD /path   → status, body
```

## 8. Data model

```sql
```

## 9. Deep dives

### 9.1 Decision name

The problem. The options:

| Option | Pros | Cons |
|---|---|---|
| | | |

**Choice and why.**

### 9.2 ...

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×:** 
- **Single points of failure:** 
- **When X is down:** 

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| | | | |

## 12. Check yourself

1. 

<details><summary>Answers</summary>

1. 

</details>

## 13. In Go

- Related lessons: 
- Packages: 

## 14. Extensions

- 

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads come from section 4; every CPU cost per request is an assumption, say so)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| | | | | | | |

**Rough monthly bill (≈, on-demand)**

| Line | ≈ $/month |
|---|---|
| | |

- **Biggest line and why:**
- **What changes at 10×:**

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:** the access patterns (point reads by key, range scans, joins, transactions, aggregates, full-text), the consistency it needs, the size and the write rate, from sections 4 and 8.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | | |
| MySQL (RDS / Aurora) | | |
| DynamoDB / Cassandra | | |
| Redis (ElastiCache / MemoryDB) | | |
| (others that are genuinely relevant) | | |

**Pick:** the store, the AWS service, and the two or three properties or features that decide it, tied to sections 8–9.

**If it were MySQL instead:** what would change (a Postgres-only feature replaced by a workaround), or "nothing material".

**What would flip the choice:** the number or requirement at which you would move to something else.
