# Auth Service — intermediate

A user-accounts service: register with a hashed password, log in to receive a
**JWT**, and call a protected endpoint that only works with a valid token.

It is the **fourth project** in the example-projects track. It returns to
**raw SQL** — this time through **`sqlx`** over the pgx stdlib driver (a
different flavour from project 1's native pgx) — and adds password hashing,
token issuing/verifying, and an authentication middleware.

---

## What you'll see

```bash
# register (email is normalised to lowercase; password is bcrypt-hashed)
curl -s -X POST localhost:8080/register -H 'Content-Type: application/json' \
  -d '{"email":"stan@example.com","password":"supersecret"}'
# -> 201 {"id":1,"email":"stan@example.com","created_at":"..."}

# registering the same email again is rejected
# -> 409 {"error":"email already registered"}

# log in to get a token
TOKEN=$(curl -s -X POST localhost:8080/login -H 'Content-Type: application/json' \
  -d '{"email":"stan@example.com","password":"supersecret"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')

# the protected route rejects requests with no/!invalid token...
curl -i localhost:8080/me                       # -> 401
# ...and returns your profile with a valid one
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/me
# -> 200 {"id":1,"email":"stan@example.com","created_at":"..."}
```

A wrong password and an unknown email both return the **same** `401 invalid
email or password`, so the API never reveals which accounts exist.

## Routes

| Method | Path        | Auth        | Purpose                       | Success |
|--------|-------------|-------------|-------------------------------|---------|
| POST   | `/register` | public      | Create an account             | 201     |
| POST   | `/login`    | public      | Exchange credentials for a JWT| 200     |
| GET    | `/me`       | Bearer token| Return the current user       | 200     |

## Tech stack

- **Go** standard-library HTTP (Go 1.22+ routing).
- **Raw SQL** via **`sqlx`** (`GetContext`, `QueryRowxContext`, `db:"..."` struct tags)
  over the **pgx stdlib** driver (`database/sql`).
- **bcrypt** (`golang.org/x/crypto/bcrypt`) for password hashing.
- **JWT** (`github.com/golang-jwt/jwt/v5`), HS256, subject = user id.
- **Postgres 16** + **Docker Compose**; schema applied via initdb (like project 1).

## Architecture

The dependency rule still points inward; a separate `auth` package owns tokens
so both the service (issue) and the middleware (verify) can use it without an
import cycle:

```
                 auth.JWTManager (issue / verify JWT)
                  /                         \
   service (issue) ----> domain <---- repository (sqlx)
        ^                                      
        |                                      
   handler  <--- middleware.Auth (verify) wraps /me
```

Notable decisions:

- The service depends on a small **`TokenIssuer` interface**, not on the JWT
  package directly — so the business core has no JWT import.
- The **Auth middleware** takes a `parse func(string) (int64, error)` and stores
  the user id in the request `context`; the handler reads it back with
  `middleware.UserIDFrom`. Only `/me` is wrapped; `/register` and `/login` are public.
- Login failures are deliberately indistinguishable (`ErrInvalidCredentials`).

### Layout

```
auth-service-intermediate/
├── cmd/api/main.go                              # wiring + sqlx connect/retry + shutdown (registers pgx stdlib driver)
├── internal/
│   ├── config/config.go                         # env config incl. JWT_SECRET, TOKEN_TTL
│   ├── domain/user.go                           # User entity, repo interface, errors
│   ├── auth/token.go                            # JWTManager: Generate / Parse (HS256)
│   ├── repository/postgres/user_repository.go   # raw SQL via sqlx; unique-violation -> ErrEmailTaken
│   ├── service/auth_service.go                  # validation, bcrypt hashing, login, TokenIssuer interface
│   ├── middleware/middleware.go                 # RequestID, Logger, Recover, Auth, Chain
│   ├── handler/
│   │   ├── auth_handler.go                        # register/login/me controllers, DTOs
│   │   └── response.go                            # JSON helpers
│   └── router/router.go                         # public vs protected route wiring
├── migrations/001_init.sql                      # users table (applied by Postgres initdb)
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

Postgres comes up first and applies `migrations/001_init.sql` on first boot; the
app retries the connection until the DB is ready and serves on `:8080`.

> The default `JWT_SECRET` in `.env` is for local development only — set a real
> secret via the environment for anything beyond your laptop.

Tear down (and drop the data volume): `docker compose down -v`

### Run outside Docker

```bash
docker compose up -d db
go run ./cmd/api
```

## Concepts this project teaches

- Raw SQL with `sqlx`: struct-tag scanning, `GetContext`/`QueryRowxContext`,
  `$1` placeholders, and registering the pgx stdlib driver for `database/sql`.
- Secure password storage with bcrypt (`GenerateFromPassword` / `CompareHashAndPassword`).
- Stateless auth with JWT: signing, verifying (and rejecting unexpected
  signing algorithms), and carrying the user id as the subject claim.
- An authentication middleware that gates routes and passes identity via `context`.
- Keeping a third-party concern (JWT) out of the core via an interface.
- Security hygiene: never returning the password hash, and uniform login errors.
