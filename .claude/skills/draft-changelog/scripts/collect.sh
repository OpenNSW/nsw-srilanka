#!/usr/bin/env bash
# Compatibility entry point; maintain the collector in .agents/skills/.
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
exec bash "$script_dir/../../../../.agents/skills/draft-changelog/scripts/collect.sh" "$@"
