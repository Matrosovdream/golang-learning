# 45 — Polymorphic Relations & Type Registries

> Part 9, Track A (structuring one service): deepens [22 Database](22-database.md) (persistence) and [31 DDD Tactical](31-ddd-tactical.md) (repositories), and builds on [11 Interfaces](11-interfaces.md).
> Two shapes recur in a real dashboard. One **child** table — comments, evidence, attachments, audit rows — must attach to **many** parent types. And one set of **generic operations** (list, stage, assign, resolve) must run over **many** concrete case types. Both are solved the same way: a `(type, id)` pair and an interface + registry — **without switching on type**.

## Goals
- Model a child that belongs to many parents via a `(type, id)` pair, in one table.
- Understand why a polymorphic column **drops the database foreign key**, and enforce parent existence in code instead.
- Build a **subject checker** as a *consumer-defined* interface ([32](32-hexagonal-ports-adapters.md)) that answers "does this `(type, id)` exist in this tenant?".
- Unify N concrete types behind **one interface + a registry** so generic code never `switch`es on type.
- Scope polymorphic rows correctly for **multi-tenancy** — via the subject check, or a real `tenant_id`, or both.

## Concepts

### The polymorphic relation: a (type, id) pair
A comment attaches to a chargeback *or* a prevention alert *or* an order *or* an RDR case. Instead of one table per pairing, store **which kind** and **which row** in two columns and reuse a single `comments` table:
```sql
CREATE TABLE comments (
    id               BIGSERIAL PRIMARY KEY,
    commentable_type TEXT   NOT NULL,   -- 'chargeback' | 'prevention' | 'order' | 'rdr' | ...
    commentable_id   BIGINT NOT NULL,   -- the parent row's id, within that type
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body             TEXT   NOT NULL
    -- ...
);
-- every read is "all comments for this subject", so index the pair together:
CREATE INDEX idx_comments_commentable ON comments(commentable_type, commentable_id);
```
The Go struct mirrors the pair (`ListFor` reads `WHERE commentable_type = $1 AND commentable_id = $2`). Contrast the two designs:
```go
// ❌ one nullable FK per parent — grows a column per new parent type, exactly one non-null per row.
type Comment struct {
    ChargebackID *int64
    PreventionID *int64
    OrderID      *int64 // ...and a fifth, and a sixth, forever
}

// ✅ one (type, id) pair — the table never changes shape as parents multiply.
type Comment struct {
    CommentableType string
    CommentableID   int64
}
```

### The trade-off: no database foreign key
`commentable_id` points at a *different* table depending on `commentable_type`, so it **cannot** be a real `REFERENCES`. Postgres won't stop you inserting a comment for a chargeback that doesn't exist, and won't cascade-delete comments when the parent is removed. You trade a database guarantee for schema flexibility — and take on the job of enforcing it yourself.

| | Nullable FK per parent | Polymorphic `(type, id)` |
|---|---|---|
| **Referential integrity** | ✅ enforced by the DB | ❌ must enforce in code |
| **Cascade delete** | ✅ automatic | ❌ manual / cleanup job |
| **New parent type** | ALTER TABLE (new column) | zero schema change |
| **Query by parent** | one nullable column | composite `(type, id)` index |
| **Best when** | few, fixed parents; strong integrity | open/growing set of parents |

### Enforcing existence in code: the subject checker
Since the DB won't guarantee the parent exists, the **application** must — before every read or write. Declare the need as an interface *in the domain*, where the comment service consumes it (dependency inversion, [32](32-hexagonal-ports-adapters.md)):
```go
// domain/comment.go — the consumer declares the contract it needs.
type SubjectChecker interface {
    SubjectExists(ctx context.Context, tenantID int64, subjectType string, subjectID int64) (bool, error)
}
```
The concrete implementation lives in the service layer and bridges the case registry and the order repo — it knows *how* to resolve each subject type, but the service only sees the interface:
```go
// service/subject_checker.go — returns the INTERFACE, hides *subjectChecker.
func NewSubjectChecker(registry *CaseRegistry, orders domain.OrderRepository) domain.SubjectChecker {
    return &subjectChecker{registry: registry, orders: orders}
}

func (s *subjectChecker) SubjectExists(ctx context.Context, tenantID int64, subjectType string, subjectID int64) (bool, error) {
    if subjectType == "order" {
        return s.orders.ExistsForTenant(ctx, tenantID, subjectID)
    }
    ct, ok := domain.ParseCaseType(subjectType) // comma-ok: reject unknown types
    if !ok {
        return false, nil
    }
    repo, err := s.registry.RepoFor(ct)
    if err != nil {
        return false, nil
    }
    return repo.ExistsForTenant(ctx, tenantID, subjectID)
}
```
The service calls it *before* touching the child table — validate the type against a whitelist, then confirm the subject exists in the tenant:
```go
func (s *CommentService) Create(ctx context.Context, subjectType string, subjectID int64, body string) (*domain.Comment, error) {
    if !slices.Contains(domain.CommentSubjectTypes(), subjectType) {            // 1. whitelist the type
        return nil, domain.Invalid("type", "is not a commentable subject")
    }
    ok, err := s.subjects.SubjectExists(ctx, tenantID, subjectType, subjectID)  // 2. the missing FK, in code
    if err != nil {
        return nil, err
    }
    if !ok {
        return nil, domain.NotFound(subjectType)
    }
    return s.repo.Create(ctx, &domain.Comment{CommentableType: subjectType, CommentableID: subjectID /* ... */})
}
```

### Tenant scoping for polymorphic rows
A polymorphic row can be scoped **two** ways, and midigator uses both deliberately:
- **Comments carry NO `tenant_id`.** They're scoped *transitively*: `SubjectExists(ctx, tenantID, type, id)` already proved the parent belongs to the tenant, so the comment does too. Fewer columns, no denormalized tenant to keep in sync — but every access **must** route through the subject check.
- **Evidence files DO carry `tenant_id`** (plus their own `(evidenceable_type, evidenceable_id)` pair). Files are sensitive, so defense in depth: the repository filters `WHERE tenant_id = $1` *and* the service runs the subject check. If one layer is bypassed, the other still holds the boundary.

Rule of thumb: derive the tenant from the subject when the child is cheap and always reached through a checked path; store an explicit `tenant_id` when it's sensitive or queried directly (audit exports, storage cleanup) where you can't assume the check ran.

### One interface, many types: the registry
The mirror-image problem: four concrete case repositories (chargeback, prevention, order-validation, RDR) all need generic operations — list, change stage, assign, resolve. Give them **one interface**, and each repo names its **own key**:
```go
// domain/case.go — the cross-cutting contract every case repo satisfies.
type CaseRepository interface {
    CaseType() CaseType                                        // the repo names its own key
    ExistsForTenant(ctx context.Context, tenantID, id int64) (bool, error)
    ChangeStage(ctx context.Context, tenantID, id int64, toStage string, userID int64, notes string) error
    Assign(ctx context.Context, tenantID, id int64, assigneeID *int64) error
    ListSummaries(ctx context.Context, tenantID int64, f CaseFilter) ([]CaseSummary, int, error)
    // ...
}
```
A **registry** is just a `map[CaseType]CaseRepository`. Register by asking the repo for its type; resolve with comma-ok:
```go
// service/registry.go
type CaseRegistry struct {
    repos map[domain.CaseType]domain.CaseRepository
}

func (r *CaseRegistry) Register(repo domain.CaseRepository) {
    r.repos[repo.CaseType()] = repo // key comes from the repo — no caller-supplied type to drift
}

func (r *CaseRegistry) RepoFor(t domain.CaseType) (domain.CaseRepository, error) {
    repo, ok := r.repos[t] // comma-ok: absent key ≠ present nil value
    if !ok {
        return nil, domain.NotFound("case type")
    }
    return repo, nil
}
```
Now generic code stays type-agnostic — it never learns the concrete types exist:
```go
// ❌ a switch every new case type forces you to edit — forget one branch and it's a silent bug.
func changeStage(t domain.CaseType, id int64, to string) error {
    switch t {
    case domain.CaseChargeback:
        return chargebacks.ChangeStage(/* ... */)
    case domain.CasePrevention:
        return preventions.ChangeStage(/* ... */) // ...one branch per type, forever
    }
}

// ✅ resolve once, dispatch through the interface — the map does the branching.
func changeStage(reg *CaseRegistry, t domain.CaseType, id int64, to string) error {
    repo, err := reg.RepoFor(t)
    if err != nil {
        return err
    }
    return repo.ChangeStage(ctx, tenantID, id, to, userID, "")
}
```

### When NOT to use this
Polymorphism isn't free — you gave up FKs and bought a runtime whitelist. It earns its keep only when the set of parent types is **open or growing** *and* **generic operations dominate** (one comment box, one assignment board over all four case types). If you have **two fixed parents** and strong integrity needs, two nullable FK columns (or two separate child tables) are simpler, let the database enforce existence, and cascade-delete for free. Reach for `(type, id)` + a registry when adding the *fifth* parent type shouldn't mean touching the schema or a `switch`.

## Exercises
1. Write the `comments` migration: a polymorphic `(commentable_type, commentable_id)` pair and the composite index on it. Explain in a comment why there's no `REFERENCES` on `commentable_id`.
2. Define `domain.CommentSubjectTypes()` as a whitelist and reject any `subjectType` not in it *before* hitting the database.
3. Implement `SubjectExists(ctx, tenantID, type, id)` that dispatches `"order"` to the order repo and every case type through `registry.RepoFor`. Return `(false, nil)` for unknown types.
4. Build a `CaseRegistry` (`map[CaseType]CaseRepository`) with `Register`/`RepoFor`, and write a generic `Assign(reg, t, id, assignee)` that uses it — with **no** `switch` on type.
5. Add a fifth case type (`retrieval`): implement its repo with `CaseType()` returning the new key, register it, and confirm the generic list/assign/stage code needs **zero** changes.
6. In your notes, argue the FK trade-off for *your* schema: would you keep `(type, id)`, or is a nullable FK / separate table the better call here? Sketch a cleanup job for orphaned polymorphic rows.

## Best Practices & Pitfalls
- **Index the pair together.** Reads are always "rows for this subject" — a composite `(type, id)` index serves them; two single-column indexes don't.
- **Validate the type against a whitelist.** The `type` is free-form text; parse it through `ParseCaseType` / `slices.Contains(CommentSubjectTypes(), t)` and reject anything unknown before it reaches SQL.
- **Check subject existence *and* tenant before writing.** The missing FK lives in the service now: `SubjectExists(ctx, tenantID, type, id)` gates every read and write.
- **Let the repo name its own registry key** (`repos[repo.CaseType()] = repo`). A caller-supplied key drifts out of sync; a self-describing one can't.
- **Pitfall — no FK means orphaned rows.** Delete a parent and its comments/evidence linger. Enforce cascade in code (delete children in the same transaction) and consider a periodic cleanup job; don't assume the DB did it.
- **Pitfall — trusting the `{type}` path segment unchecked.** `POST /orders/comments` and a hand-crafted `POST /wat/comments` both hit the same handler; an unvalidated `type` becomes a stored, unresolvable subject. Whitelist it.
- **Pitfall — reaching for polymorphism when two FK columns would do.** With a small fixed set of parents and real integrity needs, `(type, id)` + a registry is over-engineering — use FKs and let the database do its job.

## Checklist
- [ ] I can model a child that attaches to many parents with a `(type, id)` pair and one table.
- [ ] I can explain why a polymorphic column drops the FK and what I lose (integrity, cascade).
- [ ] I enforce parent existence + tenant in code via a consumer-defined `SubjectChecker` before any read/write.
- [ ] I can choose between transitive tenant scoping (comments) and an explicit `tenant_id` (evidence) and justify it.
- [ ] I can unify N concrete repos behind one interface + a `map[Type]Repo` registry with comma-ok resolution.
- [ ] My generic operations never `switch` on the concrete type — they resolve through the registry.
- [ ] I can name a case where nullable FKs / separate tables beat polymorphism, and say why.

## Resources
- Rails polymorphic associations — the concept's lineage & vocabulary (`*_type` / `*_id`): https://guides.rubyonrails.org/association_basics.html#polymorphic-associations
- Ties back to: [11 — Interfaces](11-interfaces.md) (the registry's whole trick), [22 — Database](22-database.md) (composite indexes, no-FK trade-off), [31 — DDD Tactical](31-ddd-tactical.md) (repositories & contracts).
- Next (Track A): [32 — Hexagonal Ports & Adapters](32-hexagonal-ports-adapters.md) — the dependency inversion the `SubjectChecker` and `CaseRepository` interfaces rest on.
