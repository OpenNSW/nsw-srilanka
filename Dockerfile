# The Alpine Go image is alpine:3.24 plus ca-certificates — it ships no git and
# no compiler. Neither is needed: modules arrive from GOPROXY as zips, and every
# build below sets CGO_ENABLED=0, so nothing links libc.
#
# Pinned to $BUILDPLATFORM so the compile runs natively and cross-compiles to the
# target arch, instead of running an emulated toolchain once per platform.
# Requires BuildKit.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24 AS builder

WORKDIR /src

# Cache go.mod / go.sum first so the dependency layer survives source edits.
# Every dependency is pinned in go.mod and checksummed in go.sum, and all are
# served by the Go module proxy — no external clone needed.
COPY go.mod go.sum ./
RUN GOWORK=off go mod download

# Copy the full source tree
COPY . .

# TARGETOS/TARGETARCH describe the image being built, not this stage's own
# platform. Declared here rather than higher up on purpose: an ARG makes every RUN
# beneath it platform-specific, so keeping them low lets `go mod download` and the
# source COPY be computed once and shared by every target platform.
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
# Migrate builder – builds the standalone migrator from OpenNSW/tools.
# It lives in a separate module, so we fetch it inside a throwaway module rather
# than adding it to this repo's go.mod. CGO is disabled: both of its database
# drivers, pgx and modernc.org/sqlite, are pure Go. We use `go build -o` rather than `go install`, because
# `go install` refuses to write the binary when cross-compiling for a non-host
# GOOS/GOARCH (multi-arch buildx) — which now binds on every cross-arch target,
# since this stage is $BUILDPLATFORM-pinned. Do not switch it back to go install.
# -------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24 AS migrate-builder

# Kept above the MIGRATE_* ARGs so a version bump invalidates only the fetch
# and build layers below, not these steps. The GOPROXY path needs no git; it is
# installed solely so a `direct`-mode fallback fetch still works.
RUN apk add --no-cache git
WORKDIR /tmp-build
RUN GOWORK=off go mod init migrate-build

# The migrator's package and release, both overridable via --build-arg /
# compose. OpenNSW/tools tags its releases migrator/vX.Y.Z; the version here
# leaves out the migrator/ prefix, as `go get` does. Bump MIGRATE_VERSION to
# adopt a newer release. The Makefile and the Migrations check read both lines.
ARG MIGRATE_PACKAGE=github.com/OpenNSW/tools/migrator/cmd/migrate
ARG MIGRATE_VERSION=v0.1.0

# Fetch kept in its own layer, above the TARGETOS/TARGETARCH ARGs: an ARG makes
# every RUN beneath it platform-specific, so this way one download serves both
# platforms rather than repeating the only network fetch in the build — which has
# already flaked a release on a transient sum.golang.org error.
RUN GOWORK=off go get ${MIGRATE_PACKAGE}@${MIGRATE_VERSION}

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -ldflags="-s -w" -o /out/migrate ${MIGRATE_PACKAGE}

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
LABEL org.opencontainers.image.description="NSW schema migrator (OpenNSW/tools migrator)"

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

# The migrator reads its connection from the db section of a config.yaml
# mounted at runtime and named by CONFIG_PATH: the server's own in compose.yml,
# one of its own (migration.config) in the Helm chart's migration Job. Its migrationDir defaults to ./migrations,
# i.e. the SQL baked in above, relative to this WORKDIR.

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
# (companies) and the config*.example.yaml templates
# land here — the live files they seed are excluded by .dockerignore
# because they carry literal credentials, so no build bakes them regardless of
# the working tree.
# Deployments supply the real files via ConfigMap or bind mount.
# Workflow/form artifacts and the manifest are NOT baked either — they are
# resolved at startup by the pluggable artifact loader, so the image does not
# couple the code to one deployment's workflow content. The code default is the
# local loader reading /app/configs (artifactLoader.type: local), i.e. a bare
# container expects the artifacts to be bind-mounted; the config*.example.yaml
# templates switch this to the GitHub loader pointed at
# OpenNSW/one-trade-artifacts (see their artifactLoader section). A host bind mount over /app/configs (docker-compose)
# takes precedence over anything baked here.
COPY --chown=1001:0 --from=builder /src/configs /app/configs

# The blob storage mount point, and the only path the server writes to
# (storage.type: local, storage.local.baseDir: ./bucket). It is group-0 writable
# the running UID may be assigned by the cluster and is unknown at build time;
# a mode-0755 dir owned by 1001 would reject the first file upload.
# Keep storage.local.baseDir at this baked path, or mount a volume over it: /app
# itself is deliberately not group-writable, so pointing the driver at a sibling
# directory it has to create would fail under an arbitrary UID. Hardened
# deployments should prefer storage.type: s3 or an emptyDir/PVC mounted here —
# the latter is required if readOnlyRootFilesystem is enabled.
RUN mkdir -p /app/bucket \
    && chown 1001:0 /app/bucket \
    && chmod g+w /app/bucket

# Numeric so the kubelet can verify it against runAsNonRoot: true — it cannot
# resolve names. A cluster that assigns its own UID overrides this either way.
USER 1001

# Expose application port (server.port in config.yaml)
EXPOSE 8080

# 127.0.0.1 rather than localhost: busybox wget resolves localhost to ::1 first
# and tries only that address, while the server's :8080 bind degrades to
# IPv4-only wherever the container has no IPv6. The port is server.port's
# default; a deployment that changes it overrides this check too.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

# Default command
CMD ["/app/server"]
