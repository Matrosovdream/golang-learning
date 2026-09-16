# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO off: go-ethereum then uses its pure-Go secp256k1 (same signatures, verified by the golden test)
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/node ./cmd/node \
 && CGO_ENABLED=0 GOOS=linux go build -o /app/wallet ./cmd/wallet

# ---- run stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 appuser && mkdir /data && chown appuser /data
WORKDIR /app
COPY --from=build /app/node /app/wallet /app/
USER appuser
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/app/node"]
