---
number: 2
title: "M1: Repo Scaffold + CI Baseline"
author: "geomyidia project"
component: All
tags: [m1, scaffold, ci, github-actions, makefile, branch-protection, golangci-lint]
created: 2026-05-06
updated: 2026-05-06
state: Final
supersedes: null
superseded-by: null
version: 1.0
---

# M1: Repo Scaffold + CI Baseline

**Status:** draft. Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md). This is the implementation-ready spec for M1.

## Goal

Establish the repo, license, module structure, and CI gates so M2-M6 can land code without rebuilding plumbing each time. The exit signal is: a clean push to `main` runs CI green on the Go-version matrix, the coverage gate fires correctly when violated, and `go install github.com/geomyidia/cascade/cmd/cascade@<HEAD>` produces a runnable (if functionally empty) binary.

## Scope

### In scope (M1 ships these)

- Module declaration with the correct Go floor version
- Final directory layout (top-level public packages + `cmd/cascade/`)
- Placeholder `cmd/cascade/main.go` that builds, accepts `--version`/`--help`, and exits cleanly
- Empty `doc.go` files in `golist/`, `depgraph/`, `changeset/` to anchor the package boundaries before code lands in M2-M4
- `Makefile` (already in working tree — codify and confirm)
- `.github/workflows/ci.yml` with the Go-version matrix and gate steps
- `.golangci.yml` config
- Branch protection on `main`
- Repo-health files: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `.github/PULL_REQUEST_TEMPLATE.md`, `.github/ISSUE_TEMPLATE/`
- Coverage gate mechanism (per-package, scripted)

### Out of scope (deferred to later milestones)

- Anything that puts real code in `golist/`, `depgraph/`, or `changeset/` (M2/M3/M4)
- Real CLI flag handling, beyond `--version` and `--help` (M5)
- The release tag itself (M6 — `v0.1.0`)
- The consumer-side CI re-wiring (Ext milestone, in a separate repo)
- Fancy release tooling (GoReleaser, release-please, etc.) — manual `make release VERSION=v0.1.0` is enough for M6

## Deliverables

### 1. `go.mod`

```text
module github.com/geomyidia/cascade

go 1.25.3
```

The `go` directive declares the **minimum** Go version required to build the module. It is intentionally lower than any contributor's local toolchain. Local development can use 1.25.3, 1.26.x, or any later version — the directive sets a floor, not a ceiling. CI tests on both ends of the supported range (see §3).

**Toolchain directive:** intentionally omitted for now. Adding `toolchain go1.26.2` would tell `go` to download a specific toolchain when building; for an OSS tool consumed via `go install`, that's friction we don't want until there's a concrete reason. Re-evaluate at M6.

**Currently in the working tree:** `go.mod` is staged with `go 1.26.2`. Needs to drop to `1.25.3` before this milestone is complete (see Open Questions §1).

### 2. Directory layout

```text
cascade/
├── .github/
│   ├── workflows/
│   │   └── ci.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md
│   │   └── feature_request.md
│   └── PULL_REQUEST_TEMPLATE.md
├── assets/
│   ├── ai/
│   │   ├── CLAUDE-CODE-COVERAGE.md     (already present)
│   │   └── go/                          (symlink, already present)
│   └── images/
│       └── logo-v1-large.png            (already staged)
├── cmd/
│   └── cascade/
│       └── main.go                      (placeholder for M1; real wiring in M5)
├── golist/                              (M1 stub; M2 fills in)
│   └── doc.go
├── depgraph/                            (M1 stub; M3 fills in)
│   └── doc.go
├── changeset/                           (M1 stub; M4 fills in)
│   └── doc.go
├── docs/
│   └── design/
│       └── 01-draft/
│           ├── 0001-cascade-high-level-project-plan.md
│           └── 0002-m1-repo-scaffold-and-ci-baseline.md   (this file)
├── scripts/
│   └── coverage-check.sh                (per-package coverage gate, see §7)
├── .gitignore                           (already present)
├── .golangci.yml                        (NEW in M1)
├── CODE_OF_CONDUCT.md                   (NEW in M1)
├── CONTRIBUTING.md                      (NEW in M1)
├── LICENSE                              (Apache-2.0, already present)
├── Makefile                             (already in working tree; codify here)
├── README.md                            (already present; expanded in M5/M6)
├── SECURITY.md                          (NEW in M1)
├── go.mod
└── odm.toml                             (already present)
```

`internal/` is intentionally absent from the M1 layout. It will appear in M5 if any CLI-glue code emerges that genuinely shouldn't be public; otherwise the project ships with no `internal/` packages.

### 3. `cmd/cascade/main.go` (placeholder)

A minimal `main.go` that:

- Builds cleanly under both Go versions in the CI matrix
- Accepts `--help` and `--version` (and rejects everything else with a clear error)
- Reads version metadata from `ldflags`-injected vars (the Makefile already injects these)
- Exits 0 on `--help` / `--version`, 1 on flag-parse error, and prints to stdout (not stderr) for the success cases

```go
// Command cascade computes the reverse-transitive closure of a Go change-set
// under the imports relation. See https://github.com/geomyidia/cascade.
package main

import (
 "flag"
 "fmt"
 "os"
)

// These are populated at build time via -ldflags. See Makefile for the
// canonical injection. Defaults make `go run` and `go install` (without the
// Makefile) still work.
var (
 Version   = "dev"
 GitCommit = "unknown"
 GitBranch = "unknown"
 BuildTime = "unknown"
)

func main() {
 fs := flag.NewFlagSet("cascade", flag.ContinueOnError)
 fs.SetOutput(os.Stderr)

 showVersion := fs.Bool("version", false, "print version information and exit")
 // --help is handled implicitly by flag.ContinueOnError + the default usage func

 if err := fs.Parse(os.Args[1:]); err != nil {
  // flag prints its own error; just exit non-zero.
  os.Exit(1)
 }

 if *showVersion {
  fmt.Printf("cascade %s (commit %s, branch %s, built %s)\n",
   Version, GitCommit, GitBranch, BuildTime)
  return
 }

 // M5 will replace this with the real pipeline. M1 placeholder behavior:
 // running with no flags prints a one-line "not yet implemented" message
 // and exits non-zero so nobody mistakes it for a working install.
 fmt.Fprintln(os.Stderr, "cascade: not yet implemented (this is the M1 placeholder)")
 os.Exit(2)
}
```

**Tests for the placeholder:** a `cmd/cascade/main_test.go` that exercises `--version` via `os/exec` against the freshly-built binary. This single integration test exists primarily to prove the build pipeline + version-injection chain works end-to-end. It can be skipped under `testing.Short()`. Coverage of `cmd/cascade/` is **not** gated (it's the io boundary).

### 4. Stub packages: `golist/`, `depgraph/`, `changeset/`

Each gets a `doc.go` with a package-level comment describing its eventual role. This (a) reserves the directory, (b) lets `go build ./...` pass cleanly, (c) gives Go's pkgsite/godoc something useful to render from day one, and (d) anchors the public-API commitment with documented intent before M2-M4 fill in implementations.

```go
// Package golist is a thin io shell around `go list -deps -json`.
// It exposes typed Package values parsed from `go list`'s JSON output,
// without going through golang.org/x/tools/go/packages.
//
// This package is the only part of cascade that shells out to `go`.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package golist
```

```go
// Package depgraph builds a directed import graph from a slice of
// golist.Package values and exposes reverse-transitive closure traversal.
//
// All operations are pure — no io, no syscalls. Tests are table-driven
// over synthetic graphs.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package depgraph
```

```go
// Package changeset maps a list of changed file paths into the set of
// import paths whose packages those files belong to.
//
// All operations are pure — no io. Tests are table-driven.
//
// API stability: pre-v1.0, package surface may change. See repo README.
package changeset
```

### 5. `Makefile` (already in working tree)

The Makefile already covers: `build`, `build-release`, `install`, `test`, `lint`, `format`, `vet`, `coverage`, `coverage-html`, `coverage-check`, `tidy`, `deps`, `check-deps`, `docs`, `clean`, `clean-all`, `check`, `check-all`, `release-dry-run`, `release`, `info`, `check-tools`, `tracked-files`, `remotes`, `push`. Multi-remote push (macpro/github/codeberg) and ldflags-driven version injection are wired.

**One change for M1:** `COVERAGE_THRESHOLD := 90` is the project-wide overall floor, kept conservative so unfinished work doesn't immediately bounce. CI does the strict per-package gating (see §7). When the project plan's "100% on non-main/non-io" intent is fully achievable post-M4, raise the Makefile floor to match.

**No changes for M1 to Makefile structure.** Just confirm the file by adding it to git (currently untracked in working tree).

### 6. CI workflow — `.github/workflows/ci.yml`

```yaml
name: CI

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

# Cancel in-progress runs of the same ref when a new commit lands.
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

jobs:
  test:
    name: test (Go ${{ matrix.go }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        # Floor (1.25.3) and latest currently-supported major (1.26.x).
        # 1.24 and 1.23 are out of Go's two-newest-major support window.
        go: ["1.25.3", "1.26.x"]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          check-latest: true
          cache: true

      - name: Verify go.mod is tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

      - name: gofmt
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "::error::The following files are not gofmt-clean:"
            echo "$unformatted"
            exit 1
          fi

      - name: go vet
        run: go vet ./...

      - name: Test (with race detector)
        run: go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...

      - name: Coverage gate (per-package)
        run: ./scripts/coverage-check.sh

      - name: Upload coverage artifact
        if: matrix.go == '1.25.3'
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out
          retention-days: 14

  lint:
    name: lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.26.x"
          cache: true

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          args: --timeout=5m
```

**Notes on the workflow:**

- Two jobs: `test` (matrix-parallel) and `lint` (single-version). They run in parallel; both must pass for the branch-protection check to be satisfied.
- `concurrency` block cancels superseded runs so a rapid push sequence doesn't pile up.
- `permissions: contents: read` is the minimum read-only scope. We only escalate if a future job needs to (e.g. uploading test results to a third-party).
- Coverage profile is uploaded once per CI run (only on the floor Go version) — uploading both produces redundant artifacts.
- Race detector is on by default (`-race`); it's cheap enough for a project this size and catches real bugs.
- The `go mod tidy` check fails the build if anyone forgets to tidy; this prevents the long-tail "but it works on my machine" go.sum drift.

### 7. Coverage gate — `scripts/coverage-check.sh`

The gate enforces the project plan's "100% on non-main/non-io packages" intent by per-package thresholds rather than a single overall number. Single-overall is too coarse: a 100% pure-core package can have its score diluted by an io shell at, say, 60%, and the overall might still pass at 90% while leaving real holes in the algorithmic core.

```bash
#!/usr/bin/env bash
set -euo pipefail

# Per-package coverage gate for cascade.
#
# Reads coverage.out (produced by `go test -coverprofile=coverage.out ./...`)
# and verifies each package meets its individual threshold.
#
# Thresholds:
#   - golist/      100  (M2 will hit this; pre-M2 the package is empty so 0/0=N/A)
#   - depgraph/    100  (M3)
#   - changeset/   100  (M4)
#   - cmd/cascade  no gate (io boundary; behavior tested by integration tests)

PROFILE="${1:-coverage.out}"

if [[ ! -f "$PROFILE" ]]; then
    echo "::error::coverage profile not found: $PROFILE"
    exit 1
fi

declare -A THRESHOLDS=(
    ["github.com/geomyidia/cascade/golist"]=100
    ["github.com/geomyidia/cascade/depgraph"]=100
    ["github.com/geomyidia/cascade/changeset"]=100
)

failed=0
while read -r pkg threshold; do
    # Empty packages report no coverage line; skip them gracefully so this
    # script doesn't fail before M2/M3/M4 land their implementations.
    pct="$(go tool cover -func="$PROFILE" \
        | awk -v p="$pkg/" '$1 ~ "^"p {sum+=$NF; count++} END {if (count) print sum/count; else print "N/A"}')"

    if [[ "$pct" == "N/A" ]]; then
        echo "::notice::$pkg has no coverage data yet (likely empty pre-implementation)"
        continue
    fi

    # bash-only float compare via awk
    if awk -v a="$pct" -v t="$threshold" 'BEGIN {exit !(a < t)}'; then
        echo "::error::$pkg coverage $pct% < required $threshold%"
        failed=1
    else
        echo "ok: $pkg coverage $pct% >= $threshold%"
    fi
done < <(for k in "${!THRESHOLDS[@]}"; do echo "$k ${THRESHOLDS[$k]}"; done)

exit $failed
```

**Discipline this enforces:** every package listed in the threshold map must hit its number once it has any code. Empty packages are skipped so the gate doesn't fire prematurely between M1 and M2-M4. When a new public package gets added in some future milestone, it must also be added to the `THRESHOLDS` map — that's the structural counterpart to "decide coverage policy explicitly per package."

### 8. `.golangci.yml`

Minimal, opinionated config. Enable the high-signal linters; disable the noisy ones.

```yaml
version: "2"

run:
  timeout: 5m

linters:
  default: none
  enable:
    - errcheck       # catches dropped errors (the gta failure mode in lint form)
    - govet
    - ineffassign    # catches ineffective assignments
    - staticcheck    # SA-checks; high signal
    - unused         # dead-code detection
    - gosec          # security audit (we'll see how noisy this is; can disable rules per-line if needed)
    - gocritic       # opinionated; readable Go patterns
    - revive         # replacement for golint; configurable

  settings:
    revive:
      rules:
        - name: exported           # require doc comments on exported names
        - name: package-comments   # require package-level doc comments
        - name: var-naming
        - name: error-return
        - name: error-naming
        - name: error-strings
        - name: receiver-naming
        - name: indent-error-flow
        - name: superfluous-else
        - name: unreachable-code
        - name: unused-parameter

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

Notes: `errcheck` is the linter that would have caught gta's silent-failure mode if gta had it on. It's the one we most want enabled. `revive`'s `exported` and `package-comments` rules enforce the "every public API has godoc" discipline that pairs with the API-stability framing in the high-level plan.

### 9. Branch protection on `main`

Configure (via GitHub web UI or `gh api`, post-CI-going-green):

- Require pull request before merging
  - Require ≥ 1 approving review
  - Dismiss stale reviews on new commits
  - Require review from Code Owners (defer until a `CODEOWNERS` file exists; not in M1 scope)
- Require status checks to pass before merging
  - Required: `test (Go 1.25.3)`, `test (Go 1.26.x)`, `lint`
  - Require branches to be up to date before merging (recommended; rebase-or-update flow)
- Require conversation resolution before merging
- Require linear history (recommended for an OSS project; keeps `main` bisectable)
- Do NOT allow force pushes
- Do NOT allow deletions
- Signed commits: **CONSIDER** — turning this on is a hard gate against contributor flow if you don't have a clear signing setup documented in CONTRIBUTING.md. Recommend deferring until v0.2 when contributor patterns settle.

### 10. Repo-health files

Lightweight, OSS-conventional, checked in at M1 so contributor expectations are clear from the start.

**`CONTRIBUTING.md`:**

- One-paragraph project description and link to `0001-cascade-high-level-project-plan.md`
- Development setup: `make check-tools`, `make check`
- Branch / PR conventions: feature branches off `main`, PRs go through CI, ≥1 approving review
- Code conventions: lean on the Go best practices in `assets/ai/go/` (symlinked); cascade-specific overrides are testify/require for assertions, table-driven tests, file ordering with test functions before helpers, var-block grouping, and 100% coverage on `golist`/`depgraph`/`changeset`
- Commit message format: imperative mood, summary line ≤72 chars, optional body separated by blank line. No required prefix (this is an OSS project, not internal-monorepo-style `[XXX-NNN]`).
- Pointer to `assets/ai/CLAUDE-CODE-COVERAGE.md` for coverage discipline

**`CODE_OF_CONDUCT.md`:** Contributor Covenant 2.1, unmodified, with a `geomyidia` contact email substituted in. (Standard OSS practice.)

**`SECURITY.md`:** Vulnerability reporting policy. For an OSS tool that runs `go list` + `git diff` and produces a list of package paths, the attack surface is small, but the file should still exist for convention. Recommend: GitHub's private vulnerability reporting (built-in to repo settings) + a `security@` or maintainer contact email.

**`.github/PULL_REQUEST_TEMPLATE.md`:** brief — what the PR does, what it touches, how to verify, any breaking-change flag.

**`.github/ISSUE_TEMPLATE/bug_report.md`** and **`feature_request.md`:** GitHub's defaults are fine; copy and lightly tweak.

**`CHANGELOG.md`:** **Defer to M6.** No release, no changelog entry to write yet.

## Implementation order

A reasonable M1 execution sequence (each step is a small, reviewable PR):

1. **Stage 1: in-tree work** (no external services touched)
   - Drop `go.mod` Go directive to `1.25.3`
   - Add `Makefile` to git (it's currently untracked)
   - Add `cmd/cascade/main.go` placeholder + companion `main_test.go`
   - Add `golist/doc.go`, `depgraph/doc.go`, `changeset/doc.go`
   - Add `scripts/coverage-check.sh` (chmod +x)
   - Add `.golangci.yml`
   - Add `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `.github/PULL_REQUEST_TEMPLATE.md`, `.github/ISSUE_TEMPLATE/{bug_report,feature_request}.md`
   - Verify locally: `make check-all` passes
2. **Stage 2: CI activation**
   - Add `.github/workflows/ci.yml`
   - Push the branch; verify CI runs green
   - Intentionally introduce a coverage gap on a deliberate branch; verify CI fails; revert
3. **Stage 3: branch protection**
   - Configure protection rules per §9 via GitHub UI or `gh api`
   - Verify by attempting a direct push to `main` (should be rejected)
   - Verify by opening a PR with failing CI (merge button should be disabled)

Each stage is independently revertible. Stage 1 is pure additive; Stage 2 lights up gates; Stage 3 enforces them.

## Exit criteria

- [ ] `go build ./...` succeeds on Go 1.25.3 and 1.26.x
- [ ] `go test ./...` passes (no real tests yet, just the placeholder integration test)
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean
- [ ] `gofmt -l .` returns nothing
- [ ] `go mod tidy` is a no-op (go.mod is tidy)
- [ ] `make check-all` passes locally
- [ ] CI runs green on a fresh PR
- [ ] Coverage gate fires correctly when violated (verified by deliberately breaking it)
- [ ] Branch protection rules in place; direct push to `main` rejected
- [ ] All repo-health files present and link-checked
- [ ] `go install github.com/geomyidia/cascade/cmd/cascade@<HEAD>` succeeds and the resulting `cascade --version` prints version metadata

## Risks and edge cases

**Risk: Go module proxy lag on a fresh push.** Until proxy.golang.org indexes the module, `go install ...@<commit>` may fail with a fetch error for a few minutes. Not blocking; just a documented quirk.

**Risk: `golangci-lint`'s default ruleset drift.** The action pinned to `version: latest` means lint output can change between CI runs as new rules ship. Mitigation: pin to a specific version once the M1 PR settles, e.g., `version: v1.62.0` (or whatever's current at M1 close-out).

**Risk: cross-platform `coverage-check.sh` behavior.** The script uses `awk` for arithmetic which is portable, but `bash -u` is GNU-bash idiomatic. Linux runners are fine. macOS contributors may hit `bash 3.2` issues (the system `/bin/bash`). Mitigation: shebang `#!/usr/bin/env bash` and document that contributors need bash ≥ 4 in `CONTRIBUTING.md`. Or rewrite in Go if it gets gnarlier than ~80 lines.

**Risk: coverage threshold churn during M2-M4.** As implementations land, the thresholds will fire on partial-implementation PRs. Mitigation: each milestone PR ships its package's tests in the same PR as its implementation, so the gate stays meaningful. The script's "skip empty packages" branch keeps M1 itself green.

## Open questions to surface before execution

1. **`go.mod` Go directive.** Currently staged as `go 1.26.2`; plan calls for `1.25.3`. Confirm the drop to `1.25.3` (recommended) so consumers on Go 1.25.x can still install.

2. **Coverage threshold reconciliation.** Three numbers in flight: plan = 100% on non-main/non-io, prompt = 95% baseline, Makefile = 90% overall floor. Recommended state: per-package CI gate at 100% on the three pure packages (via `scripts/coverage-check.sh`), Makefile's `coverage-check` target left at 90% as a quick local sanity check. Confirm or override.

3. **Required CI checks for branch protection.** I've listed `test (Go 1.25.3)`, `test (Go 1.26.x)`, `lint`. The Go-version check names are dynamic — when GitHub's protection-rule config asks for the exact name, it'll be the form the matrix expansion produces. Worth a quick verification on the first CI run.

4. **`golangci-lint` ruleset scope.** I've picked a moderately strict set (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `gosec`, `gocritic`, `revive`). `gosec` in particular can be noisy on initial integration. If it's loud against the placeholder `main.go`, drop it for M1 and revisit at M5.

5. **Signed commits.** I recommend deferring to v0.2; OK to skip for M1?

6. **Issue templates beyond bug/feature.** Common additions: `documentation.md`, `question.md`. Worth adding now or trim?

7. **Multi-remote push pattern (macpro/github/codeberg).** This is your existing local-dev convention (in the Makefile). Should the M1 doc explicitly mention it as a maintainer-only flow, or treat it as private workflow not to surface in public docs?

8. **Go-version matrix policy at the next major.** When Go 1.27 ships (~Aug 2026), Go's two-newest-major support window will move to {1.26, 1.27}. cascade's matrix should follow. Worth a one-liner in CONTRIBUTING.md? ("CI matrix tracks Go's currently-supported major versions.")

9. **`CODEOWNERS` file.** Single-maintainer for now (Duncan). Worth a stub `CODEOWNERS` file with `* @maintainer` (or whatever your GH handle is)? This pairs with the "require review from Code Owners" branch protection rule once a second contributor exists.
