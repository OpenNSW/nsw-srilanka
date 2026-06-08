FROM golang:1.26.3-bookworm AS builder

WORKDIR /src

# Cache go.mod / go.sum first. All dependencies — including
# github.com/OpenNSW/nsw/backend, which is pinned in go.mod and checksummed
# in go.sum — are fetched from the Go module proxy. No external clone needed.
COPY go.mod go.sum ./
RUN GOWORK=off go mod download

# Copy the full source tree
COPY . .

# Build the binary (adjust path if main package differs).
# BuildKit populates TARGETOS/TARGETARCH for cross/multi-arch builds. When they
# are empty (e.g. no BuildKit), leaving GOOS/GOARCH unset lets Go target the
# builder's native platform — avoids forcing amd64 emulation on arm64 hosts.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -ldflags="-s -w" -o /out/server ./cmd/server

# -------------------------------------------------------------------
# Runtime image – minimal, non‑root, with healthcheck and labels
# -------------------------------------------------------------------
FROM debian:bookworm-slim AS runtime

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Backend API Service (built from nsw‑srilanka)"

# Install runtime dependencies and create non‑root user
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates wget \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -s /sbin/nologin -d /app appuser

WORKDIR /app

# Copy the binary. Configs are NOT baked into the image — they are
# environment data, mounted at /app/configs at runtime (see docker-compose.yml).
COPY --from=builder /out/server /app/server

# Create the mount points for runtime config and blob storage. /app/configs
# is overlaid by a read-only bind mount; /app/bucket by a writable volume.
RUN mkdir -p /app/configs /app/bucket \
    && chown -R appuser:appuser /app

USER appuser

# Expose application port (configurable via SERVER_PORT env var)
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Default command
CMD ["/app/server"]

