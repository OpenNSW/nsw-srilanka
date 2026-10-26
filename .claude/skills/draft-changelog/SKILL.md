---
name: draft-changelog
description: Draft the CHANGELOG.md section for the next TNSW release (vX.Y.Z), written for the people who deploy it — upgrade steps first, then what was added, changed and fixed — from the pull requests merged since the last release, their "Deployment Notes", and the changes to migrations, env vars, config, IdP scopes and the Helm chart. Use it whenever someone is preparing a release or asks for release notes, a changelog entry, what changed since the last release, or what deployers need to know for the next version, even if they don't say "changelog". It edits CHANGELOG.md and version.txt only; it never tags, pushes, merges or publishes.
---

# Draft Changelog

Read and follow [the shared draft-changelog skill](../../../.agents/skills/draft-changelog/SKILL.md) at `.agents/skills/draft-changelog/SKILL.md` before performing this task.

The shared skill is the source of truth. Resolve its repository paths from the repository root, and use its supporting resources. Maintain workflow instructions in that file; keep this Claude entry point as a pointer.
