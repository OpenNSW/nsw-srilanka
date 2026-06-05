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