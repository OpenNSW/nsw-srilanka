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
# Migrate builder – builds the standalone migrator from nsw-agency.
# It lives in a separate module, so we fetch it at a pinned pseudo-version
# inside a throwaway module rather than adding it to this repo's go.mod. CGO is
# disabled: the tool imports go-sqlite3, but we only drive postgres (pure-Go
# pgx), so sqlite stays an unused runtime stub. We use `go build -o` rather than
# `go install`, because `go install` refuses to write the binary when
# cross-compiling for a non-host GOOS/GOARCH (multi-arch buildx).
# -------------------------------------------------------------------
FROM golang:1.26.3-bookworm AS migrate-builder

ARG TARGETOS
ARG TARGETARCH

# Version-independent setup is kept above the MIGRATE_VERSION ARG so a version
# bump only invalidates the fetch+build layer below, not these cached steps.
WORKDIR /tmp-build
RUN GOWORK=off go mod init migrate-build

# Bump to adopt a newer migrator (overridable via --build-arg / compose). No
# semver tag exists on nsw-agency/backend yet, so this is a pinned pseudo-version.
ARG MIGRATE_VERSION=v0.0.0-20260610120959-d981e67a7a47
RUN GOWORK=off go get github.com/OpenNSW/nsw-agency/backend/cmd/migrate@${MIGRATE_VERSION} \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
       go build -ldflags="-s -w" -o /out/migrate github.com/OpenNSW/nsw-agency/backend/cmd/migrate

# -------------------------------------------------------------------
# Migrate image – self-contained schema migrator. Runs to completion
# (compose gates `api` on it via service_completed_successfully) and
# is also usable ad hoc: `docker compose run --rm migrate status`.
# The SQL files are baked in so the image needs no bind mount.
# Built explicitly via `--target migrate`; `runtime` is kept LAST so a
# bare `docker build .` (and any consumer without an explicit target)
# resolves to the server image, not the migrator.
# -------------------------------------------------------------------
FROM debian:bookworm-slim AS migrate

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW schema migrator (nsw-agency migrate tool)"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -r -s /sbin/nologin -d /app appuser

WORKDIR /app

COPY --from=migrate-builder /out/migrate /usr/local/bin/migrate
COPY migrations/ /app/migrations/

# Tell the migrator where the baked-in SQL lives; the postgres connection
# is supplied via DB_* env vars at runtime (see compose.yml).
ENV MIGRATION_DIR=/app/migrations \
    DB_DRIVER=postgres

USER appuser

# Apply all pending migrations by default; override with status/down/generate.
CMD ["migrate", "up"]

# -------------------------------------------------------------------
# Runtime image – minimal, non‑root, with healthcheck and labels.
# Kept as the LAST stage so it is the default build target.
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

# Copy the binary.
COPY --from=builder /out/server /app/server

# Bake application configs into the image. These files are tracked in git and
# version with the code (workflow/form definitions, payment_methods.json,
# manifest, notification, etc.), and the payment registry reads them at boot.
# Environment-specific values (e.g. services.json) are still overlaid at runtime
# via ConfigMap/bind mount; a host bind mount over /app/configs (docker-compose)
# also continues to take precedence over what is baked here.
COPY --from=builder /src/configs /app/configs

# Create the writable blob storage mount point and normalise ownership.
RUN mkdir -p /app/bucket \
    && chown -R appuser:appuser /app

USER appuser

# Expose application port (configurable via SERVER_PORT env var)
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Default command
CMD ["/app/server"]

