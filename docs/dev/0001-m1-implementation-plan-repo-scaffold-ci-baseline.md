# M1 Implementation Plan: Repo Scaffold + CI Baseline

**Source spec:** `docs/design/05-active/0002-m1-repo-scaffold-ci-baseline.md`
**Parent plan:** `docs/design/01-draft/0001-cascade-high-level-project-plan.md`

## Context

Cascade is a brand-new Go CLI + library that computes the reverse-transitive closure of a Go change-set under the imports relation (replacing DigitalOcean's `gta`, which broke silently against Go 1.25's stricter `packages.Load`). M1 is the scaffold-only milestone: it puts in place the repo's plumbing — module declaration, directory layout, Makefile, CI workflow, lint config, repo-health files, and branch-protection rules — so M2/M3/M4 can land real code in `golist/`, `depgraph/`, and `changeset/` without rebuilding infrastructure each time.

The exit signal is: a clean push to `main` runs CI green on a Go-version matrix; the coverage gate fires correctly when violated; `go install github.com/geomyidia/cascade/cmd/cascade@<HEAD>` produces a runnable binary that prints version metadata.

## Decisions resolved (open questions from the design doc)

| # | Question | Decision |
|---|----------|----------|
| 1 | go.mod Go directive | **`go 1.25.3`** — drop from current 1.26.2 |
| 2 | Coverage gate model | **Per-package 100% in CI** (`scripts/coverage-check.sh`) **+ 90% overall in Makefile** for quick local sanity |
| 3 | golangci-lint scope | Full proposed set (errcheck, govet, ineffassign, staticcheck, unused, gosec, gocritic, revive). Drop/relax noisy rules in a follow-up if the first CI run is loud. |
| 4 | Signed commits | Defer to v0.2 |
| 5 | Issue templates | `bug_report.md` + `feature_request.md` only |
| 6 | CODEOWNERS | **Defer** — not in M1 |
| 7 | Multi-remote push | Keep in Makefile as maintainer flow; do **not** surface in CONTRIBUTING.md |
| 8 | Required CI checks for branch protection | `test (Go 1.25.3)`, `test (Go 1.26.x)`, `lint` — verify exact names from first CI run |
| 9 | Toolchain directive | Omit. Re-evaluate at M6. |

## Current working-tree state (verified)

**Already present:** `go.mod` (declares `go 1.26.2` — needs change), `Makefile` (already has ldflags version injection wired), `LICENSE` (Apache-2.0), `README.md`, `.gitignore` (excludes `assets/ai`, `workbench`, `.claude` — local working areas not part of the OSS surface), `odm.toml`, `assets/ai/CLAUDE-CODE-COVERAGE.md` (Go-flavoured; tracked from a prior commit, but the directory is now gitignored so further edits won't propagate), `assets/ai/go/` (symlink to `/Users/oubiwann/lab/billosys/ai-engineering/knowledge/go`), `assets/images/logo-v1.png`, `docs/design/{01-draft,05-active}/...`. Single git remote: `origin` → `git@github.com:geomyidia/cascade.git`.

**Missing — to be created in M1:** all `cmd/cascade/`, `golist/`, `depgraph/`, `changeset/`, `scripts/`, `.golangci.yml`, all `.github/` files, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`.

## Implementation

Three stages, each independently revertible. Stage 1 is purely additive (no external systems touched). Stage 2 lights up CI gates. Stage 3 enforces them.

### Stage 1 — In-tree files (single PR or split as preferred)

#### 1.1 Drop `go.mod` floor

**File:** `go.mod`
**Change:** `go 1.26.2` → `go 1.25.3`. No toolchain directive. No new requirements.

After change, run `go mod tidy` and confirm it's a no-op.

#### 1.2 Placeholder `cmd/cascade/main.go`

**New file:** `cmd/cascade/main.go`

Adapted from the design doc's §3 placeholder, with a `run()` indirection that makes the logic testable in-process (so `make coverage-check` has something to count — see §1.3 rationale below). Key properties:

- Declares package-level vars with explicit defaults so `go run` / `go install` (without ldflags) still produce sensible output:
  ```go
  var (
      Version   = "dev"
      GitCommit = "unknown"
      GitBranch = "unknown"
      BuildTime = "unknown"
  )
  ```
  The Makefile's existing `LDFLAGS_VERSION` block already injects values into these symbols, so no Makefile change is needed.
- `main()` is a one-liner: `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))`. All real logic lives in `func run(args []string, stdout, stderr io.Writer) int`. This is the standard Go pattern for testable mains and is what M5 will extend with the real CLI pipeline.
- `run()` uses `flag.NewFlagSet("cascade", flag.ContinueOnError)` with output routed to the passed-in stderr. Defines `--version` (bool).
- Exit-code contract:
  - `0` on `--version` (prints metadata to stdout) or `--help` (prints usage; surfaced by `flag.Parse` returning `flag.ErrHelp`)
  - `1` on flag-parse error other than `ErrHelp`
  - `2` on no flags / unknown positional args (prints "not yet implemented" to stderr — distinguishes M1 placeholder from a working install)

#### 1.3 Tests for the placeholder

**New file:** `cmd/cascade/main_test.go`

Two layers, both mandatory:

**Layer 1 — table-driven unit tests of `run()` (in-process; coverage-counted):**

```go
tests := []struct {
    name       string
    args       []string
    wantCode   int
    wantStdout string  // substring match
    wantStderr string  // substring match
}{
    {"version flag",       []string{"--version"}, 0, "cascade dev",        ""},
    {"help flag",          []string{"--help"},    0, "",                   "Usage of cascade"},
    {"flag parse error",   []string{"--bogus"},   1, "",                   "flag provided but not defined"},
    {"no flags",           []string{},            2, "",                   "not yet implemented"},
}
```

Tests call `run(tc.args, &stdoutBuf, &stderrBuf)` directly. No subprocess; coverage of `main.go` is captured normally. With these four cases, `cmd/cascade` should hit ~100% statement coverage — which keeps `make coverage-check` (Makefile-level 90% gate) green in M1.

**Layer 2 — single end-to-end smoke test, `TestCascadeBinaryVersion`, that:**

1. Skips under `testing.Short()`.
2. Builds the binary via `go build -o <tempdir>/cascade -ldflags="-X main.Version=test-1.0.0 -X main.GitCommit=abcdef" .`.
3. Executes the resulting binary with `--version`.
4. Asserts stdout contains `cascade test-1.0.0` and `commit abcdef`.

Layer 2 proves the build + ldflags-injection chain end-to-end. Layer 1 owns the coverage; Layer 2 owns the integration proof. Coverage of `cmd/cascade/` is **not** gated by the per-package CI script (io boundary; gated by Makefile's overall 90% only).

#### 1.4 Stub packages

Create three files with package-level docstrings (verbatim from design doc §4):
- **New file:** `golist/doc.go`
- **New file:** `depgraph/doc.go`
- **New file:** `changeset/doc.go`

Each is a one-package-comment file (no other code) so `go build ./...` and `pkgsite` have something to render and so the public-API boundary is reserved before M2/M3/M4 land code.

#### 1.5 `scripts/coverage-check.sh`

**New file:** `scripts/coverage-check.sh` (mode 0755)

Adapted from the design doc's §7 script, with one portability change: **use parallel arrays instead of `declare -A` (associative arrays)** so the script runs on macOS's default `bash 3.2.x` without forcing contributors to install Homebrew bash. Behavior:

- Shebang `#!/usr/bin/env bash`. POSIX-`set -euo pipefail`. No bash-4-only features.
- Reads `coverage.out` (default) or `$1`.
- Threshold table expressed as parallel arrays:
  ```bash
  PACKAGES=(
      "github.com/geomyidia/cascade/golist"
      "github.com/geomyidia/cascade/depgraph"
      "github.com/geomyidia/cascade/changeset"
  )
  THRESHOLDS=(100 100 100)
  ```
  Iterate by index: `for i in "${!PACKAGES[@]}"; do pkg="${PACKAGES[$i]}"; t="${THRESHOLDS[$i]}"; …; done`.
- Skips packages with no coverage data ("N/A") so the gate doesn't fire pre-M2 against empty stubs.
- Uses `awk` for float comparison (portable; no `bc` dependency).
- Emits `::error::` / `::notice::` / `ok:` lines. Exits non-zero on any threshold miss.

When future milestones add a new public package with implementation, the maintainer must add it to **both** parallel arrays — one entry for the import path, one for the threshold, both at the same index. That's the structural counterpart to "decide coverage policy explicitly per package."

#### 1.6 `.golangci.yml`

**New file:** `.golangci.yml` (verbatim from design doc §8). Linters: errcheck, govet, ineffassign, staticcheck, unused, gosec, gocritic, revive. Revive rules cover exported/package-comments/var-naming/error-* discipline.

`errcheck` is the linter that would have caught `gta`'s silent-failure mode — keep it on.

#### 1.7 `.github/workflows/ci.yml`

**New file:** `.github/workflows/ci.yml` (verbatim from design doc §6). Two jobs:

- **`test`** — matrix on `go: ["1.25.3", "1.26.x"]`. Steps: checkout, setup-go (with cache), `go mod tidy` + `git diff --exit-code go.mod go.sum`, gofmt-check, `go vet ./...`, `go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...`, then `./scripts/coverage-check.sh`. Uploads `coverage.out` as an artifact only on the floor Go version (`1.25.3`) to avoid duplicates.
- **`lint`** — single Go version (`1.26.x`); uses `golangci/golangci-lint-action@v6` with `version: latest` and `--timeout=5m`.

Top-level: `concurrency: group: ${{ workflow }}-${{ ref }}, cancel-in-progress: true`. `permissions: contents: read`.

#### 1.8 Repo-health files

**New file:** `CONTRIBUTING.md` — codify per design doc §10:
- One-paragraph project description + link to `docs/design/01-draft/0001-cascade-high-level-project-plan.md`.
- Dev setup: `make check-tools`, `make check`.
- Branch / PR convention: feature branches off `main`; ≥1 approving review; CI must pass.
- Code conventions: lean on `assets/ai/go/` (symlinked, gitignored). Cascade-specific:
  - testify/require for assertions
  - Table-driven tests
  - File ordering: test functions before helpers
  - Var-block grouping
  - 100% coverage on `golist`/`depgraph`/`changeset` (enforced by `scripts/coverage-check.sh`)
- Commit message format: imperative mood, summary ≤72 chars, optional body separated by blank line. **No required prefix** (this is OSS, not internal).
- Pointer to `assets/ai/CLAUDE-CODE-COVERAGE.md` for coverage discipline.
- One-liner on Go-version policy: "CI matrix tracks Go's currently-supported major versions; expect the floor to advance with each Go release."

**New file:** `CODE_OF_CONDUCT.md` — Contributor Covenant 2.1 verbatim, with `geomyidia` contact email substituted in. Ask the user for the contact email if not obvious.

**New file:** `SECURITY.md` — Vulnerability reporting policy. Direct reporters to GitHub's private vulnerability reporting (Settings → Security → Private vulnerability reporting). Note that cascade's attack surface is small (it shells out to `go list` + `git diff` and emits package paths) but the file should exist for convention.

**New file:** `.github/PULL_REQUEST_TEMPLATE.md` — short: what / where / how-to-verify / breaking-change flag.

**New file:** `.github/ISSUE_TEMPLATE/bug_report.md` — adapt GitHub's default. Slots: description, repro, expected, actual, environment (OS, Go version, cascade version).

**New file:** `.github/ISSUE_TEMPLATE/feature_request.md` — adapt GitHub's default. Slots: motivation, proposal, alternatives considered, scope.

**Skipped:** `CHANGELOG.md` (defer to M6), `CODEOWNERS` (deferred), `documentation.md` / `question.md` issue templates (not in M1).

#### 1.9 Stage 1 local verification

Before opening the Stage 1 PR, run locally:

```bash
make check-tools    # confirm go, gofmt, golangci-lint installed
make tidy           # confirm go.mod is tidy after the 1.26.2→1.25.3 change
make format         # gofmt + goimports clean
make lint           # gofmt-check + go vet + golangci-lint
make test           # go test -race -count=1 ./...
make build          # produces ./bin/cascade
./bin/cascade --version    # prints injected metadata
./bin/cascade --help       # prints flag.Usage
./bin/cascade              # exits 2 with "not yet implemented"
make coverage-check        # 90% Makefile-level gate. Passes in M1 because run() unit tests cover ~100% of cmd/cascade, which is the only package with statements in coverage.out.
bash scripts/coverage-check.sh  # per-package gate; all three pure pkgs report N/A → skipped → exit 0
make check-all      # everything together
```

### Stage 2 — Activate CI

1. Push the branch (with all Stage 1 files) to `origin`.
2. Open a PR. CI workflow runs.
3. Verify `test (Go 1.25.3)`, `test (Go 1.26.x)`, and `lint` jobs all go green.
4. **Negative test:** on a throwaway branch, deliberately introduce a coverage-gate violation — e.g., add a non-trivial exported function to `golist/` with no test. Push and verify CI fails with `::error::github.com/geomyidia/cascade/golist coverage X% < required 100%`. Then revert / discard the branch.
5. Capture the exact job names from the first green CI run for use in Stage 3.

### Stage 3 — Branch protection on `main`

Configure via GitHub Settings → Branches → Branch protection rule for `main` (or `gh api`):

- ✅ Require pull request before merging
  - Require ≥ 1 approving review
  - Dismiss stale reviews on new commits
  - Require review from Code Owners — **disabled** (no CODEOWNERS file in M1)
- ✅ Require status checks to pass before merging
  - **Required checks** (exact names from Stage 2): `test (Go 1.25.3)`, `test (Go 1.26.x)`, `lint`
  - Require branches up-to-date before merging — enabled
- ✅ Require conversation resolution before merging
- ✅ Require linear history
- ❌ Allow force pushes
- ❌ Allow deletions
- ❌ Require signed commits (deferred to v0.2)

**Verification:**
1. `git push origin main` directly (with a trivial test commit on `main` locally) → expect rejection with "branch protection".
2. Open a PR with a deliberately failing test → merge button is disabled.
3. Approve a clean PR → merge button is enabled.

After verification, drop the test commits and clean up the verification PRs.

## Critical files to be created/modified

| Path | Action | Reuses |
|------|--------|--------|
| `go.mod` | Modify (1.26.2 → 1.25.3) | — |
| `cmd/cascade/main.go` | Create | Makefile `LDFLAGS_VERSION` (lines 32–35) |
| `cmd/cascade/main_test.go` | Create | — |
| `golist/doc.go` | Create | — |
| `depgraph/doc.go` | Create | — |
| `changeset/doc.go` | Create | — |
| `scripts/coverage-check.sh` | Create (mode 0755) | — |
| `.golangci.yml` | Create | — |
| `.github/workflows/ci.yml` | Create | `scripts/coverage-check.sh` |
| `.github/PULL_REQUEST_TEMPLATE.md` | Create | — |
| `.github/ISSUE_TEMPLATE/bug_report.md` | Create | — |
| `.github/ISSUE_TEMPLATE/feature_request.md` | Create | — |
| `CONTRIBUTING.md` | Create | `Makefile` targets, `assets/ai/CLAUDE-CODE-COVERAGE.md`, `assets/ai/go/` |
| `CODE_OF_CONDUCT.md` | Create (Contributor Covenant 2.1) | — |
| `SECURITY.md` | Create | — |

**Existing files that won't change:** `Makefile` (already correct — `COVERAGE_THRESHOLD := 90`, version injection wired, all targets present), `LICENSE`, `README.md`, `.gitignore`, `odm.toml`, `assets/**`, `docs/**`.

## Verification (M1 exit criteria)

After Stage 3, all the following must hold:

- [ ] `go build ./...` succeeds on Go 1.25.3 and Go 1.26.x (verified by CI matrix)
- [ ] `go test ./...` passes (placeholder integration test only)
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run` clean
- [ ] `gofmt -l .` returns nothing
- [ ] `go mod tidy` is a no-op
- [ ] `make check-all` passes locally
- [ ] CI runs green on a fresh PR (both `test` matrix jobs + `lint`)
- [ ] Coverage gate fires correctly when violated (Stage 2 step 4)
- [ ] Branch protection rejects direct `main` pushes (Stage 3 step 1)
- [ ] All repo-health files present and links work
- [ ] `go install github.com/geomyidia/cascade/cmd/cascade@<commit-sha>` succeeds and `cascade --version` prints `cascade <sha-or-tag> (commit <short>, branch <branch>, built <ts>)`

## Risks & mitigations

- **Go module proxy lag** on a fresh push — `go install ...@<sha>` may fail for a few minutes until proxy.golang.org indexes. Documented quirk; not blocking.
- **`golangci-lint version: latest` drift** — pin to a specific version (e.g., `v1.62.x`) once M1 PR settles. Mitigation tracked as a follow-up.
- **`gosec` noise on the placeholder `main.go`** — if it fires, drop `gosec` from `.golangci.yml` for M1 and revisit at M5 once real CLI code lands.
- **macOS `bash 3.2` running `coverage-check.sh`** — addressed by using parallel arrays instead of bash-4 associative arrays (see §1.5). No contributor friction; no Homebrew-bash requirement. If the script grows beyond ~80 lines, rewrite as a Go script (`go run`-able) to drop the bash-portability concern entirely.
- **Coverage threshold churn during M2-M4** — each milestone PR ships its package's tests in the same PR as its impl, so the gate stays meaningful between milestones. The script's "skip empty packages" branch keeps M1 itself green.

## Out of scope (explicit)

- Real code in `golist/`, `depgraph/`, or `changeset/` — M2/M3/M4
- Real CLI flag handling beyond `--version` / `--help` — M5
- The `v0.1.0` release tag — M6
- Consumer-side CI re-wiring — separate "Ext" milestone, separate repo
- Release tooling beyond `make release VERSION=v0.1.0` (no GoReleaser, no release-please)
- `CHANGELOG.md` — defer to M6
- `CODEOWNERS` — deferred (single-maintainer)
- Signed commits — defer to v0.2
- Pinning `golangci-lint` version — follow-up after first green CI
- Discussions / non-bug-or-feature issue templates — not requested
