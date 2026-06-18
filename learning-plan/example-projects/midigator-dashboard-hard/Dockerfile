# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build all three binaries (api, worker, seed) into /app.
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api    ./cmd/api  && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux go build -o /app/seed   ./cmd/seed

# ---- run stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/api    /app/api
COPY --from=build /app/worker /app/worker
COPY --from=build /app/seed   /app/seed
USER appuser
EXPOSE 8080
# CMD (not ENTRYPOINT) so docker-compose `command:` fully replaces it for the
# worker and seed services instead of being appended as arguments.
CMD ["/app/api"]
