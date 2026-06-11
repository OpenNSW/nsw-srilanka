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
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Go code quality (mirrors the backend CI pipeline)
# Prepend GOPATH/bin so tools installed by `make tools` are found without
# requiring the developer to manually update their shell profile.
# ---------------------------------------------------------------------------

export PATH := $(shell go env GOPATH)/bin:$(PATH)

.PHONY: setup
setup: tools ## Install Go quality tools and configure git hooks
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit .githooks/pre-push
	@echo "  Git hooks configured: .githooks/"

.PHONY: tools
tools: ## Install Go quality tools (gosec, govulncheck, gitleaks; golangci-lint must be v2 — see CONTRIBUTING.md)
	@echo "Installing Go quality tools..."
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint --version | grep -qv "^golangci-lint has version v1" \
		|| { echo "ERROR: golangci-lint v2 is required. Install via Homebrew: brew install golangci-lint"; exit 1; }
	go install github.com/securego/gosec/v2/cmd/gosec@v2.27.1
	go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
	go install github.com/zricethezav/gitleaks/v8@v8.30.1
	@echo "Tools installed."

.PHONY: fmt
fmt: ## Format all Go source files with gofmt
	gofmt -w $$(find . -name '*.go' -not -path '*/vendor/*')

.PHONY: lint
lint: ## Run golangci-lint
	GOWORK=off golangci-lint run --config .golangci.yml ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	GOWORK=off go mod tidy

.PHONY: test
test: ## Run all tests with the race detector
	GOWORK=off go test -race -count=1 ./...

.PHONY: vuln
vuln: ## Run govulncheck against the Go vulnerability database
	GOWORK=off govulncheck ./...

.PHONY: secrets
secrets: ## Run gitleaks secret scan on the repository
	gitleaks detect --config .gitleaks.toml --verbose

.PHONY: check
check: tidy fmt lint test ## Run all quality checks: tidy → fmt → lint → test