# musl base. Every native addon this build needs (@swc/core, @tailwindcss/oxide,
# lightningcss, @rolldown/binding) has a `libc: [musl]` variant pinned in
# pnpm-lock.yaml, which is what --frozen-lockfile resolves to here.
#
# Pinned to $BUILDPLATFORM: `pnpm build` emits only text assets, so dist/ is
# architecture-independent and one native build serves every target. This stage
# therefore runs once however many platforms are requested — only the nginx stage
# below is per-platform. Requires BuildKit.
FROM --platform=$BUILDPLATFORM node:24-alpine3.24 AS builder

WORKDIR /workspace

RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.json ./
COPY apps/trader-app/package.json apps/trader-app/package.json
COPY packages/ui/package.json packages/ui/package.json
COPY packages/theme/package.json packages/theme/package.json

RUN pnpm install --frozen-lockfile

COPY . .

ARG BUILD_VERSION=dev
ENV VITE_APP_VERSION=$BUILD_VERSION

RUN pnpm --filter trader-app build

# Alpine's own nginx package keeps nginx on the same security feed as the rest
# of the base image, so `apk upgrade` picks up fixes without waiting for an
# upstream nginx image rebuild. It also installs only core nginx (no perl, njs,
# xslt, geoip, or image-filter modules), keeping the CVE surface small.
FROM alpine:3.24 AS runtime

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Trader Portal Frontend"

# Create the nginx user with UID/GID 101 before installing nginx so the package
# reuses it, matching the non-root UID documented in deployments/helm/lk-tnsw/values.yaml.
# Upgrade Alpine packages so security fixes published after the base image was
# built are included; CI scans this image and fails on a fixable CRITICAL CVE.
RUN addgroup -g 101 -S nginx \
  && adduser -u 101 -S -D -H -h /var/cache/nginx -s /sbin/nologin -G nginx nginx \
  && apk upgrade --no-cache \
  && apk add --no-cache nginx \
  && rm -f /etc/nginx/http.d/default.conf \
  && mkdir -p /usr/share/nginx/html /var/cache/nginx /var/log/nginx /run/nginx \
  && chgrp -R 0 /var/cache/nginx /var/log/nginx /run/nginx /etc/nginx /usr/share/nginx/html \
  && chmod -R g+w /var/cache/nginx /var/log/nginx /run/nginx /etc/nginx /usr/share/nginx/html

COPY apps/trader-app/nginx.conf /etc/nginx/nginx.conf
COPY --chown=101:0 --from=builder /workspace/apps/trader-app/dist /usr/share/nginx/html

# config.js is not generated here — it's supplied externally: bind-mounted by
# docker-compose, or mounted from a ConfigMap by Helm (see
# deployments/helm/lk-tnsw/templates/frontend/configmap.yaml).

USER 101
EXPOSE 8080

# Use 127.0.0.1 (not localhost): busybox wget resolves localhost to ::1 first,
# but nginx listens on IPv4 only, which would fail the check.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1

# Graceful shutdown: nginx finishes in-flight requests on SIGQUIT, whereas
# SIGTERM stops workers immediately.
STOPSIGNAL SIGQUIT

# Pass -e /dev/stderr so nginx doesn't try to open its compiled-in default
# error log path before parsing nginx.conf (which fails under non-root / read-only rootfs).
CMD ["nginx", "-e", "/dev/stderr", "-g", "daemon off;"]

