# Implementation Plan: Package-Layout Refactor (`pkg/` + `internal/`)

**Source spec:** N/A — refactor, not a feature. Decisions resolved inline.
**Parent plan:** [`docs/design/01-draft/0001-cascade-high-level-project-plan.md`](../design/01-draft/0001-cascade-high-level-project-plan.md).
**Predecessor:** M3 — `depgraph` (must be merged to `main` before this PR opens).
**Successor:** M4 — `changeset` mapping (lands inside `pkg/changeset/` post-refactor).

## Context

cascade's top-level has gotten sprawly. Current state lists five Go-package directories at the repo root (`cmd/`, `golist/`, `depgraph/`, `changeset/`, `project/`) plus six non-Go directories (`scripts/`, `assets/`, `docs/`, `workbench/`, `bin/`, `.github/`) plus a dozen root files. The maintainer prefers a tighter top-level — the `src/` / `lib/` / `crates/` shape that other ecosystems make easy.

This refactor consolidates the Go-package set into two top-level Go directories:

- **`pkg/`** — the public Go API surface that downstream library consumers import (`golist`, `depgraph`, `changeset`).
- **`internal/`** — private library code that only cascade's own binary can import (`project` build metadata).
- **`cmd/`** — binary entry point; stays at top-level per Go canonical convention.

Result: three Go-package directories at top-level instead of five. The non-Go directories (scripts, docs, etc.) are untouched.

## Why now

Three factors line up:

1. **Pre-v1.0 window is the right time for breaking import-path changes.** The README's Status section commits to API stability *only* from v1.0 onward; v0.x explicitly allows surface iteration. Doing this refactor now means downstream consumers see one breaking change pre-v1.0 instead of two.
2. **M3 (depgraph) finishes before this lands.** Moving depgraph mid-implementation would force a fragile rebase. Letting M3 land at top-level and *then* moving the completed package is cleaner.
3. **M4 (changeset) starts after this lands.** Net win: M4's implementation goes directly into `pkg/changeset/` without a follow-up move.

## Decisions resolved

| # | Question | Decision |
|---|---|---|
| L-1 | Use `pkg/` despite Go-community ambivalence about it? | **Yes.** The Go FAQ and several authoritative voices (Cox, Cheney) discourage `pkg/` for small projects and recommend top-level. Cascade is small (4 packages today), so the convention-cost is real but minor. The maintainer's preference for a tight top-level wins, with eyes open to the trade-off. |
| L-2 | Which packages are public (→ `pkg/`)? | **`golist`, `depgraph`, `changeset`.** All three are exported per the high-level plan's library-API decision; all three are imported by `cmd/cascade/` plus reachable to external consumers via the public module path. |
| L-3 | Which packages are private (→ `internal/`)? | **`project`.** It carries cascade-specific build metadata (Version, GitCommit, ldflags injection targets). The pattern is generic (modeled on zylog) but the package's values are cascade-specific. Other Go projects copy the pattern; they don't import this package. Go's `internal/` rule structurally enforces this. |
| L-4 | Where does `cmd/` live? | **Top-level**, unchanged. `cmd/<binary>/` is a Go-canonical convention; moving it would be the *less* idiomatic choice. |
| L-5 | Migration mode: atomic vs gradual? | **Atomic** — single PR moves all four packages and updates every import path in one commit. Gradual migration (with type-aliases or temporary forwarding packages) is a v1.0+ concern; v0.x's no-stability commitment makes the atomic move acceptable. |
| L-6 | Sequencing | **After M3 merge, before M4 start.** Encoded in §"Pre-flight" below. |
| L-7 | `VERSION` file location | **Move to `internal/project/VERSION`.** The root-level `./VERSION` symlink stays (re-pointed from `project/VERSION` → `internal/project/VERSION`) to preserve the convention that "the version is at `./VERSION`" for casual readers and external tooling. |
| L-8 | Spec-amendment trail | **High-level plan (0001) §M1 said:** *"`internal/` is reserved for CLI-only glue that emerges in M5 — not used in M2-M4."* This refactor brings `internal/` into use earlier, for `project` rather than M5 CLI glue. Disclosed amendment, not silent drift. |

## Pre-flight

Refuse-to-start checks (the equivalent of M3's "wait for M2 merge" gate):

- [ ] M3 PR is merged to `main`. `git log --oneline main` shows M3's closing commit.
- [ ] M3 closing retrospective exists at `docs/dev/0006-m3-implementation-retrospective.md` (or wherever the next retro slot is) — refactor doesn't depend on the retro per se, but the retro is the cleanest closure marker.
- [ ] No outstanding work on a feature branch that touches the affected packages. `git branch -a` is clean.

If any of the above fails, this PR doesn't open yet. Wait.

## Implementation outline

Branch: `refactor/pkg-internal-layout` off post-M3-merge `main`. Single PR, ~50 lines of `git mv` + maybe 30 lines of import-path updates spread across the codebase.

### Stage 1 — Move directories

```bash
mkdir pkg internal

git mv golist pkg/golist
git mv depgraph pkg/depgraph
git mv changeset pkg/changeset
git mv project internal/project

# Re-point the root-level VERSION symlink
rm VERSION
ln -s internal/project/VERSION VERSION
```

`git mv` preserves history (each file's blame trail follows it into the new location). No other moves required.

### Stage 2 — Update import paths

Find every file that imports the affected packages and rewrite the path. The set is small enough to enumerate:

```bash
# Package import paths to migrate:
#   github.com/geomyidia/cascade/golist     → github.com/geomyidia/cascade/pkg/golist
#   github.com/geomyidia/cascade/depgraph   → github.com/geomyidia/cascade/pkg/depgraph
#   github.com/geomyidia/cascade/changeset  → github.com/geomyidia/cascade/pkg/changeset
#   github.com/geomyidia/cascade/project    → github.com/geomyidia/cascade/internal/project

# Files affected (verified by grep):
#   cmd/cascade/main.go       (imports project)
#   cmd/cascade/main_test.go  (imports project; has versionPkg const for ldflags)
#   pkg/depgraph/*.go         (depgraph imports golist post-M3)
#   pkg/depgraph/*_test.go    (test code imports golist for buildTestGraph helper)
#   pkg/changeset/*.go        (M4 will import golist; M1 stub doesn't yet)
```

Mechanical sed equivalent (CC may use any tool that gets the same result):

```bash
git grep -l 'github.com/geomyidia/cascade/golist[^/]'  | xargs sed -i.bak 's|github.com/geomyidia/cascade/golist|github.com/geomyidia/cascade/pkg/golist|g'
git grep -l 'github.com/geomyidia/cascade/depgraph[^/]' | xargs sed -i.bak 's|github.com/geomyidia/cascade/depgraph|github.com/geomyidia/cascade/pkg/depgraph|g'
git grep -l 'github.com/geomyidia/cascade/changeset[^/]' | xargs sed -i.bak 's|github.com/geomyidia/cascade/changeset|github.com/geomyidia/cascade/pkg/changeset|g'
git grep -l 'github.com/geomyidia/cascade/project[^/]'  | xargs sed -i.bak 's|github.com/geomyidia/cascade/project|github.com/geomyidia/cascade/internal/project|g'
find . -name '*.bak' -delete
```

The `[^/]` lookahead avoids touching the new `pkg/golist` paths during migration (since `cascade/pkg/golist` matches the prefix `cascade/golist` if you don't anchor). Test-by-grep before committing: `git grep 'github.com/geomyidia/cascade/golist[^/]'` should return nothing.

### Stage 3 — Update build / config files

| File | Change | Why |
|---|---|---|
| `Makefile` | `VERSION_PKG := $(MODULE_PATH)/project` → `$(MODULE_PATH)/internal/project` | ldflags injection target |
| `Makefile` | `cat project/VERSION` → `cat internal/project/VERSION` | canonical path read by `make` |
| `scripts/coverage-check.sh` | `PACKAGES` array import paths updated to `pkg/golist`, `pkg/depgraph`, `pkg/changeset` | per-package coverage gate's target list |
| `README.md` | Library section's `import` block: `cascade/golist` → `cascade/pkg/golist`, etc. | doc accuracy for library consumers |
| `CLAUDE.md` | "Pure core" section mentions `golist/`, `depgraph/`, `changeset/` at top-level | rewrite to `pkg/golist/`, `pkg/depgraph/`, `pkg/changeset/`; `internal/` section gets a one-liner about `internal/project` |
| `CONTRIBUTING.md` | Any path references | spot-check; minimal expected |

### Stage 4 — Local verification

```bash
make tidy           # confirm go.mod is still tidy (no-op expected)
make format         # gofmt + goimports clean
make lint           # gofmt-check + go vet + golangci-lint
make test           # go test -race -count=1 ./...
make build          # produces ./bin/cascade
./bin/cascade --version    # prints injected version metadata, confirming ldflags still wired
make check-all      # everything together
bash scripts/coverage-check.sh  # per-package gate against new paths
```

### Stage 5 — Push, PR, merge

Standard PR flow. The pre-push hook installed in the Git Safety Protocol commit will pass (refactor branch is a fast-forward target for `main`); CI matrix runs against the new layout.

## Critical files

| Path | Action | Notes |
|---|---|---|
| `golist/*` | `git mv → pkg/golist/*` | history preserved per `git mv` |
| `depgraph/*` | `git mv → pkg/depgraph/*` | history preserved |
| `changeset/*` | `git mv → pkg/changeset/*` | history preserved |
| `project/*` | `git mv → internal/project/*` | history preserved; includes `VERSION` file |
| `VERSION` (symlink) | `rm + re-create` | repoints from `project/VERSION` to `internal/project/VERSION` |
| `cmd/cascade/main.go` | Modify | import path `github.com/geomyidia/cascade/project` → `.../internal/project` |
| `cmd/cascade/main_test.go` | Modify | same; plus the `versionPkg` const used in ldflags-injection test |
| `pkg/depgraph/*.go` | Modify | import path `cascade/golist` → `cascade/pkg/golist` |
| `pkg/depgraph/*_test.go` | Modify | same |
| `Makefile` | Modify | `VERSION_PKG`, `cat project/VERSION` → `cat internal/project/VERSION` |
| `scripts/coverage-check.sh` | Modify | `PACKAGES` array entries (3 paths) |
| `README.md` | Modify | Library section import block |
| `CLAUDE.md` | Modify | Architecture section package paths + a one-liner on `internal/project` |
| `CONTRIBUTING.md` | Modify (likely) | spot-check for path references |

## Verification (acceptance ledger)

Per `assets/ai/LEDGER_DISCIPLINE.md`. Every row reaches a final status before the refactor PR merges.

| ID | Criterion | Verify | Significance | Status | Evidence |
|----|-----------|--------|--------------|--------|----------|
| L-F1 | `pkg/golist/` exists with all original golist files | `test -d pkg/golist && test -f pkg/golist/golist.go && test -f pkg/golist/parse.go && test -f pkg/golist/errors.go && test -d pkg/golist/testdata` | serious | open | |
| L-F2 | `pkg/depgraph/` exists with depgraph implementation (M3 carry) | `test -d pkg/depgraph && test -f pkg/depgraph/doc.go && [ "$(ls pkg/depgraph/*.go \| wc -l)" -ge 2 ]` | serious | open | |
| L-F3 | `pkg/changeset/` exists with the M1 stub | `test -d pkg/changeset && test -f pkg/changeset/doc.go` | serious | open | |
| L-F4 | `internal/project/` exists with project files | `test -d internal/project && test -f internal/project/version.go && test -f internal/project/version_test.go && test -f internal/project/VERSION` | serious | open | |
| L-F5 | Old top-level Go directories are removed | `! test -d golist && ! test -d depgraph && ! test -d changeset && ! test -d project` | serious | open | |
| L-F6 | No file imports the old paths | `! git grep -E 'github.com/geomyidia/cascade/(golist\|depgraph\|changeset\|project)[^/]' -- '*.go'` returns nothing | serious | open | the `[^/]` excludes the new `pkg/<name>` paths from the false-positive set |
| L-F7 | `cmd/cascade` imports the new internal path | `git grep 'github.com/geomyidia/cascade/internal/project' cmd/cascade/main.go` | serious | open | |
| L-F8 | `Makefile`'s `VERSION_PKG` points at internal | `grep '^VERSION_PKG' Makefile \| grep -q 'internal/project'` | serious | open | |
| L-F9 | `scripts/coverage-check.sh` PACKAGES updated | `grep -c 'pkg/' scripts/coverage-check.sh` returns 3 (one per public package) | serious | open | |
| L-F10 | README's Library example uses new import paths | `grep -c 'cascade/pkg/' README.md` returns ≥ 3 | serious | open | |
| L-F11 | `go build ./...` succeeds on Go 1.25.3 + 1.26.x | CI matrix run | serious | open | CI evidence |
| L-F12 | `go test -race ./...` passes | CI matrix run | serious | open | |
| L-F13 | `golangci-lint run` clean | CI lint job | serious | open | |
| L-F14 | `gofmt -l .` empty | CI gofmt step | serious | open | |
| L-F15 | `go mod tidy` is a no-op | CI tidy gate | serious | open | |
| L-F16 | Per-package coverage gate at 100% on `pkg/{golist,depgraph,changeset}` | `bash scripts/coverage-check.sh` | serious | open | gate now references new paths |
| L-F17 | Build + version chain works end-to-end | `make build && ./bin/cascade --version` shows injected version metadata | serious | open | proves ldflags still hits the moved package |
| L-F18 | `git mv` preserved history | `git log --follow pkg/golist/golist.go \| grep -q 'M2: golist adapter'` returns the M2 landing commit | correctness | open | confirms blame trail survived |
| L-F19 | Closing report names guides loaded + IDs cited (none expected for a pure refactor; explicitly note "no Go-pattern citations beyond standard `internal/` rules") | reviewer reads closing report | polish | open | |

**Closure expectation:** zero deferrals. This is a small, mechanical refactor; if any row needs to defer, something has gone unexpectedly wrong and the PR should pause for analysis.

## Risks & mitigations

**Symlink + git interaction.** The `./VERSION` symlink re-pointing is the most fragile part. If `git mv` doesn't handle the symlink correctly, the working tree may have a broken link. Mitigation: explicit `rm VERSION && ln -s internal/project/VERSION VERSION` rather than trying to `git mv` the symlink. Verify post-move: `cat VERSION` returns the version string. The existing `//go:embed VERSION` directive inside `internal/project/version.go` reads the *real* file (`internal/project/VERSION`), not via the root symlink, so the embed contract is unaffected.

**Stale `golangci-lint` cache.** The lint-cache fix landed in M3 (per the M3 impl plan's OUT-1) means local `make lint` should already match CI's cold-cache behaviour. If somehow this refactor surfaces a lint issue that local missed, it's the same failure pattern M2 hit and the M3 fix is the protection. No new mitigation needed.

**External library consumers' breakage.** Anyone who has written `import "github.com/geomyidia/cascade/golist"` against an in-development cascade will get a clean compile error after this PR merges. Pre-v1.0 explicitly allows this; the README's Status section documents v0.x as no-stability. Mitigation: nothing required beyond honouring the existing API-stability framing. The closing report should mention the breakage in the M3+1 retrospective so future v1.0 lock-in remembers this is the kind of move v0.x is for.

**`internal/` import-rule surprise.** Go's compiler enforces that `internal/` packages can only be imported by packages within the same module subtree. After the move, `cmd/cascade/` (in the same module) can still import `internal/project` — verified by L-F11. External consumers cannot — verified by the compiler refusing to build a downstream `import "github.com/geomyidia/cascade/internal/project"` (not an in-CI test, but confirmable spot-check by anyone curious).

**Active design docs reference old paths.** The active M3 spec (`docs/design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md`) and the M3 impl plan (`docs/dev/0005-m3-implementation-plan-depgraph-reverse-dep-closure.md`) reference `depgraph/` and `pkg/depgraph/`-distinguished paths inconsistently. Per the maintainer's instruction, **historical references are not updated.** The active M3 spec, by virtue of being the source-of-truth for completed M3 work, stays as-is. Forward-looking docs (this plan, the M4 design when it lands) use the new paths.

**Coverage gate path mismatch transient state.** Between Stage 1 (move directories) and Stage 3 (update `scripts/coverage-check.sh`), `make coverage-check` would fail because the script's PACKAGES array references paths that no longer exist. Mitigation: don't run coverage-check between stages; complete Stage 3 in the same commit as Stage 1+2.

## Out of scope (explicit)

- **Backward-compat aliases / forwarding packages.** Not creating `golist/golist.go` that re-exports `pkg/golist`'s symbols. v0.x breakage is allowed; aliases would muddy v1.0's API surface.
- **`cmd/cascade/` reorganisation.** `cmd/` stays at top-level. Moving it under `pkg/` or `internal/` would violate the Go-canonical convention without buying anything.
- **Adding new packages.** No new public surface, no new internal helpers. Pure layout refactor.
- **Updating M1, M2, M3 historical design/impl/retro docs.** Per the maintainer's instruction; only this plan and forward-looking docs use the new paths.
- **API surface changes.** Function signatures, type definitions, exported names — all unchanged. Only the import path differs.
- **Documentation rewrites.** README, CLAUDE.md, CONTRIBUTING.md get path updates only; the prose around them is unchanged.
- **Versioning bump.** This isn't a v0.1.0 release; M6 is. The refactor lands as a pre-release milestone marker, no tag.

## Carry-forward expected to land in the post-refactor retrospective

- **Did `git mv` actually preserve history cleanly across all four moves?** L-F18 spot-checks one file; the retro should walk all four packages' `git log --follow` results.
- **Did the import-path update miss anything subtle?** A `git grep 'github.com/geomyidia/cascade/(golist|depgraph|changeset|project)[^/]'` post-merge against `main` should return nothing. Worth re-verifying as a closing check.
- **Did the symlink survive the merge cleanly on every contributor's filesystem?** macOS APFS handles symlinks fine; Linux fine; Windows-with-default-git is the wildcard. CI runs Linux only, so a Windows-issue would surface only when a Windows contributor first clones. Worth a one-line note in the retro.
- **Forward-looking impact.** The M4 design doc references `pkg/changeset/` paths from the start; the M4 impl plan references `pkg/changeset/` deliverables. No retroactive doc edits required for older milestones.

## Cross-references

- Parent: [`docs/design/01-draft/0001-cascade-high-level-project-plan.md`](../design/01-draft/0001-cascade-high-level-project-plan.md), §M1 ("`internal/` is reserved for CLI-only glue that emerges in M5") — this plan amends that reservation timing.
- Predecessor: M3 ([`docs/design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md`](../design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md), [`docs/dev/0005-m3-implementation-plan-depgraph-reverse-dep-closure.md`](./0005-m3-implementation-plan-depgraph-reverse-dep-closure.md)).
- Methodology: [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md).
- Go canon: `assets/ai/go/10-project-structure.md` for `internal/` rules and module/package layout discipline. The `pkg/` placement is *contra* the FAQ's recommendation (per L-1) and doesn't have a single citable best-practice; this is a maintainer-aesthetic call.
