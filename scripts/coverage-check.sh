#!/usr/bin/env bash
set -euo pipefail

# Per-package coverage gate for cascade.
#
# Reads coverage.out (produced by `go test -coverprofile=coverage.out ./...`)
# and verifies each package meets its individual threshold.
#
# Why per-package and not a single overall number? A 100% pure-core package
# can have its score diluted by an io shell at, say, 60%, and the overall
# might still pass at 90% while leaving real holes in the algorithmic core.
#
# Threshold policy (parallel arrays — bash 3.2 compatible, so this works on
# macOS's stock /bin/bash without forcing contributors to brew install bash):
#
#   - golist/      100  (M2 will hit this; pre-M2 the package is empty so 0/0=N/A)
#   - depgraph/    100  (M3)
#   - changeset/   100  (M4)
#   - cmd/cascade  no gate (io boundary; behavior tested by integration tests)
#
# When a future milestone adds a new public package with implementation,
# add a matching pair of entries to PACKAGES and THRESHOLDS at the same
# index. That's the structural counterpart to "decide coverage policy
# explicitly per package."

PROFILE="${1:-coverage.out}"

if [[ ! -f "$PROFILE" ]]; then
    echo "::error::coverage profile not found: $PROFILE"
    exit 1
fi

PACKAGES=(
    "github.com/geomyidia/cascade/golist"
    "github.com/geomyidia/cascade/depgraph"
    "github.com/geomyidia/cascade/changeset"
    "github.com/geomyidia/cascade/project"
)
THRESHOLDS=(100 100 100 100)

if [[ "${#PACKAGES[@]}" -ne "${#THRESHOLDS[@]}" ]]; then
    echo "::error::PACKAGES and THRESHOLDS arrays differ in length"
    exit 1
fi

failed=0
for i in "${!PACKAGES[@]}"; do
    pkg="${PACKAGES[$i]}"
    threshold="${THRESHOLDS[$i]}"

    # Empty packages report no coverage line; skip them gracefully so this
    # script doesn't fail before M2/M3/M4 land their implementations.
    pct="$(go tool cover -func="$PROFILE" \
        | awk -v p="$pkg/" '$1 ~ "^"p {gsub(/%/,"",$NF); sum+=$NF; count++} END {if (count) print sum/count; else print "N/A"}')"

    if [[ "$pct" == "N/A" ]]; then
        echo "::notice::$pkg has no coverage data yet (likely empty pre-implementation)"
        continue
    fi

    if awk -v a="$pct" -v t="$threshold" 'BEGIN {exit !(a < t)}'; then
        echo "::error::$pkg coverage $pct% < required $threshold%"
        failed=1
    else
        echo "ok: $pkg coverage $pct% >= $threshold%"
    fi
done

exit "$failed"
