---
number: 5
title: "M4: `changeset` (Changed-Files-to-Packages Mapping)"
author: "API surface"
component: All
tags: [change-me]
created: 2026-05-07
updated: 2026-05-07
state: Active
supersedes: null
superseded-by: null
version: 1.1
---

# cascade — M4: `changeset` (Changed-Files-to-Packages Mapping)

**Status:** draft. Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md). Predecessor: package-layout refactor ([`docs/dev/0006-package-layout-refactor.md`](../../dev/0006-package-layout-refactor.md)). Successor: M5 (CLI + main wiring).

## Goal

Build the `changeset` package — pure-data mapping from a list of changed file paths to the import paths of the packages those files belong to. This is the small but load-bearing converter between cascade's two algorithmic primitives: M2 (`golist`) produces the package list with file membership, M3 (`depgraph`) produces the closure operation, and `changeset` is the bridge that turns *"these files changed"* into *"these packages are the seeds for `g.RevDepClosure`."*

The exit signal: `changeset.Resolve(changedFiles, pkgs)` returns the import paths of packages whose Go files appear in `changedFiles`, sorted lexicographically; per-package coverage gate at 100% holds against table-driven tests; a change-set with a file outside any package returns gracefully (empty or skipped, not an error).

## Why M4 is its own milestone

Two reasons. First, file-to-package mapping is conceptually independent of both `go list` parsing (M2) and graph traversal (M3) — keeping it isolated means changes to either neighbor (e.g., new file-list fields in `golist.Package` for cgo or embed support) don't ripple through changeset's tests. Second, it's the smallest of cascade's three pure-data packages by API surface: a single exported function. The 100% coverage gate is genuinely cheap and the package's correctness is determined by edge-case handling — exactly the shape that exhaustive table-driven tests are good at.

## Required reading (before implementation)

Per the substrate pillar of [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md). M4's load list is the leanest yet — single function, no state, no errors, no concurrency — so the topic-specific set is minimal.

**Load order:**

1. **Index, always:** [`assets/ai/go/SKILL.md`](../../../assets/ai/go/SKILL.md). Document Selection Guide + Critical Rules.

2. **Anti-patterns, always:** [`assets/ai/go/09-anti-patterns.md`](../../../assets/ai/go/09-anti-patterns.md). Walk the AP-NN list. M4-relevant clusters: filepath idioms (`filepath.Clean`, `filepath.Dir`, `filepath.ToSlash`), slice/map idioms (the entries about nil-map writes, sort determinism), and the API-design entries about minimal exports.

3. **Topic-specific for M4:**

   - [`assets/ai/go/02-api-design.md`](../../../assets/ai/go/02-api-design.md) — `Resolve` is the only exported name; the contract is the entire public surface. Load-bearing IDs: **API-42** (no globals; everything threaded through `Resolve`'s arguments), and the entries about minimal-API-surface discipline.

   - [`assets/ai/go/07-testing.md`](../../../assets/ai/go/07-testing.md) — pure-data testing under exhaustive table-driven cases. Load-bearing IDs: **TE-01…TE-15** (table-driven idiom), **TE-43** (write tests in `package changeset_test` so the contract is exercised, not the internals).

   - [`assets/ai/go/11-documentation.md`](../../../assets/ai/go/11-documentation.md) — `Resolve`'s godoc carries the full contract since it's the only exported name. Load-bearing IDs: **DC-01** (doc starts with the identifier name), **DC-02** (every exported name documented).

4. **Skim only if needed:**

   - `01-core-idioms.md` — `golangci-lint` enforces most.
   - `04-type-design.md` — there are no exported types in M4; only `Resolve`.
   - `03-error-handling.md`, `05-interfaces-methods.md`, `06-concurrency.md`, `08-performance.md`, `10-project-structure.md` — not load-bearing for M4.

Closing report names guides loaded + pattern IDs cited.

## Public API surface

One exported function plus an `Option` type and one option constructor. Three exported names total — still the leanest in the public surface.

### `Option` and `WithModuleRoot`

```go
package changeset

// Option configures a Resolve call. Apply via Resolve's variadic parameter.
type Option func(*config)

// WithModuleRoot sets the module root used to resolve relative entries in
// changedFiles. When supplied, Resolve uses dir directly without consulting
// the filesystem.
//
// If WithModuleRoot is not supplied, Resolve falls back to os.Getwd at call
// time. Tests should pass WithModuleRoot explicitly so test outcomes don't
// depend on the working directory and the io is bypassed.
func WithModuleRoot(dir string) Option
```

### `Resolve`

```go
package changeset

// Resolve maps changed file paths to the import paths of the packages whose
// Go files they are. Returns the set of distinct import paths sorted
// lexicographically for deterministic CI behaviour.
//
// changedFiles are paths to Go files that have been modified, added, renamed,
// or removed (typically `git diff --name-only` output, one per line). Paths
// may be relative (resolved against moduleRoot) or absolute; filepath.Clean
// is applied before lookup.
//
// pkgs is the package list returned by golist.Run; each Package's Dir field
// (absolute path) drives the lookup.
//
// opts configure the call. The only option in v0.x is WithModuleRoot; if not
// supplied, Resolve falls back to os.Getwd at call time. Typically M5's CLI
// passes WithModuleRoot(rootDir) with the value of `git rev-parse
// --show-toplevel`.
//
// Mapping rules:
//   - A file ending in ".go" whose parent directory exactly matches some
//     package's Dir maps to that package's ImportPath. This catches added,
//     modified, and removed Go files (the parent-directory match works
//     regardless of whether the file currently exists on disk).
//   - _test.go files map to the same package as non-test files in the same
//     directory. Internal-test (package foo) and external-test (package
//     foo_test) both yield foo's ImportPath, because affecting either re-runs
//     foo's tests.
//   - Files in a package's IgnoredGoFiles (build-tag-excluded) map to the
//     package — the rule is parent-dir match; file-list membership is
//     irrelevant to the lookup.
//   - Non-Go files (no .go extension) are silently skipped.
//   - Files in subdirectories of a package's Dir (e.g., testdata/) are
//     skipped, because the parent directory does not match any package's Dir.
//   - Files outside any package's Dir are silently skipped — empty seed,
//     not an error.
//
// Determinism: identical inputs yield identical output across runs. The
// returned slice is sorted; duplicates are deduplicated.
func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string
```

**Decided:**
- Three exported names: `Resolve`, `Option`, and `WithModuleRoot`. The contract fits in two function signatures plus an option-pattern config.
- `moduleRoot` is supplied via `WithModuleRoot(dir string) Option`, not as a positional parameter. The functional-option pattern future-proofs for additional knobs (e.g. `WithIgnorePattern`) and lets tests opt out of the `os.Getwd` fallback so they remain pure (no io). Plan-mode resolution of Q1.
- The single io edge (`os.Getwd` fallback when `WithModuleRoot` is not supplied) is hooked through an internal function-variable seam (`getCwd`) following M2's `runGoList` pattern. Tests in `seam_test.go` (`package changeset` internal) replace it to drive the success/error branches without depending on the actual test cwd. Plan-mode resolution of Q2.
- Return type is `[]string`, not a named `[]ImportPath` type. Consistent with M3's decision to use `string` for import paths throughout.
- No error return. Every input shape is handled gracefully — out-of-package files, non-Go files, removed files, nil/empty inputs, and `os.Getwd` failures all produce sensible output.

**Not exposed in M4** (deliberately deferred):
- A `WithIgnore(pattern)` option to skip files matching some glob. Speculative; no current need.
- A separate `ResolveFromAbsolute` variant that requires absolute paths only. The combined entry point with normalisation is friendlier; if a perf concern surfaces later, an absolute-only fast path is a non-breaking addition.

## Out of scope (deferred to later milestones or never)

- **Forward closure** (mapping packages back to files). Not needed by cascade; reverse-only direction.
- **File-content inspection.** `Resolve` looks at paths only — it never reads file contents. Whether a `_test.go` file has `package foo` or `package foo_test` is determined by whether `golist.Package` lists it under `TestGoFiles` or `XTestGoFiles`; both map to the same package's ImportPath, so the distinction doesn't surface in `Resolve`'s logic.
- **Build-tag awareness in the changeset.** `golist.Package.IgnoredGoFiles` lists files excluded by tags at `golist.Run` time; if a tag-excluded file changes but the active tag union doesn't enable it, mapping it is correct but the downstream `RevDepClosure` walks edges that exclude it. M4 maps it; M5+ may filter.
- **Symlink resolution.** Paths are compared after `filepath.Clean`; symlinks are not followed. If `changedFiles` contains a symlink path, it's matched literally. Document; don't try to be clever.
- **Cross-module change-sets.** `Resolve` operates over a single module's package list. A change-set spanning multiple modules requires the caller to invoke `Resolve` per module and merge.
- **Renames as two events.** `git diff` may emit a rename as old-path + new-path or as a single `R100` line. `Resolve` handles both — each path mapped independently.
- **Error returns.** `Resolve` always returns successfully. Bad inputs (empty slices, nil pkgs, blank moduleRoot) yield empty output, not errors.

## Test strategy

Pure-data, table-driven, all in `package changeset_test` per TE-43. Synthetic `golist.Package` literals constructed inline.

### Helper: `buildTestPackages`

```go
// buildTestPackages constructs a []golist.Package for use in tests. Compact
// representation: import path → (dir, go files, test go files, xtest go
// files). The caller chooses whether moduleRoot for the test is relative
// or absolute; tests may use a tmpdir or a fixed string.
func buildTestPackages(t *testing.T, entries map[string]struct {
    Dir          string
    GoFiles      []string
    TestGoFiles  []string
    XTestGoFiles []string
}) []golist.Package {
    // ...
}
```

### Test cases (table-driven)

The unit-test surface is small enough to enumerate per ledger row.

**`TestResolve_StandardCases` cases:**

- `empty changedFiles, any pkgs` → `[]`
- `nil pkgs, any changedFiles` → `[]`
- `single Go file change in pkga` → `[pkga]`
- `two Go file changes in pkga` → `[pkga]` (deduped)
- `one Go file change each in pkga and pkgb` → `[pkga, pkgb]` (sorted)
- `_test.go in pkga` → `[pkga]` (test file mapped)
- `xtest .go file in pkga` (file in `XTestGoFiles`, parent dir = pkga.Dir) → `[pkga]` (external-test mapped to package, not pkga_test)
- `mixed Go + non-Go (`.md`, `.json`)` → only Go files map; non-Go skipped
- `Go file outside any package's Dir` (e.g., `cmd/cascade/main.go` when pkgs only contains pkga, pkgb) → skipped
- `Go file in a subdirectory of a package's Dir` (e.g., `pkg/golist/testdata/foo.go` when pkga.Dir = `pkg/golist`) → skipped (parent dir `pkg/golist/testdata` doesn't match pkga.Dir = `pkg/golist`)
- `removed Go file` (path that doesn't exist on disk; parent dir matches pkga.Dir) → `[pkga]` (parent-dir match works regardless of existence)
- `relative path with moduleRoot resolution` → maps via moduleRoot+path
- `absolute path that already matches Dir prefix` → maps directly without moduleRoot prepending
- `path with `..` components` → cleaned before lookup (`filepath.Clean`)
- `duplicate entries in changedFiles` → deduped in output
- `unsorted entries in changedFiles` → output still sorted lexicographically

**`TestResolve_HandTraceable`:**

A 4-package synthetic case constructed inline with hand-derived expected output. Documents the mapping semantics and serves as a sanity check.

```text
   Packages:
     example.test/pkga    Dir=/m/pkga    GoFiles=[a.go, a_test.go]
     example.test/pkgb    Dir=/m/pkgb    GoFiles=[b.go]   XTestGoFiles=[b_x_test.go]
     example.test/pkgc    Dir=/m/pkgc    GoFiles=[c.go]
     example.test/pkgd    Dir=/m/pkgd    GoFiles=[d.go]   IgnoredGoFiles=[d_linux.go]

   Resolve(["pkga/a.go"], pkgs, "/m") = [example.test/pkga]
   Resolve(["pkga/a_test.go"], pkgs, "/m") = [example.test/pkga]
   Resolve(["pkgb/b_x_test.go"], pkgs, "/m") = [example.test/pkgb]
   Resolve(["pkga/a.go", "pkgc/c.go"], pkgs, "/m") = [example.test/pkga, example.test/pkgc]
   Resolve(["pkga/sub/x.go"], pkgs, "/m") = [] (subdirectory, no package match)
   Resolve(["README.md"], pkgs, "/m") = [] (non-Go file)
   Resolve([], pkgs, "/m") = []
```

**`TestResolve_PathNormalisation`:**

OS-portable path handling. Cases: `/abs/path`, `rel/path`, `./rel`, `../parent/x`, paths with redundant separators (`a//b/c`). `filepath.Clean` should normalise all of them before lookup.

### Coverage target

100% statement coverage on `pkg/changeset/`, gated by `scripts/coverage-check.sh`. With a single exported function and a small set of helpers, hitting 100% is straightforward via the case enumeration above.

## Acceptance ledger

Per [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md). Every row reaches a final status before M4 closes.

| ID | Criterion | Verify (reproducible) | Significance | Status | Evidence |
|----|-----------|----------------------|--------------|--------|----------|
| F-1 | `pkg/changeset/doc.go` exists with package comment | `test -f pkg/changeset/doc.go && head -3 pkg/changeset/doc.go \| grep -q '^// Package changeset'` | serious | open | preserved from M1 stub |
| F-2 | `Resolve` signature matches spec | `go doc github.com/geomyidia/cascade/pkg/changeset.Resolve \| grep -F 'func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string'` | serious | open | |
| F-3 | All `TestResolve_StandardCases` rows pass | `go test -run 'TestResolve_StandardCases' ./pkg/changeset` | serious | open | enumerated cases |
| F-4 | Hand-traceable 4-package case passes | `go test -run 'TestResolve_HandTraceable' ./pkg/changeset` | serious | open | spec exit criterion |
| F-5 | Path-normalisation cases pass | `go test -run 'TestResolve_PathNormalisation' ./pkg/changeset` | serious | open | OS-portable |
| F-6 | `_test.go` files map to the package, not to `_test` ImportPath | `go test -run 'TestResolve_StandardCases/_test\.go_in_pkga' ./pkg/changeset` | serious | open | test/xtest contract |
| F-7 | Non-Go files silently skipped | `go test -run 'TestResolve_StandardCases/mixed' ./pkg/changeset` | serious | open | |
| F-8 | Files outside any package skipped, no error | `go test -run 'TestResolve_StandardCases/Go_file_outside' ./pkg/changeset` | serious | open | spec exit criterion |
| F-9 | Removed files mapped via parent-dir match | `go test -run 'TestResolve_StandardCases/removed_Go_file' ./pkg/changeset` | correctness | open | |
| F-10 | Output is sorted lexicographically and deduplicated | `go test -count=10 -run 'TestResolve' ./pkg/changeset` | serious | open | 10 runs identical |
| F-11 | Per-package coverage gate at 100% on `pkg/changeset` | `bash scripts/coverage-check.sh` | serious | open | |
| F-12 | `pkg/changeset` has no non-stdlib imports beyond `pkg/golist` | `[ "$(go list -m all \| wc -l \| tr -d ' ')" = "1" ]` | correctness | open | minimal-deps; carry from M2/M3 |
| F-13 | `go doc github.com/geomyidia/cascade/pkg/changeset` renders cleanly | `go doc github.com/geomyidia/cascade/pkg/changeset \| head -20` returns useful overview | polish | open | DC-01/DC-02 |
| F-14 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | polish | open | methodology requirement |

**Closure expectations:**

- F-1 through F-12 are CI-verifiable. CC produces green Verify output for each row in the closing report.
- F-13 is a render-check; one line of `go doc` output in the closing report is sufficient evidence.
- F-14 is the closing-report attestation.
- Zero deferrals expected.

## Risks & mitigations

**Path normalisation edge cases.** Different OS path separators, redundant separators, `..` traversal, and absolute-vs-relative ambiguity all produce subtle bugs. Mitigation: standard library `filepath.Clean` + `filepath.IsAbs` + (when needed) `filepath.Join(moduleRoot, rel)`. The `TestResolve_PathNormalisation` test covers the cases.

**Symlinks in `pkg.Dir` or `changedFiles`.** `golist` reports resolved paths; `git diff` typically emits the as-checked-in path. If a contributor's checkout has symlink-resolved paths in one and not the other, lookup may miss. Mitigation: document the limitation in the godoc; don't try to follow symlinks in M4.

**Empty `pkgs` input vs `nil` `pkgs` input.** Both should yield `[]string{}` (empty slice) for any `changedFiles`. Behaviourally identical; both are tested. Mitigation: don't distinguish; treat both as "no packages to look up against."

**Performance on very large change-sets.** `Resolve` is O(F · P) in the naive implementation (F changed files, P packages). Cascade's typical scale (M2 F-18: ~2400 packages) and PR scale (~10–100 changed files) means even quadratic is fine. Mitigation: build a `Dir → ImportPath` map once (O(P)), then look up each changed file's parent dir (O(F)); total O(P + F). The implementation gets there naturally; no premature optimisation needed.

**`go list` reports paths with platform-specific separators.** On Windows, `pkg.Dir` may use backslashes. Mitigation: comparing parent-of-changedFile to `pkg.Dir` should use a single canonical form. Use `filepath.ToSlash` (or work in OS-native form throughout) — pick one and stay consistent. CI is Linux-only so the failure mode would surface only on a Windows contributor's first run.

**`changedFiles` contains paths that have been *renamed*.** `git diff --name-only` may emit both the old and new path as separate entries (or a single rename indicator depending on flags). `Resolve` handles each entry independently — no special-casing needed. Document.

**Single io edge from `os.Getwd` fallback.** Per the Q2 resolution, the package introduces one io call when `WithModuleRoot` is not supplied. Mitigation: hook through an internal function-variable seam (`getCwd = os.Getwd`) so seam_test.go can replace it; structurally identical to M2's `runGoList` seam. Tests bypass the io entirely by passing `WithModuleRoot` explicitly; production callers (M5 CLI) should also pass `WithModuleRoot` with the result of `git rev-parse --show-toplevel` rather than relying on the fallback.

## Open questions (resolved)

All six questions resolved during plan-mode review prior to M4 implementation. Recorded here for historical context; the decisions are now reflected in the API surface and ledger above.

1. **`moduleRoot` as positional parameter, or via functional option?** **Resolved:** functional option. The maintainer overrode the spec's "positional" lean: the option-pattern future-proofs for additional knobs (e.g. `WithIgnorePattern`) and, more importantly here, lets tests opt out of the `os.Getwd` fallback (Q2) by passing `WithModuleRoot` explicitly so they remain pure (no io). The cost — one extra `Option` type plus one constructor — is small relative to the testability gain.

2. **Behaviour when `moduleRoot == ""` and `changedFiles` contains relative paths.** **Resolved:** option (a) modified — when `WithModuleRoot` is *not supplied*, fall back to `os.Getwd` at call time; when `WithModuleRoot` is supplied (even with an empty string), use that value as-is. Tests bypass the `os.Getwd` io by always passing `WithModuleRoot`. The fallback is hooked through an internal function-variable seam (`getCwd`) so the success/error branches of `os.Getwd` are testable in-process without depending on the actual cwd. Pattern matches M2's `runGoList` seam.

3. **Should `Resolve` deduplicate across distinct lexical forms of the same path?** **Resolved:** yes — followed the spec's lean. `filepath.Clean` is applied before lookup; the result-set deduplicates via `map[string]struct{}{}`. Distinct lexical forms collapse naturally.

4. **Test fixtures: synthetic-only, or include a small repo fixture under `pkg/changeset/testdata/`?** **Resolved:** synthetic-only — followed the spec's lean. The function is path-only; no filesystem reads. Tests use POSIX-style absolute paths (`/m/pkga`) directly; CI is Linux-only and Windows portability is documented as out-of-scope.

5. **`IgnoredGoFiles` handling.** **Resolved:** map regardless — followed the spec's lean and confirmed by the maintainer. The mapping rule is "parent-dir match"; file-list membership is irrelevant to the lookup. A `.go` file in a package's `Dir` maps to that package whether it's in `GoFiles`, `TestGoFiles`, `XTestGoFiles`, `IgnoredGoFiles`, or none of them.

6. **Closing-report evidence for F-13 (godoc rendering).** **Resolved:** full `go doc` output capped at first 20 lines — followed the spec's lean. Consistent with the verify command.

## Version history

| Version | Date | Author | Change |
|---------|------|--------|--------|
| 1.0 | 2026-05-07 | Duncan McGreggor (with CC) | Initial active spec. Specced `Resolve` as a single positional-parameter function with no exported types and no io. Six open questions left for plan-mode review. |
| 1.1 | 2026-05-07 | Duncan McGreggor (with CC) | Folded the M4 plan-mode resolutions into the spec body. Public API surface gains an `Option` type and a `WithModuleRoot` constructor (Q1 user override of the positional lean). `Resolve` signature changes to `(changedFiles []string, pkgs []golist.Package, opts ...Option) []string`. Q2 resolution introduces a single `os.Getwd` fallback when `WithModuleRoot` is not supplied, hooked through an internal `getCwd` function-variable seam (mirrors M2's `runGoList` pattern). Q5 mapping rule for `IgnoredGoFiles` clarified as "map regardless" (followed spec lean, confirmed by maintainer). F-2 ledger row's verify command updated to grep for the functional-option signature. Open questions section retitled "Open questions (resolved)" with each Q1-Q6 marked resolved. Risks section gains an entry for the `os.Getwd` io edge with the seam pattern as the named mitigation. No behavioural drift from 1.0's contractual rules — same mapping rules, same dedup, same sort, same no-error contract. Implementation in commits `75617a2` (M4 implementation) and `f78dcb9` (M4 closing retrospective); spec amendment in commit `9e92a95`. |

## Cross-references

- Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md), §"M4 — Changed-files-to-packages mapping".
- Predecessor: package-layout refactor ([`docs/dev/0006-package-layout-refactor.md`](../../dev/0006-package-layout-refactor.md)) — M4 lands inside the new `pkg/changeset/` directory; signatures use the post-refactor import paths (`github.com/geomyidia/cascade/pkg/golist`, `github.com/geomyidia/cascade/pkg/changeset`).
- M3 design ([`docs/design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md`](../05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md)) — the `Resolve` output (a `[]string` of import paths) feeds directly into `depgraph.RevDepClosure`'s `seeds` parameter; the contracts are designed to compose without adapter code.
- Methodology: [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md).
- Go canon: `go-guidelines` skill, especially `02-api-design.md` (API-42), `07-testing.md` (TE-01…TE-15, TE-43), `09-anti-patterns.md` (filepath idioms cluster), and `11-documentation.md` (DC-01, DC-02).
- M2 closure scale evidence (informs assumptions): F-18 measured ~2400 packages on a real corpus; M4's `Resolve` should run in milliseconds at that scale because the algorithm is O(P + F) with P ≈ 2400 and F ≈ 10–100.
