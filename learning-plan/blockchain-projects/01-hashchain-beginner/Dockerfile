# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
# stdlib only: no dependencies to download, so no separate go.mod layer
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/node ./cmd/node

# ---- run stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /app/node /app/node
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/node"]
