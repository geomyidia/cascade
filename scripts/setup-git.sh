#!/usr/bin/env bash
# One-time local git setup for cascade contributors.
#
# Idempotent — safe to run multiple times. Applies:
#   - Reflog longevity extension (90d unreachable / 365d reachable)
#   - core.hooksPath pointing at scripts/hooks/ for version-controlled hooks
#   - Executable bit on the pre-push hook (defensive)
#
# Run after cloning, or after CLAUDE.md adds new local-state requirements:
#
#   make setup-git       # invokes this script
#   bash scripts/setup-git.sh    # equivalent direct invocation
#
# Background: M2 retro and a near-miss in M3 surfaced that destructive
# git operations on main can silently destroy work despite the methodology
# guardrails. This script applies the technical backstops; the protocol
# guardrail lives in CLAUDE.md's "Git Safety Protocol" section.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Reflog longevity. Defaults are 30d unreachable / 90d reachable; we
# extend both to give a wider recovery window for git mistakes.
git config --local gc.reflogExpire 365.days
git config --local gc.reflogExpireUnreachable 90.days

# Use version-controlled hooks. Hooks live at scripts/hooks/.
git config --local core.hooksPath scripts/hooks

# Defensive chmod (handles cases where git's mode bits aren't honoured —
# e.g. some Windows toolchains, or a fresh clone on a filesystem that
# strips +x). Silently no-op if the hook isn't there yet.
[ -f scripts/hooks/pre-push ] && chmod +x scripts/hooks/pre-push

echo "✓ cascade local git setup applied"
echo "  - reflog longevity: 365d reachable / 90d unreachable"
echo "  - core.hooksPath:   scripts/hooks"
echo
echo "Re-run anytime; the script is idempotent."
