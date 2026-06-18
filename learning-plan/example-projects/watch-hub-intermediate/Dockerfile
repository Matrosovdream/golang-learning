# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/api

# ---- run stage ----
FROM alpine:3.20
# the health checks make outbound HTTPS calls, so we need CA certificates
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/server /app/server
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]
