.PHONY: help setup install clean dev-trader build build-trader lint format type-check preview-trader

# Default target
help:
	@echo "📦 NSW Portals - Development Commands"
	@echo ""
	@echo "Setup & Installation:"
	@echo "  make setup          - Install pnpm globally and setup workspace"
	@echo "  make install        - Install all dependencies"
	@echo "  make clean          - Remove node_modules and lock files"
	@echo ""
	@echo "Development:"
	@echo "  make dev-trader     - Run Trader app in dev mode"
	@echo ""
	@echo "Build:"
	@echo "  make build          - Build all workspaces"
	@echo "  make build-trader   - Build Trader app only"
	@echo ""
	@echo "Quality:"
	@echo "  make lint           - Run ESLint on all workspaces"
	@echo "  make format         - Auto-fix lint and formatting issues"
	@echo "  make type-check     - Run TypeScript type checking"
	@echo ""

# Setup pnpm using the existing install first, then corepack as a fallback.
setup:
	@echo "🔧 Setting up development environment..."
	@if command -v pnpm >/dev/null 2>&1; then \
		echo "✅ pnpm already available"; \
	elif command -v corepack >/dev/null 2>&1; then \
		corepack enable pnpm; \
		echo "✅ pnpm available via corepack"; \
	else \
		echo "❌ pnpm and corepack not found — install Node.js 22.18.0+ or pnpm 11.1.2"; \
		exit 1; \
	fi
	@pnpm --version
	@echo "✅ pnpm available"
	@$(MAKE) install
	@echo "✅ Setup complete!"

# Install dependencies
install:
	@echo "📦 Installing dependencies with pnpm..."
	@pnpm install

# Clean all node_modules and lock files
clean:
	@echo "🧹 Cleaning workspace..."
	@rm -rf node_modules
	@rm -rf apps/*/node_modules
	@rm -rf apps/*/dist
	@rm -rf apps/*/.turbo
	@echo "✅ Cleaned!"

# Development servers
dev-trader:
	@echo "🚀 Starting Trader app..."
	@pnpm --filter trader-app dev

# Build commands
build:
	@echo "🏗️  Building all workspaces..."
	@pnpm -r build

build-trader:
	@echo "🏗️  Building Trader app..."
	@pnpm --filter trader-app build

# Linting and Formatting
lint:
	@echo "🔍 Running ESLint..."
	@pnpm run lint

format:
	@echo "🔧 Auto-fixing lint and formatting issues..."
	@pnpm run format
	@echo "✅ Formatting complete!"

type-check:
	@echo "📝 Running TypeScript type checking..."
	@pnpm run type-check

# Preview production builds
preview-trader:
	@echo "👀 Previewing Trader app..."
	@pnpm --filter trader-app preview