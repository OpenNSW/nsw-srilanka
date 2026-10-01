# musl base. Every native addon this build needs (@swc/core, @tailwindcss/oxide,
# lightningcss, @rolldown/binding) has a `libc: [musl]` variant pinned in
# pnpm-lock.yaml, which is what --frozen-lockfile resolves to here.
#
# Pinned to $BUILDPLATFORM: `pnpm build` emits only text assets, so dist/ is
# architecture-independent and one native build serves every target. This stage
# therefore runs once however many platforms are requested — only the nginx stage
# below is per-platform. Requires BuildKit.
FROM --platform=$BUILDPLATFORM node:22-alpine3.24 AS builder

WORKDIR /workspace

RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.json ./
COPY apps/trader-app/package.json apps/trader-app/package.json

RUN pnpm install --frozen-lockfile

COPY . .

ARG BUILD_VERSION=dev
ENV VITE_APP_VERSION=$BUILD_VERSION

RUN pnpm --filter trader-app build

# Non-perl variant: nginx.conf declares no perl directives, so shipping
# ngx_http_perl_module and a perl interpreter would only widen the CVE surface.
# envsubst templating and /docker-entrypoint.d — which default.conf.template
# relies on — live in the base layer, not this variant.
FROM nginxinc/nginx-unprivileged:stable-alpine3.24

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Trader Portal Frontend"

COPY apps/trader-app/nginx.conf /etc/nginx/templates/default.conf.template
COPY --chown=101:0 --from=builder /workspace/apps/trader-app/dist /usr/share/nginx/html

# config.js is not generated here — it's supplied externally: bind-mounted by
# docker-compose, or mounted from a ConfigMap by Helm (see
# deployments/helm/lk-tnsw/templates/frontend/configmap.yaml).

# OpenShift Compliance: Fix permissions for random UID (Group 0)
USER root
RUN chmod -R g+w /var/cache/nginx /var/run /var/log/nginx /etc/nginx/conf.d /usr/share/nginx/html
USER 101
EXPOSE 8080

# Use 127.0.0.1 (not localhost): busybox wget resolves localhost to ::1 first,
# but nginx listens on IPv4 only, which would fail the check.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1

CMD ["nginx", "-g", "daemon off;"]

