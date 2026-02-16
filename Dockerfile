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

FROM gcr.io/distroless/static-debian12:latest

COPY --from=build /out/ct-archive-serve /ct-archive-serve

USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/ct-archive-serve"]

