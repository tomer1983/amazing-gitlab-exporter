# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /build/amazing-gitlab-exporter \
    ./cmd/amazing-gitlab-exporter

# Runtime stage
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source="https://github.com/tomer1983/amazing-gitlab-exporter"
LABEL org.opencontainers.image.description="Prometheus exporter for GitLab CI/CD analytics"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /build/amazing-gitlab-exporter /usr/local/bin/amazing-gitlab-exporter

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/amazing-gitlab-exporter"]
CMD ["run"]
