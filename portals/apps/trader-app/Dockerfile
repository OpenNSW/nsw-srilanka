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

# Non-perl variant: nginx.conf declares no perl directives, so shipping
# ngx_http_perl_module and a perl interpreter would only widen the CVE surface.
# envsubst templating and /docker-entrypoint.d — which default.conf.template
# relies on — live in the base layer, not this variant.
FROM nginxinc/nginx-unprivileged:stable-alpine3.24 AS runtime

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Trader Portal Frontend"

COPY apps/trader-app/nginx.conf /etc/nginx/templates/default.conf.template
COPY --chown=101:0 --from=builder /workspace/apps/trader-app/dist /usr/share/nginx/html

# config.js is not generated here — it's supplied externally: bind-mounted by
# docker-compose, or mounted from a ConfigMap by Helm (see
# deployments/helm/lk-tnsw/templates/frontend/configmap.yaml).

# Security fixes published after the base image was built; CI scans this image
# and fails on a fixable CRITICAL CVE. nginx and its modules come from nginx.org's
# repository, not Alpine's, and the base image pins each to its exact version, so
# they are added again by name from that repository, which installs its newest
# release. nginx.org's signing key is trusted only when its SHA-512 matches the
# one nginx's own image pins, and is removed again afterwards.
USER root
RUN set -eu; \
  signing_sha512=e09fa32f0a0eab2b879ccbbc4d0e4fb9751486eedda75e35fac65802cc9faa266425edf83e261137a2f4d16281ce2c1a5f4502930fe75154723da014214f0655; \
  wget -qO /tmp/nginx_signing.rsa.pub https://nginx.org/keys/nginx_signing.rsa.pub; \
  echo "$signing_sha512 */tmp/nginx_signing.rsa.pub" | sha512sum -c -; \
  mv /tmp/nginx_signing.rsa.pub /etc/apk/keys/; \
  nginx_pkgs=$(tr ' ' '\n' < /etc/apk/world | grep '^nginx' | cut -d= -f1); \
  apk add --no-cache --upgrade \
    -X "https://nginx.org/packages/alpine/v$(grep -oE '^[0-9]+\.[0-9]+' /etc/alpine-release)/main" \
    $nginx_pkgs; \
  apk upgrade --no-cache; \
  rm /etc/apk/keys/nginx_signing.rsa.pub

# OpenShift Compliance: Fix permissions for random UID (Group 0)
RUN chmod -R g+w /var/cache/nginx /var/run /var/log/nginx /etc/nginx/conf.d /usr/share/nginx/html
USER 101
EXPOSE 8080

# Use 127.0.0.1 (not localhost): busybox wget resolves localhost to ::1 first,
# but nginx listens on IPv4 only, which would fail the check.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1

CMD ["nginx", "-g", "daemon off;"]

