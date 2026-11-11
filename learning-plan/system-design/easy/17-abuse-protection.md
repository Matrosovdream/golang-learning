# 17 · Abuse & Spam Protection for Public Endpoints

`🟢 easy` · *Topics:* who counts as "a person" (address, account, contact value), the real client address behind a CDN and why a header is only trusted from a trusted peer, layered defence (traps → limits → score → human verdict), fake success for bots, counting only accepted submissions, fail open, spam as a flag beside the pipeline, runtime-tunable limits · *Builds on:* [03 Rate Limiter](03-rate-limiter.md) (the algorithms; this task is about *keys and layers*), [16 Public Form Intake](16-public-form-intake.md) (the handler these layers sit in)

> ← [Index](../README.md) · [Progress](../PROGRESS.md) · Prev: [16 Public Form Intake](16-public-form-intake.md) · Next: [18 Inbound Webhook Receiver](18-inbound-webhook-receiver.md)

---

> 🌱 **Seed (honeydolist-site):** PLAN Step 24 (built 2026-10-08): limits per *person*, not per address (signed-in staff keyed by account, 600 a minute, never lead-limited; visitors by address, 120 a minute, 5 leads an hour and 20 a day); the four numbers are keys in the `settings` table (`limits.*`), read from memory every 5 s, changed by the Superadmin alone (`system.write`, migration 0050) with an audit row in words; behind Cloudflare the visitor's address is `CF-Connecting-IP`, trusted only when `X-Real-IP` is one of Cloudflare's published ranges (production visitors had been sharing one limit). `intake.Screen` drops a form whose hidden `hp` field is filled or that was completed in under 2,000 ms with a fake `201`; only *accepted* leads count against the limit; a limiter that cannot answer lets the lead through. Step 64 / migration 0082 added `requests.is_spam`, a verdict *beside* the status pipeline with a partial index for the Spam chip. Real volume: tens of leads a day.
> **Here, at scale:** the public write endpoints of a site with 100,000 requests/s at the edge, a botnet of a million addresses, and one office of 300 honest people behind a single IP.

## 1. The task

> "Our contact, sign-up and quote endpoints are being hammered by bots and by humans pasting
> links. Stop the abuse without turning away real customers, and without losing the real lead
> that happens to look a bit like spam."

Every defence has a false-positive rate, and a refused customer is revenue nobody ever sees. The
design question is *where each mistake lands* and whether it can be undone.

## 2. Clarifying questions

| Ask | Why it matters | Assumed answer |
|---|---|---|
| What are we protecting? | Cost shapes the defence. | Staff attention first, then database/email/SMS cost. |
| Which mistake is worse? | Decides fail open vs closed. | Losing a real lead is worse than one extra spam email. |
| Who is never limited? | Exemptions need an identity. | Signed-in staff (testing, taking orders by phone). |
| How do users reach us? | The client address is hidden by proxies. | Through a CDN and our own reverse proxy. |
| Who may change the limits, and how fast? | A tuning knob is also an attack surface. | Superadmin only, from a settings page, live in seconds, audited. |
| Scale and attackers? | Decides edge vs app. | 100k req/s edge, about 1% writes; a botnet of 1M addresses. |
| Shared addresses? | Per-address limits punish honest groups. | Offices, schools, mobile carriers (CGNAT) are normal. |
| What does the user see when refused? | A wrong message loses the customer. | Say which limit and what to do ("please call us" vs "wait a moment"). |

## 3. Requirements

**Functional**
- Refuse floods per person; silently discard obvious machine-made submissions.
- Flag likely spam without deleting it; a human can mark and unmark, and unmarking restores exactly.
- Limits editable at runtime by authorised staff; every change audited.
- Staff never locked out by their own testing.

**Non-functional**
- The cheapest layer runs first; a bot costs us microseconds, not a database write.
- If the limiter or scorer is down, real leads still get through.
- A limit change takes effect on every node within seconds.

**Out of scope**
- Volumetric DDoS absorption (CDN/WAF), account takeover and login brute force, content moderation ([19](19-moderated-submissions.md)).

## 4. Back-of-the-envelope

| Quantity | Estimate | How |
|---|---|---|
| Edge traffic | 8.64B requests/day | 100,000 × 86,400 |
| Write endpoints | ≈ 1,000/s | about 1% of the edge; the rest is cacheable reads |
| A quiet botnet | 1,000,000 IPs × 1 request/min ≈ 16,700/s | 1,000,000 ÷ 60 = 16,667/s: 17% of the whole edge, yet each address is far under 120/min |
| A loud botnet | 1M IPs × 2/s = 2M/s | the maximum under a 120/min limit; no origin absorbs it: that is the CDN's job |
| Limiter state | 10M active keys × ≈ 100 B ≈ 1 GB | a counter and a TTL per key |
| Limiter ops | writes only: 1,000/s × 2 policies × (peek + record) = 4,000 ops/s; on all traffic: ≈ 200,000 ops/s | ≈ 2 Redis cores' worth, so apply the shared limiter to write endpoints only |
| Settings reads | 200 nodes ÷ 5 s = 40 reads/s | the cost of "live in 5 seconds" |
| Cost per request | trap ≈ 50 µs · limiter ≈ 0.5 ms · score ≈ 1–5 ms · human ≈ 30 s | assumptions; the ratio is what matters |

A day of 1,000,000 submissions, assuming 40% are bots (400,000) and 600,000 are people:

| Stage | Count | Note |
|---|---|---|
| Dropped by the traps | 340,000 | 85% of the bots; fake `201`, no database write |
| Stored | 660,000 | 600,000 people + 60,000 bots that passed |
| Flagged by the scorer | 48,600 | 80% of the 60,000 bots = 48,000, plus 0.1% of people = 600 false positives |
| Left in the inbox | 611,400 | 599,400 people + 12,000 bots: about 2% of the inbox is still spam |
| Flagged rows kept | 73 MB/day · 2.2 GB for 30 days | 48,600 × 1.5 KB |

What the numbers tell us: per-address limits cannot stop a distributed low-rate botnet, so traps, scoring and the CDN do the heavy lifting. The funnel also says **600 real people were flagged**, which is why spam is a flag with an undo and not a deletion.

## 5. Plan

1. Decide what a "person" is, and how to learn the real client address.
2. Order the layers by cost: CDN, traps, limits, score, human.
3. Decide what each layer answers and what it counts.
4. Model spam as a verdict beside the pipeline.
5. Make the limits tunable and safe to tune.

## 6. High-level design

```
 request ─► CDN/WAF  volumetric floods, known-bad networks, optional challenge; adds CF-Connecting-IP
              ▼
           our reverse proxy   sets X-Real-IP = the TCP peer, overwriting anything the visitor sent
              ▼
           API   ClientIP() ──► subject keys: user:<id> | ip:<hmac(ip, /64 for IPv6)> | contact:<hmac(email|phone)>
              1 traps:  hp filled, or elapsed < 2,000 ms ──► fake 201 (counter bot_trapped++)
              2 limits (peek): visitor/min · user/min · leads/hour · leads/day ──► 429 + Retry-After + which limit
              3 validate, write ──► 4 limits (record accepted)         limiter down → allow (fail open)
              5 score (ms): links, disposable domains, repeats, velocity ──► is_spam = true | false
           stored row ──► inbox (NOT is_spam)   ·   Spam chip (is_spam)   ·   human: Mark as spam / Not spam
```

Walk through one request: a bot posts with the honeypot filled, gets a normal-looking `201`, and nothing is stored. A person fixing a typo three times is refused nothing, because only *stored* leads are recorded against the limit.

## 7. API

```
POST /v1/forms/{form}/submissions    (see 16; traps: "hp", "form_elapsed_ms")
       → 201 {id}      real or fake, indistinguishable
       → 400           trap fields missing: a broken client, not a bot
       → 429 Retry-After  { detail: "Please call us instead." | "Please wait a moment." }
PUT  /admin/submissions/{id}/spam   { "spam": true, "remove_contact": true, "withdraw_consent": true } → 200
GET  /admin/submissions?spam=true   → the Spam chip's list
GET  /admin/settings/limits         → { visitor_per_minute: 120, user_per_minute: 600, leads_per_hour: 5, leads_per_day: 20, version: 7 }
PUT  /admin/settings/limits         If-Match: 7 → 200 | 409 stale | 422 outside the allowed bounds     (permission system.write)
```

## 8. Data model

```sql
ALTER TABLE submissions
    ADD COLUMN is_spam    boolean NOT NULL DEFAULT false,  -- constant default: no table rewrite
    ADD COLUMN spam_score smallint,                        -- 0-100 from the scorer; NULL = not scored
    ADD COLUMN ip_hash    text;                            -- HMAC(secret, ip), 32 hex chars; never the raw address

-- the Spam chip: only the few flagged rows
CREATE INDEX submissions_spam_idx  ON submissions (created_at DESC, id DESC) WHERE is_spam AND deleted_at IS NULL;
-- the inbox, if flagged rows ever become a large share of the table
CREATE INDEX submissions_inbox_idx ON submissions (form_id, created_at DESC, id DESC) WHERE NOT is_spam AND deleted_at IS NULL;

CREATE TABLE settings (
    key        text PRIMARY KEY,                           -- 'limits.visitor_per_minute', …
    value      jsonb NOT NULL,
    version    integer NOT NULL DEFAULT 1,
    updated_by bigint, updated_at timestamptz NOT NULL DEFAULT now()
);
-- each change that alters a value also writes an audit row in words: "Visitors 120 → 200 a minute" (task 22)
```

Counters live in Redis, not Postgres: `lim:{policy}:{subject}` holds the bucket or window state with a TTL equal to the window ([03](03-rate-limiter.md)). A missing key is a fresh start.

## 9. Deep dives

### 9.1 Who is "a person"? Keys, and the real client address

| Key | Pros | Cons |
|---|---|---|
| IP address | free, known before login | an office of 300 or a mobile carrier shares one; botnets rotate; an IPv6 device owns a whole /64, so key on the /64 |
| Account | precise; allows a staff exemption | anonymous leads have none; sign-up bots mint accounts |
| Contact value (normalised email or phone, hashed) | follows the *person* across addresses: one phone number can't be pumped through 1,000 IPs | cheap to vary; store an HMAC, not the value |
| Device cookie / fingerprint | survives address changes | cleared or forged trivially; fingerprinting has a privacy cost |
| Behaviour (timing, challenge result) | hard for dumb bots | accessibility and false positives; an arms race |

**Choice and why.** Layered keys, strictest where the stakes are highest. Signed in: key `user:<id>`, a roomier limit (600 a minute against 120), staff never lead-limited and not counted. Anonymous: the address (IPv4 /32, IPv6 /64) for the per-minute limit, and the **contact value for the lead counts**, so an honest colleague in a 300-person office is not refused because five others sent leads. Fingerprints and behaviour are *scoring inputs*, never hard limits.

**The trusted-peer rule.** A header is only as honest as whoever set it. Honeydolist's proxy sets `X-Real-IP` from the real TCP connection and overwrites whatever the visitor sent, and the app port is published nowhere. Behind a CDN the connection *is* the CDN, so every visitor looks like one address: in production that meant one leads limit shared by all visitors (found in Step 24). The fix: take `CF-Connecting-IP` **only if the peer is in the CDN's published ranges**; from anyone else it is a header the visitor typed.

```go
func ClientIP(r *http.Request) string {
	peer := r.Header.Get("X-Real-IP") // set by our proxy from the TCP peer; visitor-sent copies are overwritten
	if cf, err := netip.ParseAddr(r.Header.Get("CF-Connecting-IP")); err == nil && inCDNRanges(peer) {
		return cf.String() // only a CDN edge may tell us who is behind it
	}
	if peer != "" {
		return peer
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
```

For `X-Forwarded-For`, walk from the **right** and take the first hop that is not one of your own proxies; the leftmost entry is whatever the client wrote. Keep the CDN range list fresh (a daily job against the published list). A missing range fails in the *safe* direction: its visitors are counted by the proxy's address, stricter, not looser. Count `CF-Connecting-IP` arriving from a non-CDN peer: that is someone trying to spoof. Store the address only as `HMAC(secret, "ip:" + addr)` truncated: keyed, so it cannot be reversed by hashing all 4.3 billion IPv4 addresses.

### 9.2 Layered defence: cheap first, undoable last

| Answer to a tripped trap | Pros | Cons |
|---|---|---|
| `4xx` with a reason | honest and simple | tells the bot which field caught it, so it adapts |
| **Silent fake `201`** | the bot learns nothing and does not retry; costs one handler pass | a person wrongly trapped loses the lead silently, so the traps must be near-zero false-positive |
| Tarpit (answer slowly) | wastes the bot's time | holds our connections too |
| CAPTCHA for everyone | strong | conversion drops; accessibility; third party |

**Choice and why.** Order by cost. **Traps** are free: a hidden `hp` field a person never fills, and "completed in under 2 s" (people cannot). A *missing* trap field is a `400` ("a client bug, not a bot", because treating it as a bot would silently drop every lead behind a `201`). **Limits** next, one Redis round trip. **Scoring** (links per message, disposable-email domains, repeated text, velocity per contact) costs milliseconds. **Humans** cost minutes, so they see only what survives.

Rules that make the limits safe:
- **Peek before, record after.** Check the budget before storing, and record against it only once the lead is *accepted*; a customer correcting a typo three times never locks themselves out. A separate cheap per-minute limit covers all requests, so 422-probing is still bounded.
- **Fail open.** If the limiter errors, let the lead through and log it once: "losing a customer costs more than one extra email".
- **Staff are exempt** from lead limits and aren't counted against their address. Step 24 started because the owner's own tests tripped the sixth-lead limit, and staff behind one office address shared a single per-minute budget with the dashboard's polling.
- **Say which limit.** `429` carries `Retry-After` and a message per policy.

### 9.3 Spam is a flag, not a state

| Option | Pros | Cons |
|---|---|---|
| Delete on detection | no storage | irreversible; a false positive is a lost customer; no labelled data |
| `status = 'spam'` in the pipeline | simple | undo must remember the old status; every transition, filter and report must understand it |
| **Boolean beside the status** | status untouched, so *Not spam* restores exactly; one `NOT is_spam` filter | every list, count and report must remember the filter (forget once and spam is in the numbers) |
| Separate quarantine table | inbox queries untouched | moving rows both ways; joins |

**Choice and why.** Step 64's rule: spam is a verdict beside the pipeline. A flagged request keeps its status, so un-marking puts it back where it was. It leaves the New list, the sidebar and overview counts, pipeline value and lead metrics, and appears under a Spam chip served by the partial index. Marking and unmarking are history lines, live-updated for anyone who has the record open, and gated by permission.

**At scale.** 48,600 flagged of 660,000 stored is 7.4%: a `NOT is_spam` filter on the existing indexes stays cheap while the rows it turns away are few (honeydolist reasoned the same way and left its inbox indexes unchanged). If spam grew past about 60% of the table, the inbox would scan 2.5 rows per useful row, so switch to the partial `WHERE NOT is_spam` indexes in §8. Purge flagged rows after 30 days (≈ 2.2 GB steady state).

**What a flag cannot undo.** Arrival already had side effects: staff alerts sent, a contact created, a consent recorded. The *Mark as spam* sheet offers to also remove the contact that this request created (only if nothing else is attached) and withdraw the consent it recorded. Honeydolist notes the gap: the alert has gone before anyone judges. Fix by scoring synchronously (milliseconds) before writing the outbox rows, and enqueue the alert as *held* for borderline scores. Every mark and unmark is a label: the later *block this sender* feature and a classifier both read it.

### 9.4 Runtime-tunable limits

| How nodes learn a change | Pros | Cons |
|---|---|---|
| Env var + deploy | simplest | minutes; an emergency needs a deploy |
| **Poll the database every N s (N = 5)** | trivial; no new infrastructure; works if push fails | N seconds stale; the database must be reachable (keep the last good value) |
| Push (`LISTEN/NOTIFY`, Redis pub/sub, NATS) | instant | a missed message still needs the poll |
| A flag service ([09](09-feature-flags.md)) | targeting, gradual rollout | another dependency for four numbers |

**Choice and why.** Poll, with the saving node refreshing at once: 200 nodes at 5 s is 40 reads/s. Keep the last known good value on a failed read, and jitter the interval so nodes don't synchronise:

```go
var current atomic.Pointer[Limits]

func refresh(ctx context.Context, load func(context.Context) (Limits, error)) {
	for {
		if l, err := load(ctx); err == nil {
			current.Store(&l) // on error keep serving the last known good limits
		}
		wait := 5*time.Second + time.Duration(rand.Int64N(int64(time.Second)))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
```

Make tuning safe: bounds on save (a typo of `1` a minute locks out everyone; `0` must not silently mean "unlimited"), a stale-version `409` (two admins), an audit row in words, and a lock when an environment override is set (the page shows it and disables those fields). Restrict who may change it: how a server protects itself is not the business's to change (`system.write`, Superadmin only). Expose `limit_refused{policy}` so the owner sees *which* limit bites before turning a knob.

## 10. Scaling, bottlenecks & failure modes

- **What breaks first at 10×** (1M req/s edge): the shared limiter (per-person checks on write endpoints only; a per-pod coarse limiter for everything else), then scoring latency (make it asynchronous for the clearly-clean majority and synchronous only above a threshold).
- **Single points of failure:** the limiter store (fail open) and the scorer (skip it and record `spam_score = NULL`; never block on it). The CDN's address list is a silent dependency: alert on stale ranges.
- **A shared-NAT office** hits the address limit: contact-value keys protect the individual; a support-set allowlist handles the rest; a challenge is better than a refusal.
- **A trap starts catching people** (e.g. a form change breaks the timing field): watch `bot_trapped / total`, a sudden jump is a bug until proven otherwise.
- **An attacker who learns the honeypot:** traps are a speed bump. The scorer, per-contact limits and a step-up challenge for high scores do the work.
- **Memory of the limiter:** TTLs bound it; a flood of unique keys is itself an attack (cap keys per prefix).

## 11. Trade-offs at a glance

| We chose | Instead of | We gain | We give up |
|---|---|---|---|
| Fake `201` for traps | `4xx` | bots learn nothing | a wrongly trapped person is invisible, so traps must be conservative |
| Contact value + account + address | address only | NAT-safe, person-centred | more keys; hashing; more state |
| Peek before, record after | count every attempt | typo-fixing never locks anyone out | invalid-request probing needs its own cheap limit |
| Fail open | fail closed | no lost customers when the limiter dies | a window of unprotected traffic |
| `is_spam` flag | delete or new status | exact undo, labelled data | every query must filter |
| Poll settings every 5 s | deploy or push | live changes with no new infrastructure | up to ≈ 6 s staleness |

## 12. Check yourself

1. Why does a CDN break a per-address limit, and why isn't it safe to just trust `CF-Connecting-IP`?
2. A bot fills the honeypot. Why return a normal `201` instead of an error?
3. Why record against the limit only after the lead is stored?
4. Why is spam a boolean beside the status instead of a status?
5. A 300-person office shares one address and the leads limit is 5 an hour. What do you change, and what do you refuse to change?

<details><summary>Answers</summary>

1. The TCP peer is the CDN's proxy, so every visitor appears to come from a few CDN addresses and shares one limit. `CF-Connecting-IP` carries the real address, but any client can send that header. Trust it only when the immediate peer is within the CDN's published ranges; otherwise ignore it.
2. An error tells the bot which field caught it and invites it to adapt. A normal `201` with a fake id costs the bot nothing to believe, stores nothing, and avoids retry traffic. The price is that a wrongly trapped person is invisible, so traps must be near-zero false-positive, and a missing trap field is a `400`.
3. So that a person fixing validation errors three times is never refused for it. Failed attempts don't use up the budget; a separate cheap per-minute limit bounds probing.
4. With a flag, the request keeps its status, so *Not spam* restores it exactly; with a status, undo needs the old status remembered and every transition and report must know the new one. Cost: every list and count must filter `NOT is_spam`.
5. Key the lead count on the contact value (and on the account when signed in), so one colleague's leads don't block another; add a support-set allowlist for the office. Do not raise the per-address limit for everyone: that weakens the defence for every botnet to fix one office.

</details>

## 13. In Go

- Related lessons: [68 Rate limiting & throttling](../../68-rate-limiting-throttling.md) (token bucket, `Retry-After`, per-key limiters), [57 Web security](../../57-web-security.md) (trusted proxies, headers), [43 Authorization & RBAC](../../43-authorization-rbac-multitenancy.md) (`system.write` as a permission, not a role), [38 Caching](../../38-caching-patterns.md) (in-memory TTL reads), [36 Resilience](../../36-resilience-patterns.md) (fail open vs closed), [59 Production DB patterns](../../59-production-database-patterns.md) (partial indexes, constant defaults).
- Use `net/netip` for addresses and prefixes (`netip.Prefix.Contains`, `Addr.Prefix(64)` for IPv6); `crypto/hmac` for the keyed hash; `sync/atomic` (`atomic.Pointer`) for lock-free reads of the live limits.
- Packages: `go-redis` (limiter state), `golang.org/x/time/rate` (per-pod coarse limiter), `pgx`.

## 14. Extensions

- **Adaptive challenge:** proof of work or Turnstile only above a score threshold.
- **Block a sender** (email, phone, `ip_hash`) so the next one is spam on arrival; how long do blocks live?
- **Classifier trained on mark/unmark labels,** with the false-positive review queue sorted by least certain first.
- **Shadow mode:** log "would have refused" for 24 h before enforcing a new limit.
- **SMS/OTP pumping:** endpoints that trigger paid messages need hard per-destination caps and country allowlists.
- **Global limits across regions:** approximate counting and token leasing (hard track: global rate limiter).

## 15. Hardware sizing on AWS

> Prices ≈ on-demand, us-east-1, rounded; method and rules of thumb: [AWS sizing cheat sheet](../README.md#aws-sizing-cheat-sheet).

**Sizing, component by component** (loads from section 4; every CPU-per-request figure is an assumption)

| Component | Peak load | Bound by | Math | Pick (managed alternative) | Count | Why |
|---|---|---|---|---|---|---|
| API write tier (Go: traps, limiter client, scorer, form handler) | ≈ 1,000 write requests/s; with 40 % bots 340/s die in the traps and 660/s go on | CPU (scoring is ≈ 58 % of it) | trap 50 µs (section 4) + ~50 µs HTTP and `ClientIP` = 0.1 ms: 1,000 × 0.1 = 0.1 vCPU; limiter peek + record 660 × 1 ms = 0.66; score 660 × 3 ms = 1.98; form handler 660 × 1 ms = 0.66; total 3.4 vCPU. N = ceil(3.4 ÷ (0.8 × 4 × ⅔ = 2.13)) = ceil(1.6) = 2 → 3 | `c7g.xlarge` (4 vCPU / 8 GiB) behind the reverse proxy | 3, one per zone | Link, domain and repeat checks plus one Redis round trip are CPU, so `c`. 28 % busy, 43 % with a zone down. The quiet botnet is the real load: 16.7k/s more, of which 14.2k/s die in the trap (1.4 vCPU) and 2.5k/s pass and cost ≈ 5 ms each (12.8 vCPU) ⇒ 17.6 vCPU ⇒ N = 9 while it lasts, scaled on the trap-rate alarm |
| Redis (limiter counters) | 4,000 ops/s (1,000/s × 2 policies × peek + record); 10M active keys × ≈ 100 B ≈ 1 GB | memory, but small | 1 GB × ~1.5 overhead = 1.5 GB; `maxmemory` ≤ 70 % ⇒ ≥ 2.2 GB of RAM; 4k ops/s (9k if the botnet's passers are counted) against ~100k per core | `cache.m7g.large` (2 vCPU / 8 GiB), ElastiCache | 1 primary + 1 replica in another zone | The smallest non-burstable 2-vCPU shape; `m`, not `r` (no need for 16 GiB). The replica is for fast failover: the limiter fails open and a lost key is a fresh start. Keep it on write endpoints only: on all 100k req/s it would be ≈ 200k ops/s, two cores' worth (section 4) |
| Postgres (`is_spam`, `spam_score`, `ip_hash`, `settings`) | 660k stored rows/day ≈ 7.6/s; settings reads 40/s; flagged rows 73 MB/day, 2.2 GB over 30 days | nothing here | extra columns ≈ 40 B a row ≈ 26 MB/day, two partial indexes, 40 reads/s: noise next to task 16's database | the form database of [16](16-public-form-intake.md) (`db.m7g.xlarge`, Multi-AZ); no new instance | 0 | The spam verdict is a column beside the pipeline, not a system; the 30-day purge keeps it at ≈ 2.2 GB |

Not sized here: the CDN/WAF, because section 3 puts volumetric defence out of scope. 100k req/s is 8.64 B requests a day, ≈ 260 B a month; at any per-request fee that is far larger than every line below, and it is the platform's edge, not this design's cost. The 200 nodes in section 4's settings math are the whole site's fleet; the write tier above needs 3.

**Rough monthly bill (≈, on-demand; without the CDN/WAF and without task 16's database)**

| Line | ≈ $/month |
|---|---|
| API write tier: 3 × `c7g.xlarge` (3 × $105) | $320 |
| Limiter Redis: 2 × `cache.m7g.large` (2 × $120) | $240 |
| ALB: $16 + ≈ 7 LCU-hours average (≈ 2 MB/s ≈ 7 GB/h) × $0.008 × 730 | $60 |
| Egress: 1,000/s × ~0.3 KB replies ≈ 0.8 TB a month × $0.09 | $70 |
| **Total** | **≈ $690** (≈ $490 with 1-year commitments on the compute lines) |

- **Biggest line and why:** the three API nodes (≈ 46 %), and they are mostly idle: 28 % busy at 1,000 writes/s, three being the floor for three zones; the 9-node botnet case is paid only while the attack lasts. The honest answer is tiny hardware. The real costs sit outside this bill: the CDN/WAF's per-request fees, and people, since 48,600 flagged rows a day need a human verdict and 600 of them are real customers.
- **What changes at 10×:** 1M req/s at the edge ⇒ 10k write requests/s ⇒ 34 vCPU if scored in-process ⇒ N = ceil(34 ÷ 4.27) = 8 → 9 × `c7g.2xlarge`; scoring is ≈ 20 of those 34 vCPU, so it moves to an async scorer pool behind a queue (section 10) and the clean majority never waits for it. Redis: 40k ops/s is still under one core, but keys grow to ≈ 100M ⇒ ≈ 15 GB ⇒ ≈ 21 GiB of RAM ⇒ `cache.r7g.xlarge` (32 GiB), the first time memory picks the family. The shared limiter on all traffic would be 2M ops/s, which is why reads keep a per-pod coarse limiter.

## 16. Database choice and why

> Method, comparison tables and rules: [Database choice cheat sheet](../README.md#database-choice-cheat-sheet).

**What this task's data looks like:**
- The hot path has no database. Limiter state is a counter with a TTL per key: 10M active keys ≈ 1 GB and ~4,000 ops/s on the write endpoints (section 4). It fails open, and a missing key is a fresh start (section 8).
- The database holds only the verdict: three columns on `submissions` (`is_spam`, `spam_score`, `ip_hash`, ≈ 40 B a row) for 660k stored rows a day (≈ 7.6/s), of which ≈ 48,600 are flagged (7.4 %, 73 MB a day), plus a `settings` table of a few rows read ~40 times a second.
- Queries stay on the lead's own table: the inbox (`NOT is_spam`), the Spam chip (`is_spam`), and mark/unmark that restores the old status exactly. Settings need an optimistic `version` check.

| Candidate | Verdict | Why / why not here |
|---|---|---|
| PostgreSQL (RDS / Aurora) | ✅ chosen for verdicts and settings, ✖ for the counters | the lead database of task 16, no new instance; `ADD COLUMN is_spam boolean NOT NULL DEFAULT false` is a metadata-only change; the partial indexes `… WHERE is_spam AND deleted_at IS NULL` keep the Spam-chip index to the ≈ 7 % flagged rows; `jsonb` setting values with a `version` column. As the limiter store it would mean ~4,000 counter updates a second and a TTL-delete job on the database that holds the leads |
| MySQL (RDS / Aurora) | ✔ works | `INSTANT` add-column does the same job as the constant default; no partial indexes, so the chip uses `(is_spam, created_at DESC, id DESC)` and the inbox `(form_id, is_spam, created_at DESC, id DESC)` over every row (≈ 14× the entries of the partial chip index, still small); `JSON` setting values. The same ✖ as the limiter store |
| Redis (ElastiCache / MemoryDB) | ✅ chosen for limiter state | one round trip per check, a TTL per key equal to the window, 1 GB fits any node; the data is disposable because the limiter fails open, so ElastiCache, not MemoryDB |
| DynamoDB | ✔ for counters, ✖ for verdicts | atomic `ADD` plus a TTL attribute give fixed-window counters, but update expressions have no min/max, so a token bucket needs read-modify-write retries, and each check is a slower network call than Redis; verdicts belong in the lead's own row so *Not spam* restores it exactly |

**Pick:** Redis (`cache.m7g.large`, primary + replica) for the counters; PostgreSQL (task 16's `db.m7g.xlarge`, Multi-AZ) for the verdict columns and settings. Postgres is the system of record here; Redis state is derived and disposable (fail open, a lost key is a fresh start). The only Postgres-specific feature used is the partial index, and a composite index replaces it.

**If it were MySQL instead:** the same columns and logic; the two partial indexes become the composite indexes above, and the settings value is `JSON`. Nothing material otherwise. If task 16's lead database is MySQL, this lesson adds nothing that would need a different engine.

**What would flip the choice:** the limiter outgrowing one Redis core (~100k ops/s; section 15's all-traffic case is ~2M ops/s) means cluster mode or per-pod coarse limiters, still Redis. Spam labels growing into billions for classifier training means exporting them to S3 and Athena or ClickHouse, not changing the OLTP store.
