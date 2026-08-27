# One image for every Go binary of the repository — the platform's own and the
# demo venue's: the service is picked by the compose `command`, so they share
# the same layers instead of being built twice.
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/... ./venue/cmd/...

FROM alpine:3.21

# ca-certificates for outbound TLS; busybox wget is what the compose
# healthchecks call.
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 app

COPY --from=build /out/ /app/

USER app
WORKDIR /app

CMD ["/app/kitchen-api"]
