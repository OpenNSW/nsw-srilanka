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
# Source services built from this repo (TNSW and the CDA and Customs agencies);
# `make deps` starts everything else.
APP_SERVICES    := api trader-portal cda-api cda-portal customs-api customs-portal
# Newline for turning `docker compose config --services` output into a word list.
define NL


endef
# Lazy (=) so `docker compose config` runs only when `make deps` is invoked.
ALL_SERVICES  = $(subst $(NL), ,$(shell $(COMPOSE) config --services))
DEPS_SERVICES = $(filter-out $(APP_SERVICES),$(ALL_SERVICES))
# Migrator version for `make migration`, read straight out of the Dockerfile's
# ARG so the two cannot drift apart. Lazy (=, not :=) so the sed runs only when
# `make migration` expands it, not on every make invocation.
MIGRATE_VERSION = $(shell sed -n 's/^ARG MIGRATE_VERSION=//p' Dockerfile)

.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Development (hot reload: air for Go, Vite HMR for the portal)
# ---------------------------------------------------------------------------

.PHONY: dev
dev: export APP_ENV = development
dev: ## Start the full stack with hot reload (detached; use `make logs` to watch)
	$(COMPOSE) up -d

.PHONY: logs
logs: ## Tail logs from all running services
	$(COMPOSE) logs -f

# ---------------------------------------------------------------------------
# Preview (build and run the real images from the Dockerfiles)
# ---------------------------------------------------------------------------

.PHONY: preview
preview: export APP_ENV = development
preview: ## Build and run the real images locally (detached; use `make logs` to watch)
	$(COMPOSE_PREVIEW) up --build -d

.PHONY: build
build: ## Build the images without starting anything
	$(COMPOSE_PREVIEW) build

# ---------------------------------------------------------------------------
# Native development (run the Go API on the host, e.g. for go.work cross-repo)
# ---------------------------------------------------------------------------

.PHONY: deps
deps: ## Start everything EXCEPT the apps (api, trader-portal, cda-api, cda-portal, customs-api, customs-portal)
	$(COMPOSE) up -d $(DEPS_SERVICES)

.PHONY: test-e2e
test-e2e: export APP_ENV = development
test-e2e: export E2E = 1
test-e2e: export GOWORK = off
test-e2e: ## Run in-process replay E2E tests (needs `make deps`; stops the api container)
	$(COMPOSE) stop api
	@if [ -f .env ]; then \
		set -a; . ./.env; set +a; \
	else \
		echo "⚠️  No .env found — using the current environment"; \
	fi; \
	go test -v -count=1 -timeout 240s ./test/e2e/...

# ---------------------------------------------------------------------------
# Migrations (uses the OpenNSW/agency migrate tool; generate needs no database)
# ---------------------------------------------------------------------------

.PHONY: migration
migration: export GOWORK = off
# generate touches no database: an empty config leaves the migrator on its
# defaults (sqlite, unused; migrationDir ./migrations).
migration: export CONFIG_PATH = /dev/null
migration: ## Scaffold a new migration file: make migration name=<description>
	@test -n "$(name)" || { echo "Usage: make migration name=<description>  (e.g. make migration name=add_users_table)"; exit 1; }
	@go run github.com/OpenNSW/agency/backend/cmd/migrate@$(MIGRATE_VERSION) generate $(name)

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

# cmd.exe only: Windows_NT with MSYSTEM unset. Git Bash / MSYS set MSYSTEM
# and keep Unix recipes. cmd's find is FIND.EXE, so it cannot walk files, and
# it has no grep or awk. Defined above `help` because make picks the recipe
# when it reads the Makefile.
ifeq ($(OS),Windows_NT)
ifeq ($(MSYSTEM),)
  USE_CMD := 1
endif
endif

.PHONY: help
help: ## Show this help
ifdef USE_CMD
	@powershell -NoProfile -Command "Get-Content '$(firstword $(MAKEFILE_LIST))' | ForEach-Object { if ($$_ -match '^([a-zA-Z0-9_-]+):.*?## (.*)$$') { '  {0,-14} {1}' -f $$Matches[1], $$Matches[2] } }"
else
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
endif

# ---------------------------------------------------------------------------
# Go code quality (mirrors the backend CI pipeline)
# Prepend GOPATH/bin so tools installed by `make tools` are found without
# requiring the developer to manually update their shell profile.
# cmd.exe uses ';'. Git Bash / MSYS set MSYSTEM and keep ':'.
# ---------------------------------------------------------------------------

ifdef USE_CMD
  export PATH := $(shell go env GOPATH)/bin;$(PATH)
else ifneq ($(MSYSTEM),)
  # go env GOPATH is C:\... here. cygpath makes /c/... so the drive colon is not a PATH separator.
  export PATH := $(shell cygpath -u "$$(go env GOPATH)")/bin:$(PATH)
else
  export PATH := $(shell go env GOPATH)/bin:$(PATH)
endif

.PHONY: setup
setup: tools ## First-time setup: install tools, configure git hooks, seed config files from examples
	git config core.hooksPath .githooks
ifdef USE_CMD
	@echo Git hooks configured: .githooks/
	@if exist .env.example if not exist .env copy /Y .env.example .env
	@if exist idp\.env.example if not exist idp\.env copy /Y idp\.env.example idp\.env
	@if exist portals\apps\trader-app\public\config.example.js if not exist portals\apps\trader-app\public\config.js copy /Y portals\apps\trader-app\public\config.example.js portals\apps\trader-app\public\config.js
	@if exist configs\services.example.json if not exist configs\services.json copy /Y configs\services.example.json configs\services.json
	@if exist configs\services.docker.example.json if not exist configs\services.docker.json copy /Y configs\services.docker.example.json configs\services.docker.json
	@if exist configs\payment_methods.example.json if not exist configs\payment_methods.json copy /Y configs\payment_methods.example.json configs\payment_methods.json
	@if exist configs\catalog.example.json if not exist configs\catalog.json copy /Y configs\catalog.example.json configs\catalog.json
	@if exist configs\companies.example.json if not exist configs\companies.json copy /Y configs\companies.example.json configs\companies.json
	@if exist configs\config.example.yaml if not exist configs\config.yaml copy /Y configs\config.example.yaml configs\config.yaml
	@if exist configs\config.docker.example.yaml if not exist configs\config.docker.yaml copy /Y configs\config.docker.example.yaml configs\config.docker.yaml
else
	chmod +x .githooks/pre-commit .githooks/pre-push
	@echo "  Git hooks configured: .githooks/"
	@for f in .env.example idp/.env.example portals/apps/trader-app/public/config.example.js; do \
		target=$$(echo $$f | sed 's/\.example//'); \
		if [ ! -f "$$f" ]; then echo "  Skipped: $$target ($$f not found)"; \
		elif [ ! -f "$$target" ]; then cp "$$f" "$$target" && echo "  Created: $$target"; \
		else echo "  Skipped: $$target (already exists)"; fi; \
	done
	@for f in configs/services.example.json configs/services.docker.example.json configs/payment_methods.example.json configs/catalog.example.json configs/companies.example.json configs/config.example.yaml configs/config.docker.example.yaml; do \
		target=$$(echo $$f | sed 's/\.example\././'); \
		if [ ! -f "$$f" ]; then echo "  Skipped: $$target ($$f not found)"; \
		elif [ ! -f "$$target" ]; then cp "$$f" "$$target" && echo "  Created: $$target"; \
		else echo "  Skipped: $$target (already exists)"; fi; \
	done
endif

.PHONY: tools
tools: ## Install Go quality tools (gosec, govulncheck, gitleaks; golangci-lint must be v2 — see CONTRIBUTING.md)
	@echo Installing Go quality tools...
ifdef USE_CMD
	@where golangci-lint >nul 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@golangci-lint --version | findstr /C:"version 1." /C:"version v1." >nul && (echo ERROR: golangci-lint v1 is not supported. Install v2. && exit 1) || ver >nul
else
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint --version | grep -Eqv 'has version v?1\.' \
		|| { echo "ERROR: golangci-lint v2 is required. Install via Homebrew: brew install golangci-lint"; exit 1; }
endif
	go install github.com/securego/gosec/v2/cmd/gosec@v2.27.1
	go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
	go install github.com/zricethezav/gitleaks/v8@v8.30.1
	@echo Tools installed.

.PHONY: fmt
fmt: ## Format all Go source files with gofmt
ifdef USE_CMD
	powershell -NoProfile -Command "Get-ChildItem -Recurse -Filter *.go | Where-Object { $$_.FullName -notlike '*\vendor\*' } | ForEach-Object { gofmt -w $$_.FullName; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE } }"
else
	gofmt -w $$(find . -name '*.go' -not -path '*/vendor/*')
endif

.PHONY: lint
lint: export GOWORK = off
lint: ## Run golangci-lint
	golangci-lint run --config .golangci.yml ./...

.PHONY: tidy
tidy: export GOWORK = off
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: test
test: export GOWORK = off
test: ## Run all tests with the race detector
	go test -race -count=1 ./...

.PHONY: vuln
vuln: export GOWORK = off
vuln: ## Run govulncheck against the Go vulnerability database
	govulncheck ./...

.PHONY: secrets
secrets: ## Run gitleaks secret scan on the repository
	gitleaks detect --config .gitleaks.toml --verbose

.PHONY: check
check: tidy fmt lint test ## Run all quality checks: tidy → fmt → lint → test

# ---------------------------------------------------------------------------
# Docs quality (mirrors .github/workflows/docs-ci.yml)
# markdownlint runs through npx (Node.js); lychee and Vale run in Docker.
# ---------------------------------------------------------------------------

# The pre-commit hook pins the same markdownlint-cli2 version.
MDLINT_VERSION := 0.23.2
# markdownlint --fix cannot pad tables, so mdlint-fix runs Prettier for them.
PRETTIER_VERSION := 3.9.9
LYCHEE_IMAGE   := lycheeverse/lychee:0.24.2
VALE_IMAGE     := jdkato/vale:v3.24.0
# Lazy (=) so git runs only when a target that needs the file list is invoked.
MD_FILES = $(shell git ls-files "*.md")

.PHONY: mdlint
mdlint: ## Lint Markdown files (rules: .markdownlint-cli2.jsonc)
	npx --yes markdownlint-cli2@$(MDLINT_VERSION)

.PHONY: mdlint-fix
mdlint-fix: ## Apply markdownlint's automatic fixes and align tables with Prettier
	npx --yes markdownlint-cli2@$(MDLINT_VERSION) --fix || true
	npx --yes prettier@$(PRETTIER_VERSION) --prose-wrap preserve --embedded-language-formatting off --write $(MD_FILES)
	npx --yes markdownlint-cli2@$(MDLINT_VERSION)

.PHONY: linkcheck
linkcheck: ## Check links and #anchors in Markdown files with lychee (needs Docker)
	docker run --rm -e GITHUB_TOKEN -v "$(CURDIR):/input" -w /input $(LYCHEE_IMAGE) --no-progress $(MD_FILES)

.PHONY: prose
prose: ## Check Markdown spelling and term casing with Vale (needs Docker)
	docker run --rm -v "$(CURDIR):/docs" -w /docs $(VALE_IMAGE) $(MD_FILES)

.PHONY: docs-check
docs-check: mdlint linkcheck prose ## Run all docs checks: mdlint → linkcheck → prose
