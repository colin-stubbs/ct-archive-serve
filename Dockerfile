FROM docker.io/golang:1.25.7-bookworm AS build

WORKDIR /src

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./

# Download dependencies (cached layer if go.mod/go.sum unchanged)
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

ENV CGO_ENABLED=0

RUN go build -trimpath -ldflags="-s -w" -o /out/ct-archive-serve ./cmd/ct-archive-serve

FROM debian:trixie-slim

COPY --from=build /out/ct-archive-serve /ct-archive-serve

RUN mkdir -p /var/log/ct/archive && \
  chown -R 65534:65534 /var/log/ct/archive && \
  apt-get update && \
  apt-get install --yes --no-install-recommends curl && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

USER 65534:65534

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD curl -s -f http://localhost:8080/logs.v3.json | grep -q 'log_list_timestamp' || exit 1

ENTRYPOINT ["/ct-archive-serve"]

