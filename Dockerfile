FROM golang:1.25.5-bookworm AS build

WORKDIR /src

COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/

ENV CGO_ENABLED=0

RUN go build -trimpath -ldflags="-s -w" -o /out/ct-archive-serve ./cmd/ct-archive-serve

FROM gcr.io/distroless/static-debian12:latest

COPY --from=build /out/ct-archive-serve /ct-archive-serve

USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/ct-archive-serve"]

