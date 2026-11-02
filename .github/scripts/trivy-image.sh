#!/usr/bin/env bash
# Scans a container image with Trivy and applies the image CVE policy: fixable
# HIGH and CRITICAL findings are reported, and a fixable CRITICAL fails the gate.
# Findings with no fix yet are not counted (--ignore-unfixed). An accepted risk
# is listed in .trivyignore, read from the working directory, with an expiry
# date; a listed CVE is left out of the reports and the gate.
#
#   scan  Scans IMAGE_REF and writes trivy-NAME-PLATFORM.sarif for each platform,
#         plus a readable table to the log. PLATFORMS ("linux/amd64,linux/arm64")
#         lists the platforms to scan in the registry; empty scans the image in
#         the local Docker daemon, written as trivy-NAME-local.sarif. Findings
#         never fail it. The platforms it could not scan go to GITHUB_OUTPUT as
#         failed_platforms.
#   gate  Summarises the SARIF files `scan` wrote. Fails on a fixable CRITICAL,
#         on a platform `scan` could not scan (UNSCANNED, from failed_platforms),
#         and when nothing was scanned.
#
# Usage: trivy-image.sh scan IMAGE_REF NAME
#        trivy-image.sh gate NAME
set -euo pipefail

SEVERITIES=HIGH,CRITICAL

scan() {
  local ref="$1" name="$2" failed="" platform slug
  local -a platforms source

  if [ -n "${PLATFORMS:-}" ]; then
    IFS=',' read -ra platforms <<< "$PLATFORMS"
  else
    platforms=("")
  fi

  for platform in "${platforms[@]}"; do
    if [ -n "$platform" ]; then
      slug="${platform//\//-}"
      # `trivy image` resolves a manifest list to ONE entry, so each platform is
      # scanned explicitly or arm64 goes unscanned. --platform needs the full
      # os/arch form; a bare `arm64` parses as an OS name and matches nothing.
      # --image-src remote is load-bearing: the default `docker` source has
      # priority and silently IGNORES --platform, which would turn both
      # per-platform scans into two scans of the same arch.
      source=(--image-src remote --platform "$platform")
    else
      slug=local
      source=(--image-src docker)
    fi

    echo "::group::Trivy ${name} ${platform:-local image}"
    if trivy image "${source[@]}" \
         --severity "$SEVERITIES" \
         --ignore-unfixed \
         --format sarif \
         --output "trivy-${name}-${slug}.sarif" \
         --exit-code 0 \
         "$ref"; then
      # Same scan again as a readable table; caches are warm, so it is cheap.
      trivy image "${source[@]}" \
        --severity "$SEVERITIES" \
        --ignore-unfixed \
        --format table \
        --exit-code 0 \
        "$ref" || true
    else
      echo "::warning::Trivy could not scan ${platform:-the local image}."
      failed="${failed} ${platform:-local}"
    fi
    echo "::endgroup::"
  done

  echo "failed_platforms=${failed# }" >> "${GITHUB_OUTPUT:-/dev/null}"
}

gate() {
  local name="$1" unscanned="${UNSCANNED:-}" scanned=0 critical=0 high=0
  local file counts results file_critical
  local summary="${GITHUB_STEP_SUMMARY:-/dev/null}"

  {
    echo "### ${name} — image CVEs"
    echo ""
  } >> "$summary"

  if [ -n "$unscanned" ]; then
    echo "::error::Trivy could not scan ${name} (${unscanned}); an unscanned image fails the gate."
    {
      echo "> **Not scanned:** \`${unscanned}\`"
      echo ""
    } >> "$summary"
  fi

  # `[ -e ]` guards the no-match case: an unexpanded glob would otherwise reach
  # jq as a literal filename.
  for file in trivy-"${name}"-*.sarif; do
    [ -e "$file" ] || continue
    scanned=$((scanned + 1))
    # Each result names its rule by CVE id; the rule's tags carry the severity.
    # Prints "<results> <results tagged CRITICAL>".
    counts=$(jq -r '
      [ .runs[] | (.tool.driver.rules // []) as $rules
        | .results[]? | .ruleId as $id
        | [ $rules[] | select(.id == $id) | .properties.tags[]? ] ]
      | "\(length) \(map(select(index("CRITICAL"))) | length)"' "$file")
    results="${counts% *}"
    file_critical="${counts#* }"
    critical=$((critical + file_critical))
    high=$((high + results - file_critical))
    echo "${file}: ${results} fixable HIGH/CRITICAL finding(s), ${file_critical} CRITICAL"
    echo "- \`${file}\`: ${results} fixable HIGH/CRITICAL, ${file_critical} CRITICAL" >> "$summary"
  done

  echo "" >> "$summary"
  if [ "$scanned" -eq 0 ]; then
    echo "::error::No Trivy results for ${name}; nothing was scanned."
    echo "Nothing was scanned." >> "$summary"
    return 1
  fi
  if [ "$critical" -gt 0 ]; then
    echo "::error::${critical} fixable CRITICAL CVE(s) in ${name}. Fix the image, or list each CVE in .trivyignore with a reason and an expiry date if the risk is accepted."
    echo "**Fails the gate:** ${critical} fixable CRITICAL finding(s). List a CVE in \`.trivyignore\` with an expiry date to accept the risk." >> "$summary"
    return 1
  fi
  if [ -n "$unscanned" ]; then
    echo "**Fails the gate:** a platform was not scanned." >> "$summary"
    return 1
  fi
  if [ "$high" -gt 0 ]; then
    echo "::warning::${high} fixable HIGH CVE(s) in ${name}; reported, not blocking."
    echo "${high} fixable HIGH finding(s); reported, not blocking." >> "$summary"
  else
    echo "No fixable HIGH/CRITICAL CVEs." >> "$summary"
  fi
}

case "${1:-}" in
  scan) scan "${2:?usage: trivy-image.sh scan IMAGE_REF NAME}" "${3:?usage: trivy-image.sh scan IMAGE_REF NAME}" ;;
  gate) gate "${2:?usage: trivy-image.sh gate NAME}" ;;
  *) echo "usage: trivy-image.sh scan IMAGE_REF NAME | gate NAME" >&2; exit 2 ;;
esac
