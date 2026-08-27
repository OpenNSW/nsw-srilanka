# The Alpine Go image is alpine:3.24 plus ca-certificates — it ships no git and
# no compiler. Neither is needed: modules arrive from GOPROXY as zips, and every
# build below sets CGO_ENABLED=0, so nothing links libc.
FROM golang:1.27-alpine3.24 AS builder

WORKDIR /src

# Cache go.mod / go.sum first so the dependency layer survives source edits.
# Every dependency is pinned in go.mod and checksummed in go.sum, and all are
# served by the Go module proxy — no external clone needed.
COPY go.mod go.sum ./
RUN GOWORK=off go mod download

# Copy the full source tree
COPY . .

# BuildKit populates TARGETOS/TARGETARCH for cross/multi-arch builds. When they
# are empty (e.g. no BuildKit), leaving GOOS/GOARCH unset lets Go target the
# builder's native platform — avoids forcing amd64 emulation on arm64 hosts.
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -ldflags="-s -w -X 'github.com/OpenNSW/nsw-srilanka/internal/version.version=${BUILD_VERSION}'" \
    -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -ldflags="-s -w -X 'github.com/OpenNSW/nsw-srilanka/internal/version.version=${BUILD_VERSION}'" \
    -o /out/otc ./cmd/otc

# -------------------------------------------------------------------
# Migrate builder – builds the standalone migrator from OpenNSW/agency.
# It lives in a separate module, so we fetch it inside a throwaway module rather
# than adding it to this repo's go.mod. CGO is disabled: the tool imports
# go-sqlite3, but we only drive postgres (pure-Go pgx), so sqlite stays an unused
# runtime stub. We use `go build -o` rather than `go install`, because
# `go install` refuses to write the binary when cross-compiling for a non-host
# GOOS/GOARCH (multi-arch buildx).
# -------------------------------------------------------------------
FROM golang:1.27-alpine3.24 AS migrate-builder

ARG TARGETOS
ARG TARGETARCH

# Kept above the MIGRATE_VERSION ARG so a version bump invalidates only the
# fetch+build layer below, not these steps. The GOPROXY path needs no git; it is
# installed solely so a `direct`-mode fallback fetch still works.
RUN apk add --no-cache git
WORKDIR /tmp-build
RUN GOWORK=off go mod init migrate-build

# Bump to adopt a newer migrator (overridable via --build-arg / compose).
# This commit is release v0.3.0. The value must be a pseudo-version rather than
# `v0.3.0`: the module sits in the repo's backend/ subdirectory, so Go honours
# only a `backend/`-prefixed tag, while the release tags sit at the repo root.
# A bare `v0.3.0` resolves the root module and fails with "does not contain
# package .../backend/cmd/migrate", because backend/ is carved out of the root
# module by its own go.mod. Publishing `backend/v0.3.0` upstream would make the
# plain tag usable here.
ARG MIGRATE_VERSION=v0.0.0-20260827070610-a0c2d032a061
RUN GOWORK=off go get github.com/OpenNSW/agency/backend/cmd/migrate@${MIGRATE_VERSION} \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
       go build -ldflags="-s -w" -o /out/migrate github.com/OpenNSW/agency/backend/cmd/migrate

# -------------------------------------------------------------------
# Migrate image – self-contained schema migrator. Runs to completion
# (compose gates `api` on it via service_completed_successfully) and
# is also usable ad hoc: `docker compose run --rm migrate status`.
# The SQL files are baked in so the image needs no bind mount.
# Built explicitly via `--target migrate`; `runtime` is kept LAST so a
# bare `docker build .` (and any consumer without an explicit target)
# resolves to the server image, not the migrator.
# -------------------------------------------------------------------
FROM alpine:3.24 AS migrate

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW schema migrator (OpenNSW/agency migrate tool)"

# ca-certificates for TLS to postgres; tzdata because Alpine carries no zoneinfo.
# -H skips home-dir creation — WORKDIR below makes /app. The UID is pinned so
# file ownership is stable across rebuilds, but the container is not guaranteed
# to run as it: a cluster with a restricted pod-security policy may override
# USER and assign a UID of its own, so do not mirror this value as runAsUser.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1001 -S appuser \
    && adduser -u 1001 -S -D -H -h /app -s /sbin/nologin -G appuser appuser

WORKDIR /app

COPY --from=migrate-builder /out/migrate /usr/local/bin/migrate
COPY migrations/ /app/migrations/

# Tell the migrator where the baked-in SQL lives; the postgres connection
# is supplied via DB_* env vars at runtime (see compose.yml).
ENV MIGRATION_DIR=/app/migrations \
    DB_DRIVER=postgres

# Numeric so the kubelet can verify it against runAsNonRoot: true — it cannot
# resolve names. A cluster that assigns its own UID overrides this either way.
USER 1001

# Apply all pending migrations by default; override with status/down/generate.
CMD ["migrate", "up"]

# -------------------------------------------------------------------
# Runtime image – minimal, non‑root, with healthcheck and labels.
# Kept as the LAST stage so it is the default build target.
# -------------------------------------------------------------------
FROM alpine:3.24 AS runtime

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Backend API Service (built from nsw‑srilanka)"

# ca-certificates for outbound TLS; tzdata because Alpine carries no zoneinfo.
# busybox supplies the wget the HEALTHCHECK uses, so it needs no package of its
# own. -H skips home-dir creation — WORKDIR below makes /app. The UID is pinned
# so file ownership is stable across rebuilds, but the container is not
# guaranteed to run as it: a cluster with a restricted pod-security policy may
# override USER and assign a UID of its own, so do not mirror it as runAsUser.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1001 -S appuser \
    && adduser -u 1001 -S -D -H -h /app -s /sbin/nologin -G appuser appuser

WORKDIR /app

# --chown at copy time avoids a second chown -R layer that would duplicate every
# copied file. Group 0 because a cluster that overrides USER runs the container
# as an arbitrary UID placed in group 0 — that group is then the only identity
# the process is guaranteed to hold.
COPY --chown=1001:0 --from=builder /out/server /app/server
COPY --chown=1001:0 --from=builder /out/otc /usr/local/bin/otc

# Bake the configs directory. Only the committed *.example.json templates
# (services, payment_methods, notification, catalog) and argus/ land here — the
# live *.json files they seed are excluded by .dockerignore because they carry
# literal credentials, so no build bakes them regardless of the working tree.
# Deployments supply the real files via ConfigMap or bind mount.
# Workflow/form artifacts and the manifest are NOT baked either — they are
# resolved at startup by the pluggable artifact loader, so the image does not
# couple the code to one deployment's workflow content. The code default is the
# local loader reading /app/configs (ARTIFACT_LOADER_TYPE=local), i.e. a bare
# container expects the artifacts to be bind-mounted; compose.yml and .env.example
# override this to the GitHub loader pointed at OpenNSW/one-trade-artifacts (see
# the ARTIFACT_* env there). A host bind mount over /app/configs (docker-compose)
# takes precedence over anything baked here.
COPY --chown=1001:0 --from=builder /src/configs /app/configs

# The blob storage mount point, and the only path the server writes to
# (STORAGE_TYPE=local, STORAGE_LOCAL_BASE_DIR=./bucket). It is group-0 writable
# the running UID may be assigned by the cluster and is unknown at build time;
# a mode-0755 dir owned by 1001 would reject the first file upload.
# Keep STORAGE_LOCAL_BASE_DIR at this baked path, or mount a volume over it: /app
# itself is deliberately not group-writable, so pointing the driver at a sibling
# directory it has to create would fail under an arbitrary UID. Hardened
# deployments should prefer STORAGE_TYPE=s3 or an emptyDir/PVC mounted here —
# the latter is required if readOnlyRootFilesystem is enabled.
RUN mkdir -p /app/bucket \
    && chown 1001:0 /app/bucket \
    && chmod g+w /app/bucket

# Numeric so the kubelet can verify it against runAsNonRoot: true — it cannot
# resolve names. A cluster that assigns its own UID overrides this either way.
USER 1001

# Expose application port (configurable via SERVER_PORT env var)
EXPOSE 8080

# 127.0.0.1 rather than localhost: busybox wget resolves localhost to ::1 first
# and tries only that address, while the server's :8080 bind degrades to
# IPv4-only wherever the container has no IPv6.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${SERVER_PORT:-8080}/health || exit 1

# Default command
CMD ["/app/server"]
