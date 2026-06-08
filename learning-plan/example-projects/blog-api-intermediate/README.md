# Blog API — intermediate

A blog backend: posts with **tags** (many-to-many) and **comments** (one-to-many),
listed with **pagination**, title search and tag/published filters, each post
auto-assigned a unique **slug**, all behind a real **middleware chain**.

It is the **third project** in the example-projects track. It builds on the GORM
basics from `task-manager-beginner` and adds **associations**, **pagination**, and
a dedicated **middleware** package.

---

## What you'll see

```bash
# create a post with tags (tags are normalised to lowercase and de-duplicated)
curl -s -X POST localhost:8080/posts -H 'Content-Type: application/json' \
  -d '{"title":"Intro to Go","body":"hello go","published":true,"tags":["Go","Web"]}'
# -> {"id":1,"slug":"intro-to-go","tags":["go","web"],...}

# a second post with the same title gets a unique slug
# -> {"slug":"intro-to-go-2",...}

# list with pagination metadata
curl -s 'localhost:8080/posts?page=1&page_size=2'
# -> {"items":[...],"page":1,"page_size":2,"total":4,"total_pages":2}

# filter by tag / published, or search the title
curl -s 'localhost:8080/posts?tag=go'
curl -s 'localhost:8080/posts?published=true'
curl -s 'localhost:8080/posts?q=testing'

# comments (author defaults to "anonymous")
curl -s -X POST localhost:8080/posts/1/comments -H 'Content-Type: application/json' \
  -d '{"author":"Stan","body":"great post"}'

# a single post comes back with its comments preloaded
curl -s localhost:8080/posts/1

# every response carries a request id from the middleware
curl -sD - -o /dev/null localhost:8080/posts | grep -i x-request-id
```

## Routes

| Method | Path                      | Purpose                                   | Success |
|--------|---------------------------|-------------------------------------------|---------|
| POST   | `/posts`                  | Create a post (slug generated)            | 201     |
| GET    | `/posts`                  | List posts (paginated, filtered)          | 200     |
| GET    | `/posts/{id}`             | Get one post + its comments               | 200     |
| PUT    | `/posts/{id}`             | Update a post (slug stays fixed)          | 200     |
| DELETE | `/posts/{id}`             | Delete a post (cascades comments)         | 204     |
| POST   | `/posts/{id}/comments`    | Add a comment to a post                   | 201     |
| GET    | `/posts/{id}/comments`    | List a post's comments                    | 200     |

### List query parameters

| Param       | Values            | Effect                                |
|-------------|-------------------|---------------------------------------|
| `q`         | any text          | case-insensitive title search (ILIKE) |
| `tag`       | a tag name        | only posts carrying that tag          |
| `published` | `true` / `false`  | filter by publish state               |
| `page`      | int ≥ 1           | page number (default 1)               |
| `page_size` | 1–50              | items per page (default 10)           |

## Tech stack

- **Go** standard-library HTTP (Go 1.22+ method+pattern routing).
- **GORM** with two association styles: `hasMany` (post → comments) and
  `many2many` (post ↔ tags), plus `Preload`, a subquery `comment_count`,
  and a transaction for the update path.
- **Postgres 16** + **Docker Compose**.

## Architecture

Same inward-pointing dependency rule, now with a `middleware` package:

```
            middleware (request-id -> logging -> recover)
                              |
handler  ->  service  ->  domain  <-  repository
                           (core)
```

The GORM models (`post`, `tag`, `comment`) live in
`internal/repository/postgres` and are mapped to/from the domain entities
(`domain.Post`, `domain.Comment`) — the business core never imports GORM.

Two lessons worth calling out:

- **Associations.** `post` *hasMany* `comment` (with `Preload` and an
  `OnDelete:CASCADE` foreign key) and *many2many* `tag` through a `post_tags`
  join table. Listing fills a read-only `comment_count` via a correlated subquery
  so it doesn't load every comment.
- **Stable slugs.** A slug is generated once from the title and stays fixed on
  update — good for permalinks and simpler than re-checking uniqueness each edit.

### Layout

```
blog-api-intermediate/
├── cmd/api/main.go                              # wiring + GORM connect/retry + AutoMigrate + shutdown
├── internal/
│   ├── config/config.go                         # env-driven config
│   ├── domain/blog.go                           # Post & Comment entities, PostFilter, generic Page[T], repo interface, errors
│   ├── repository/postgres/post_repository.go   # GORM models + associations + paginated/filtered queries
│   ├── service/post_service.go                  # validation, slug generation, pagination rules
│   ├── middleware/middleware.go                 # RequestID, Logger, Recover, Chain
│   ├── handler/
│   │   ├── post_handler.go                        # post + comment controllers, DTOs
│   │   └── response.go                            # JSON helpers
│   └── router/router.go                         # routes + middleware chain
├── Dockerfile
├── docker-compose.yml
├── .env
├── go.mod / go.sum
├── progress.md
└── README.md
```

## Run it

```bash
docker compose up --build
```

Postgres comes up first (healthcheck-gated); the app retries until the DB is
ready, runs `AutoMigrate` (creating `posts`, `tags`, `comments` and the
`post_tags` join table), then serves on `:8080`. The GORM logger is set to
**Info**, so the generated SQL for every request shows in the logs.

Tear down (and drop the data volume): `docker compose down -v`

### Run outside Docker

```bash
docker compose up -d db
go run ./cmd/api
```

## Concepts this project teaches

- GORM associations: `hasMany` + `Preload`, `many2many` through a join table,
  `Association(...).Replace`, and `Select(clause.Associations)` on delete.
- Avoiding N+1 on a list with a correlated-subquery `comment_count` column (`gorm:"->"`).
- Pagination done correctly: a separate `Count` query plus `Limit`/`Offset`, and
  computing `total_pages`.
- Building safe dynamic filters (`ILIKE` search, a tag subquery) without string-building SQL.
- Slug generation and uniqueness handling.
- A composable middleware chain in its own package (request id propagated via
  `context`, status captured for logging, panic recovery).
- A generic `Page[T]` result type.
