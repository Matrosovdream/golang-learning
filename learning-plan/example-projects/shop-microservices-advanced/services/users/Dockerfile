# Build context is the repo root (shared module). This image builds only the
# users service binary.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./services/users

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/server /app/server
USER appuser
ENTRYPOINT ["/app/server"]
