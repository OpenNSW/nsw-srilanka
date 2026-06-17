# NSW Sri Lanka — container stack commands.
#
# Two modes:
#   dev     = compose.yml + compose.override.yml (auto-merged) -> hot reload
#   preview = compose.yml ONLY                                 -> real built images
#
# `make` with no target prints this help.

# compose.override.yml auto-loads, so plain `docker compose` == dev.
COMPOSE         := docker compose
# Pass only the base file to exclude the override == the real built images.
COMPOSE_PREVIEW := docker compose -f compose.yml
# Source services built from this repo; `make deps` starts everything else.
APP_SERVICES    := api trader-portal
# A literal space, so APP_SERVICES can be turned into a grep alternation.
SPACE           := $(subst ,, )
# Migrator version for `make migration` — KEEP IN SYNC with the
# MIGRATE_VERSION build arg in the Dockerfile.
MIGRATE_VERSION := v0.0.0-20260610120959-d981e67a7a47

.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Development (hot reload: air for Go, Vite HMR for the portal)
# ---------------------------------------------------------------------------

.PHONY: dev
dev: ## Start the full stack with hot reload (detached; use `make logs` to watch)
	$(COMPOSE) up -d

.PHONY: logs
logs: ## Tail logs from all running services
	$(COMPOSE) logs -f

# ---------------------------------------------------------------------------
# Preview (build and run the real images from the Dockerfiles)
# ---------------------------------------------------------------------------

.PHONY: preview
preview: ## Build and run the real images locally (detached; use `make logs` to watch)
	$(COMPOSE_PREVIEW) up --build -d

.PHONY: build
build: ## Build the images without starting anything
	$(COMPOSE_PREVIEW) build

# ---------------------------------------------------------------------------
# Native development (run the Go API on the host, e.g. for go.work cross-repo)
# ---------------------------------------------------------------------------

.PHONY: deps
deps: ## Start everything EXCEPT api & trader-portal (run those natively yourself)
	$(COMPOSE) up -d $$($(COMPOSE) config --services | grep -vxE '$(subst $(SPACE),|,$(APP_SERVICES))')

.PHONY: test-e2e
test-e2e: ## Run in-process replay E2E tests (needs `make deps`; stops the api container)
	$(COMPOSE) stop api
	@if [ -f .env ]; then \
		set -a; . ./.env; set +a; \
	else \
		echo "⚠️  No .env found — using the current environment"; \
	fi; \
	E2E=1 go test -v -count=1 -timeout 240s ./integration/...

# ---------------------------------------------------------------------------
# Migrations (uses the nsw-agency migrate tool; generate needs no database)
# ---------------------------------------------------------------------------

.PHONY: migration
migration: ## Scaffold a new migration file: make migration name=<description>
	@test -n "$(name)" || { echo "Usage: make migration name=<description>  (e.g. make migration name=add_users_table)"; exit 1; }
	@GOWORK=off MIGRATION_DIR=./migrations DB_DRIVER=sqlite \
		go run github.com/OpenNSW/nsw-agency/backend/cmd/migrate@$(MIGRATE_VERSION) generate $(name)

# ---------------------------------------------------------------------------
# Lifecycle
# ---------------------------------------------------------------------------

.PHONY: down
down: ## Stop and remove containers (keeps volumes/data)
	$(COMPOSE) down

.PHONY: clean
clean: ## Stop and remove containers AND named volumes (wipes db/bucket data)
	$(COMPOSE) down -v

.PHONY: ps
ps: ## Show the status of the stack's containers
	$(COMPOSE) ps

.PHONY: config
config: ## Print the merged dev config (for debugging)
	$(COMPOSE) config

# ---------------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'