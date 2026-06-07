# Portals Workspace

A pnpm monorepo containing the NSW portal applications built with React and Radix UI.

> **📦 Using pnpm** - Faster installs, better disk usage, single lock file for the entire monorepo

## Quick Start

```bash
# First time setup
make setup      # Installs pnpm (if needed) + all dependencies

# Start developing
make dev-trader # Start Trader app

# Quality checks & formatting
make lint       # Check for lint errors
make format     # Auto-fix lint and formatting issues
make type-check # Run TypeScript type checking

make help       # See all available commands
```

---

## Project Structure

```
portals/
├── Makefile               # Team development commands
├── pnpm-workspace.yaml    # pnpm workspace configuration
├── pnpm-lock.yaml         # Single lock file for entire monorepo
├── package.json           # Root workspace configuration
├── tsconfig.json          # Shared TypeScript configuration
└── apps/                  # Applications
    └── trader-app/        # Trader portal application
```

## Apps

The `apps/` directory contains the portal applications. Each app is a standalone
project. Shared JSON Forms renderers are consumed from the published
`@opennsw/jsonforms-renderers` npm package.

**Current apps:**

- `trader-app` - Trader portal application

---

## Development Workflow

### Common Commands

```bash
# Development
make dev-trader     # Start Trader app

# Building
make build          # Build all workspaces
make build-trader   # Build Trader app only

# Code quality
make lint           # Run linter
make format         # Auto-fix linting issues
```

### Adding Dependencies

```bash
# To a specific app
pnpm --filter trader-app add axios

# To workspace root (dev dependencies)
pnpm add -w prettier -D
```

---

## Tech Stack

- **React** 19
- **Radix UI** - Unstyled, accessible component primitives
- **TypeScript** - Type safety
- **Vite** - Build tooling
- **pnpm** - Fast, efficient package manager

## Why pnpm?

- ⚡ **2x faster** than npm
- 💾 **30-50% less disk space** via content-addressable storage
- 🔒 **Stricter** - prevents phantom dependencies
- 🎯 **Single lock file** - better for monorepos
- ✅ **Industry standard** - used by Vue, Vite, Svelte, and more
