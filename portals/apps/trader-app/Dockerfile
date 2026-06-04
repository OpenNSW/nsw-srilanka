FROM node:22-bookworm-slim AS builder

WORKDIR /workspace

RUN corepack enable

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.json ./
COPY apps/trader-app/package.json apps/trader-app/package.json

RUN pnpm install --frozen-lockfile

COPY . .

RUN pnpm --filter trader-app build

FROM nginxinc/nginx-unprivileged:stable-alpine3.23-perl

LABEL org.opencontainers.image.source="https://github.com/OpenNSW/nsw-srilanka"
LABEL org.opencontainers.image.description="NSW Trader Portal Frontend"

COPY apps/trader-app/nginx.conf /etc/nginx/templates/default.conf.template
COPY --chown=101:0 --from=builder /workspace/apps/trader-app/dist /usr/share/nginx/html

# OpenShift Compliance: Fix permissions for random UID (Group 0)
USER root
COPY --chown=101:0 apps/trader-app/docker-entrypoint.sh /docker-entrypoint.d/40-runtime-env.sh
RUN chmod +x /docker-entrypoint.d/40-runtime-env.sh && \
    chmod -R g+w /var/cache/nginx /var/run /var/log/nginx /etc/nginx/conf.d /usr/share/nginx/html
USER 101
EXPOSE 8080

# Use 127.0.0.1 (not localhost): busybox wget resolves localhost to ::1 first,
# but nginx listens on IPv4 only, which would fail the check.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health || exit 1

CMD ["nginx", "-g", "daemon off;"]

