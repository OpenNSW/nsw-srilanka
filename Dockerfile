FROM golang:1.25-bookworm AS builder

# Install git (required for cloning the nsw shared library)
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

# Clone the nsw shared library (public repo; Go module lives in backend/)
ARG NSW_REF=taskv2
RUN git clone --depth 1 --branch ${NSW_REF} \
    https://github.com/OpenNSW/nsw.git /deps/nsw

WORKDIR /src

# Cache go.mod / go.sum first
COPY go.mod go.sum ./
# Rewrite the local replace directive to point at the cloned copy
RUN go mod edit -replace github.com/OpenNSW/nsw=/deps/nsw/backend

# Download dependencies
RUN GOWORK=off go mod download

# Copy the full source tree
COPY . .
# Re-apply replace (COPY overwrites go.mod with the repo's original)
RUN go mod edit -replace github.com/OpenNSW/nsw=/deps/nsw/backend

# Ensure bucket directory exists for runtime data
RUN mkdir -p /src/bucket

# Build the binary (adjust path if main package differs)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOWORK=off \
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

# Copy binary and required runtime assets
COPY --from=builder /out/server /app/server
COPY --from=builder /deps/nsw/backend/configs /app/configs
COPY --from=builder /src/bucket /app/bucket

# Adjust ownership
RUN chown -R appuser:appuser /app

USER appuser

# Expose application port (configurable via SERVER_PORT env var)
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Default command
CMD ["/app/server"]
