# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/api

# ---- run stage ----
FROM alpine:3.20
# this app makes NO outbound calls (the broker is fully in-memory), so no
# ca-certificates package is needed — just a non-root user to run as.
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/server /app/server
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]
