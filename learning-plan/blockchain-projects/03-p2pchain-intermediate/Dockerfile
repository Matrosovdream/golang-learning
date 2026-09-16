# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/node ./cmd/node \
 && CGO_ENABLED=0 GOOS=linux go build -o /app/chainctl ./cmd/chainctl \
 && CGO_ENABLED=0 GOOS=linux go build -o /app/peerid ./cmd/peerid

# ---- run stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 appuser && mkdir /data && chown appuser /data
WORKDIR /app
COPY --from=build /app/node /app/chainctl /app/peerid /app/
USER appuser
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080 4001
ENTRYPOINT ["/app/node"]
