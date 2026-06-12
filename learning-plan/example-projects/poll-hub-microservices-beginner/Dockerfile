# One Dockerfile builds any of the three services — the build arg picks
# which one:  docker build --build-arg SERVICE=poll .
# Compose passes the arg per service, so there is exactly one file to maintain.

FROM golang:1.26-alpine AS build
ARG SERVICE
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./services/${SERVICE}

# The final image is just the static binary on a tiny base.
FROM alpine:3.21
COPY --from=build /bin/app /bin/app
ENTRYPOINT ["/bin/app"]
