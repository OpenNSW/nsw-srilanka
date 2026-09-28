#!/usr/bin/env bash
# Prints one version's section of CHANGELOG.md: the lines between
# "## [VERSION]" and the next "## " heading or the link references at the foot
# of the file, trimmed of blank lines at either end. Prints nothing when the
# section is missing or empty.
#
# Shared so a drafted section is checked exactly the way it will be released:
# release-notes.sh uses it for the GitHub Release body (release.yml), and the
# draft-changelog skill (.claude/skills) runs it on its draft.
#
# Usage: changelog-section.sh VERSION [FILE]    FILE defaults to CHANGELOG.md
set -euo pipefail

version="${1:?usage: changelog-section.sh VERSION [FILE]}"
file="${2:-CHANGELOG.md}"

# index() matches literally, so the dots in VERSION are not regex wildcards and
# "## [1.2.0-rc.1]" never matches "## [1.2.0]". Plain POSIX awk: the release
# runner's is mawk.
awk -v heading="## [${version}]" '
  index($0, heading) == 1 { found = 1; next }
  found && (/^## / || /^\[.+\]: /) { exit }
  found { lines[++n] = $0; if (NF) { if (!first) first = n; last = n } }
  END { for (i = first; i && i <= last; i++) print lines[i] }
' "$file"
