# Task Manager — beginner

A full-CRUD task API: create tasks with a status, priority, and optional due
date; list them with filtering, search, and sorting; update and delete.

It is the **second project** in the example-projects track. It reuses the clean
-architecture skeleton from `url-shortener-beginner` but swaps the data layer
from raw SQL to **GORM**, and stores the schema via GORM **AutoMigrate** instead
of an init SQL script.

---

## What you'll see

```bash
# create
curl -s -X POST localhost:8080/tasks -H 'Content-Type: application/json' \
  -d '{"title":"Write README","priority":"high","due_date":"2026-06-20"}'
# -> {"id":1,"title":"Write README","status":"todo","priority":"high","due_date":"2026-06-20",...}

# list with a filter + search + sort
curl -s 'localhost:8080/tasks?priority=high&sort=due_date&order=asc'
curl -s 'localhost:8080/tasks?q=readme'

# update (full replace) and delete
curl -s -X PUT localhost:8080/tasks/1 -H 'Content-Type: application/json' \
  -d '{"title":"Write README","status":"done","priority":"high"}'
curl -s -X DELETE localhost:8080/tasks/1 -i   # -> 204 No Content
```

Validation is enforced: an unknown `status`/`priority` or a missing `title`
returns `400` with a message; an unknown id returns `404`.

## Routes

| Method | Path           | Purpose                          | Success |
|--------|----------------|----------------------------------|---------|
| POST   | `/tasks`       | Create a task                    | 201     |
| GET    | `/tasks`       | List tasks (filter/search/sort)  | 200     |
| GET    | `/tasks/{id}`  | Get one task                     | 200     |
| PUT    | `/tasks/{id}`  | Replace a task                   | 200     |
| DELETE | `/tasks/{id}`  | Delete a task                    | 204     |

### List query parameters

| Param      | Values                                   | Effect                          |
|------------|------------------------------------------|---------------------------------|
| `status`   | `todo` `in_progress` `done`              | filter by status                |
| `priority` | `low` `medium` `high`                    | filter by priority              |
| `q`        | any text                                 | case-insensitive title search   |
| `sort`     | `created_at` `due_date` `title` `priority` | sort column (default `created_at`) |
| `order`    | `asc` `desc`                             | sort direction (default `desc`) |

## Task fields

| Field         | Type           | Notes                                            |
|---------------|----------------|--------------------------------------------------|
| `title`       | string         | required, ≤ 200 chars                            |
| `description` | string         | optional                                         |
| `status`      | enum           | `todo` (default), `in_progress`, `done`          |
| `priority`    | enum           | `low`, `medium` (default), `high`                |
| `due_date`    | `YYYY-MM-DD`   | optional; omit or `null` to clear                |
| `created_at` / `updated_at` | RFC3339 | managed automatically by GORM            |

## Tech stack

- **Go** standard-library HTTP (Go 1.22+ method+pattern routing) — no framework.
- **GORM** (`gorm.io/gorm` + `gorm.io/driver/postgres`) for data access.
- **GORM AutoMigrate** builds/updates the `tasks` table on startup (no SQL file).
- **Postgres 16** + **Docker Compose**.

## Architecture

Same dependency rule as project 1 — pointing **inward**:

```
handler  ->  service  ->  domain  <-  repository
                           (core)
```

The notable lesson here: **your ORM model is not your domain entity.**
`domain.Task` is a plain struct with no tags. The GORM model `taskModel` lives in
`internal/repository/postgres` with all the `gorm:"..."` tags, and the repository
maps between the two (`toModel` / `toDomain`). The business core never imports GORM.

### Layout

```
task-manager-beginner/
├── cmd/api/main.go                              # wiring + GORM connect/retry + AutoMigrate + graceful shutdown
├── internal/
│   ├── config/config.go                         # env-driven config
│   ├── domain/task.go                           # Task entity, Status/Priority enums, TaskFilter, repo interface, errors
│   ├── repository/postgres/task_repository.go   # GORM model + mapping + queries (filter/search/sort)
│   ├── service/task_service.go                  # validation, defaults, orchestration
│   ├── handler/
│   │   ├── task_handler.go                        # CRUD controllers + request/response DTOs
│   │   └── response.go                            # JSON write helpers
│   └── router/router.go                         # routes + logging/recover middleware
├── Dockerfile
├── docker-compose.yml                           # app + postgres
├── .env
├── go.mod / go.sum
├── progress.md
└── README.md
```

## Run it

```bash
docker compose up --build
```

Postgres comes up first (healthcheck-gated); the app retries the connection
until the DB is ready, runs `AutoMigrate`, then serves on `:8080`.

The GORM logger is set to **Info**, so you'll see the generated SQL for every
request in the app logs — handy for learning what the ORM does under the hood.

Tear down (and drop the data volume): `docker compose down -v`

### Run outside Docker

```bash
docker compose up -d db
go run ./cmd/api
```

## Concepts this project teaches

- GORM basics: `Open`, `AutoMigrate`, `Create`, `First`, `Find`, `Updates`, `Delete`,
  and reading `RowsAffected`.
- Keeping the ORM model separate from the domain entity (mapping layer).
- Translating ORM errors (`gorm.ErrRecordNotFound`) into domain errors.
- Building dynamic queries: conditional `Where`, `ILIKE` search, and a
  **whitelisted** `ORDER BY` so user input never reaches SQL directly.
- Input validation and defaults in the service layer, mapped to `400`/`404` at the edge.
- Full REST CRUD semantics and status codes (`201`/`200`/`204`/`400`/`404`).
