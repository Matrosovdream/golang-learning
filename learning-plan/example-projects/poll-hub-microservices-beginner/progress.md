# Poll Hub — beginner microservices · Progress

Build shared toolkit first, then service by service (poll → vote → stats);
each service must compile (`go build ./services/<name>`) before moving on.
Style here is **flat per-service packages**, not the clean-arch layers.

> ▶ **Resume here:** reference copy fully authored and verified
> (`gofmt` clean, `go vet` clean, `go build ./...` OK, `docker compose config` OK).
> Next step for the rebuild: scaffold + shared `internal/` packages.
>
> User's rebuild lives at **`projects/poll-hub/`** (repo root). This reference
> copy stays untouched.

### 🧱 Scaffold
- [ ] Folder tree created (`db/`, `internal/{env,httpx,postgres}`, `services/{poll,vote,stats}`)
- [ ] `go mod init pollhub` + `go get github.com/jackc/pgx/v5`

### 🧰 Shared toolkit (used by all three services)
- [ ] internal/env/env.go — `Get(key, fallback)`
- [ ] internal/httpx/httpx.go — `JSON` / `Error` / `Decode` / `Log` / `Serve`
- [ ] internal/postgres/postgres.go — pool with connect-retry

### 🐘 Database
- [ ] db/init.sql — 3 tables, the two key constraints
      (`UNIQUE (poll_id, voter)`, composite FK option↔poll), starter poll

### 🟢 poll-service (:8081)
- [ ] services/poll/store.go — CreatePoll (tx!), ListPolls, GetPoll, ClosePoll, DeletePoll
- [ ] services/poll/handlers.go — routes + validation
- [ ] services/poll/main.go
- [ ] compiles: `go build ./services/poll`

### 🗳 vote-service (:8082)
- [ ] services/vote/pollclient.go — HTTP client to poll-service (timeout, 404 → errPollNotFound)
- [ ] services/vote/store.go — CastVote (23505 → errAlreadyVoted), ListVotes
- [ ] services/vote/handlers.go — castVote decision ladder (404/502/409/400)
- [ ] services/vote/main.go
- [ ] compiles: `go build ./services/vote`

### 📊 stats-service (:8083)
- [ ] services/stats/store.go — PollResults (GROUP BY + LEFT JOIN), TopPolls
- [ ] services/stats/handlers.go — results + `?limit=` validation
- [ ] services/stats/main.go
- [ ] compiles: `go build ./services/stats`

### 🐳 Infra
- [ ] Dockerfile (one file, `ARG SERVICE`)
- [ ] docker-compose.yml (db + 3 services, healthcheck, init.sql mount)
- [ ] .dockerignore

### ▶ Run & verify
- [ ] `docker compose up --build` — all four containers healthy
- [ ] curl pass from README: create → vote → duplicate-vote 409 →
      wrong-option 400 → results → close → vote-after-close 409 → top
- [ ] bonus: stop the `poll` container and try voting — expect 502
