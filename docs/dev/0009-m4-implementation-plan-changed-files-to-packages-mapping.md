# Implementation Plan: M4 — `pkg/changeset` (Changed-Files-to-Packages Mapping)

**Source spec:** [`docs/design/05-active/0005-cascade-m4-changeset-changed-files-to-packages-mapping.md`](/Users/oubiwann/lab/geomyidia/cascade/docs/design/05-active/0005-cascade-m4-changeset-changed-files-to-packages-mapping.md)
**Predecessor:** package-layout refactor PR #8 (must merge to `main` before M4 PR opens).
**Successor:** M5 — CLI + main wiring (consumes `Resolve`'s output as seeds for `depgraph.RevDepClosure`).

## Context

M4 is the bridge between cascade's two algorithmic primitives: `golist` (M2) produces a `[]golist.Package` with file-membership information; `depgraph` (M3) computes reverse-transitive closures from a seed set. `changeset.Resolve` turns *"these files changed"* (the output of `git diff --name-only`) into *"these packages are the seeds"* (the input of `depgraph.RevDepClosure`).

Why now: the package-layout refactor (PR #8) lands `pkg/changeset/` with an M1 doc.go stub and the per-package coverage gate already wired at 100%. M4 fills the implementation directly into that location. Once M4 closes, the trio of pure packages (`pkg/golist`, `pkg/depgraph`, `pkg/changeset`) compose without adapter code: `g := depgraph.Build(pkgs); seeds := changeset.Resolve(changedFiles, pkgs, opts...); affected := g.RevDepClosure(seeds)`. This is the API the README's Library section already advertises; M4 makes it real.

The intended outcome: `pkg/changeset/` ships one exported function (`Resolve`) plus one option type (`Option`) plus one option constructor (`WithModuleRoot`); zero exported types beyond those; 100% statement coverage; godoc renders cleanly; M5 can wire the CLI on top with no pending design questions.

## Decisions resolved (with user input from plan-mode review)

The spec author left 6 open questions. The user resolved the three with material implementation impact during plan-mode review; the remaining three follow the spec's leans.

| # | Question | Decision |
|---|---|---|
| Q1 | `moduleRoot` positional vs functional option | **Functional option.** `Resolve(changedFiles, pkgs, opts ...Option)`; the only option is `WithModuleRoot(string)`. **User override of spec lean**: spec leaned positional for simplicity; user prefers the option pattern so the io-edge default (Getwd fallback) can be opted out of in tests. |
| Q2 | Empty/unset `moduleRoot` behaviour with relative paths | **Fall back to `os.Getwd()` at call time, but only when `WithModuleRoot` was not supplied.** Tests pass `WithModuleRoot(...)` explicitly so they don't trigger the io. **Implementation note**: this introduces a single io edge into a pure-data package, requiring the function-variable seam pattern from M2's `golist` (see "Implementation approach" below). If `os.Getwd` returns an error, `moduleRoot` stays empty and relative paths silently fail to resolve (parent-dir lookup misses) — same outcome as the spec's "skip relative paths" alternative, but only in the failure case. |
| Q3 | Lexical-form dedup via `filepath.Clean` | **Yes.** `filepath.Clean` is applied before lookup so `pkga/a.go`, `./pkga/a.go`, and `pkga/./a.go` all collapse to the same dir-key. Followed spec lean. |
| Q4 | Test fixtures: synthetic-only or repo fixture | **Synthetic-only.** Pure-data, no filesystem reads in production. Followed spec lean. |
| Q5 | `IgnoredGoFiles` handling | **Map regardless.** The lookup is parent-dir-match — file-list membership doesn't enter the algorithm at all. Followed spec lean (and confirmed by user). The implementation builds a `dir → importPath` map from `pkg.Dir` only; whether a file is in `GoFiles` / `TestGoFiles` / `XTestGoFiles` / `IgnoredGoFiles` is irrelevant. |
| Q6 | F-13 (godoc rendering) evidence shape | **Full `go doc` output capped at first 20 lines.** Consistent with the spec's verify command. Followed spec lean. |

## Implementation approach

### File layout

```
pkg/changeset/
├── doc.go              (M1 stub — untouched; spec F-1)
├── changeset.go        Resolve + Option + WithModuleRoot + internal config + getCwd seam + helpers
├── changeset_test.go   public-API tests (package changeset_test, per TE-43)
├── helpers_test.go     buildTestPackages helper (package changeset_test)
└── seam_test.go        getCwd seam-driven tests (package changeset, internal — only place we cross the package boundary)
```

`changeset.go` will land at ~80–120 lines including comments. If it grows beyond ~200, split into `changeset.go` (Resolve + algorithm) + `options.go` (Option type + constructors) — decide at write time.

### Public API surface

Three exported names (counting Option's constructor):

```go
package changeset

// Option configures a Resolve call.
type Option func(*config)

// WithModuleRoot sets the module root used to resolve relative entries in
// changedFiles. If WithModuleRoot is not supplied, Resolve falls back to
// os.Getwd() at call time. Tests should pass WithModuleRoot explicitly to
// avoid the io and keep test outcomes independent of the working directory.
func WithModuleRoot(dir string) Option

// Resolve maps changed file paths to the import paths of the packages whose
// Go files they are. Returns the set of distinct import paths sorted
// lexicographically for deterministic CI behaviour.
//
// [full doc per spec — mapping rules, dedup, sort, no error]
func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string
```

Every other type / function is unexported.

### Internal representation

```go
type config struct {
    moduleRoot      string
    moduleRootSet   bool   // distinguishes "explicitly empty" from "unset"
}

// getCwd is the function-variable seam over os.Getwd. Production builds use
// os.Getwd; seam_test.go replaces it to drive the success / error branches
// without depending on the actual test cwd. Pattern matches M2's runGoList.
var getCwd = os.Getwd
```

### Algorithm (O(P + F))

1. Apply options into a `config{}`. (`for _, opt := range opts { opt(&cfg) }`)
2. If `!cfg.moduleRootSet`, call `getCwd()`. On success → `cfg.moduleRoot = cwd`. On error → leave empty.
3. Build `dirMap := map[string]string{}` mapping `pkg.Dir` → `pkg.ImportPath` (one entry per pkg). Pre-allocate with `make(map[string]string, len(pkgs))` per AP-40.
4. Build the result set as `seen := map[string]struct{}{}` to dedup naturally.
5. For each file in `changedFiles`:
   - Skip empty.
   - Skip non-`.go` files (`!strings.HasSuffix(file, ".go")`).
   - If relative (`!filepath.IsAbs(file)`), join with `cfg.moduleRoot` (which may be empty if Getwd failed; in that case `filepath.Join("", relPath)` returns the path unchanged, which won't match any absolute `pkg.Dir`, so it gets silently skipped at lookup — correct behaviour).
   - `filepath.Clean` the joined/absolute path.
   - `parent := filepath.Dir(cleanPath)`.
   - If `importPath, ok := dirMap[parent]; ok`, record in `seen`.
6. Convert `seen` keys to a sorted `[]string`. Pre-allocate with `make([]string, 0, len(seen))` per AP-40. Return.

Empty / nil inputs at any stage produce `nil` return (no allocation when nothing matches).

### Existing patterns reused

- **M2's function-variable seam pattern** (`pkg/golist/golist.go:96-100`'s `runGoList = defaultRunGoList` + `pkg/golist/seam_test.go:11-17`'s `withSeam` helper). Same structural shape: an internal var holds the io-edge function; tests in the same package replace it via `t.Cleanup(restore)`. M4's `getCwd` follows the identical pattern. Reuse the exact `withSeam` idiom.
- **M3's table-driven test layout** (`pkg/depgraph/depgraph_test.go` + `pkg/depgraph/helpers_test.go`). One test file with `package changeset_test` for public-API cases; one helpers file for `buildTestPackages` shared across tests. Each subtest also asserts `sort.StringsAreSorted(got)` so non-determinism surfaces inline (M3's F-13 pattern).
- **Coverage-gate convention** in `scripts/coverage-check.sh:35-38` already lists `pkg/changeset` at threshold 100; no change needed.
- **`golist.Package.Dir`** is documented absolute (`pkg/golist/golist.go:14-21` package comment) and observed absolute in the sample-module fixture. The dir-map lookup relies on this; doc the assumption inline.

## Test strategy

All tests in `package changeset_test` for public-API cases (TE-43); seam-driven tests in `package changeset` (internal, `seam_test.go` only). Plain stdlib `testing`, no testify (matches existing cascade convention; honours TE-15).

### Helper (`helpers_test.go`)

```go
func buildTestPackages(t *testing.T, entries []golist.Package) []golist.Package
```

Spec proposed a map-based helper but the inline `[]golist.Package{...}` literal is cleaner for tests where each pkg has different `Dir` values, so the helper is essentially a thin t.Helper-bearing wrapper. Decide at write time; default to simpler form.

### Test functions

| Test function | Cases | Spec ledger row |
|---|---|---|
| `TestResolve_StandardCases` | 16 cases per spec (empty, nil pkgs, single/double Go file, _test.go, xtest .go, mixed Go+non-Go, file outside, file in subdir, removed, relative+moduleRoot, absolute, .. cleanup, dup, unsorted) | F-3 |
| `TestResolve_HandTraceable` | 4-pkg synthetic case from spec | F-4, F-6 (xtest), F-9 (removed) |
| `TestResolve_PathNormalisation` | OS-portable cases: redundant separators, `..`, mixed-style | F-5 |
| `TestResolve_DefaultUsesGetCwd` (in seam_test.go) | No WithModuleRoot supplied; getCwd seam returns "/test/cwd"; verify relative path resolves against it | (no spec row; coverage-completion) |
| `TestResolve_GetCwdErrorTolerated` (in seam_test.go) | getCwd seam returns error; verify relative paths silently skip; no panic | (no spec row; coverage-completion) |
| `TestResolve_IgnoredGoFiles_MappedRegardless` | File in a package's Dir whose name appears in IgnoredGoFiles → still maps to the package | (covers Q5 decision behaviourally) |

Coverage target: 100% on `pkg/changeset/`. The seam tests close the os.Getwd success/error branches without depending on the test process's actual cwd.

## Implementation order

1. **Pre-flight reading** (refresh against the spec's load list):
   - `assets/ai/go/SKILL.md` Critical Rules + Document Selection Guide (already in context from M3 session).
   - `assets/ai/go/guides/09-anti-patterns.md` walked. Specific IDs cited: **AP-29** (no interface returns), **AP-30** (no producer-side interfaces), **AP-36** (early-return over deep nesting), **AP-40** (slice prealloc), **AP-44** (no log+return), **AP-52** (filepath confinement — informational; M4 doesn't open files but worth knowing the prefix-check pattern exists).
   - `assets/ai/go/guides/02-api-design.md` API-42 (no globals; threaded through Resolve's args + opts).
   - `assets/ai/go/guides/07-testing.md` TE-01..15 + TE-43 (already in context).
   - `assets/ai/go/guides/11-documentation.md` DC-01 + DC-02 (already in context).
2. Branch `m4/changeset-mapping` off post-PR-#8-merge `main`.
3. Implement `pkg/changeset/changeset.go` — Option/WithModuleRoot/config/getCwd seam/Resolve.
4. Public-API tests in `pkg/changeset/changeset_test.go` (16 StandardCases + HandTraceable + PathNormalisation + IgnoredGoFiles_MappedRegardless).
5. Helper file `pkg/changeset/helpers_test.go` (buildTestPackages).
6. Internal seam tests in `pkg/changeset/seam_test.go` (DefaultUsesGetCwd + GetCwdErrorTolerated).
7. Local verification:
   - `go test -race -count=10 ./pkg/changeset/...` (F-3..F-10)
   - `bash scripts/coverage-check.sh` (F-11)
   - `make check-all` (build + lint + test + coverage gate)
   - `go doc github.com/geomyidia/cascade/pkg/changeset | head -20` (F-13)
8. Pre-PR CC self-check: walk each ledger row F-1..F-14, comparing planned-evidence text against criterion text (M3 retro carry-forward).
9. Commit + push + open PR + monitor CI matrix.
10. Closing retrospective at `docs/dev/0010-implementation-retrospective-m4-changeset.md` walking F-1..F-14 row-by-row.

## Critical files

| Path | Action | Notes |
|---|---|---|
| `pkg/changeset/doc.go` | Untouched | M1 stub satisfies F-1. |
| `pkg/changeset/changeset.go` | Create | Option, WithModuleRoot, config, getCwd, Resolve. |
| `pkg/changeset/changeset_test.go` | Create | Public API tests in `package changeset_test`. |
| `pkg/changeset/helpers_test.go` | Create | `buildTestPackages` helper. |
| `pkg/changeset/seam_test.go` | Create | Internal tests in `package changeset` for the `getCwd` seam. |
| `scripts/coverage-check.sh` | No change | Already lists `pkg/changeset` at threshold 100 from PR #8. |
| `docs/dev/0010-implementation-retrospective-m4-changeset.md` | Create at close | Closing report with per-row ledger walk. |

`README.md`, `CLAUDE.md`, `CONTRIBUTING.md` need no changes — the architecture sections already cover `pkg/changeset` post-refactor.

## Verification (mapping to spec ledger F-1..F-14)

| ID | Verify | Planned evidence shape |
|---|---|---|
| F-1 | `head -3 pkg/changeset/doc.go \| grep '^// Package changeset'` | M1 stub, untouched. Carry-over. |
| F-2 | `go doc ./pkg/changeset.Resolve` | `func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string` — exact match (note: signature differs from spec's positional form per Q1 decision; the active spec should pick this up at next ODM promotion). |
| F-3 | `go test -run TestResolve_StandardCases ./pkg/changeset` | 16 named subtests pass. |
| F-4 | `go test -run TestResolve_HandTraceable ./pkg/changeset` | 4-pkg case with hand-derived expected closures passes. |
| F-5 | `go test -run TestResolve_PathNormalisation ./pkg/changeset` | OS-portable cases pass. |
| F-6 | `go test -run 'TestResolve_StandardCases/_test\.go_in_pkga' ./pkg/changeset` | Test/xtest contract verified. |
| F-7 | `go test -run 'TestResolve_StandardCases/mixed' ./pkg/changeset` | Non-Go skipped. |
| F-8 | `go test -run 'TestResolve_StandardCases/Go_file_outside' ./pkg/changeset` | Outside-package files skipped, no error. |
| F-9 | `go test -run 'TestResolve_StandardCases/removed_Go_file' ./pkg/changeset` | Parent-dir match works for removed files. |
| F-10 | `go test -count=10 -run TestResolve ./pkg/changeset` | 10 runs, output sorted + deduped. Each subtest also asserts `sort.StringsAreSorted(got)` inline. |
| F-11 | `bash scripts/coverage-check.sh` | `ok: github.com/geomyidia/cascade/pkg/changeset coverage 100% >= 100%`. The `getCwd` seam closes the os.Getwd branch coverage gap. |
| F-12 | `[ "$(go list -m all \| wc -l \| tr -d ' ')" = "1" ]` | `1` (own module only; `pkg/changeset` imports `os`, `path/filepath`, `sort`, `strings` from stdlib + `pkg/golist` from the own module). |
| F-13 | `go doc github.com/geomyidia/cascade/pkg/changeset \| head -20` | Renders package overview, Option type, WithModuleRoot, Resolve with full doc comments. Capped at 20 lines per Q6 decision. |
| F-14 | reviewer reads closing report | `docs/dev/0010-implementation-retrospective-m4-changeset.md` enumerates substrate loaded + AP/API/TE/DC IDs cited. |

**Note on F-2 (signature drift from spec):** the spec says `func Resolve(changedFiles []string, pkgs []golist.Package, moduleRoot string) []string` (positional). User overrode to functional option during plan-mode review. The active spec at `docs/design/05-active/0005-...md` will need an amendment to match; CC will note this in the closing retrospective so the spec stays the source-of-truth. Same disclosed-amendment pattern as M3's F-20 (Stats type) addition.

## Risks & mitigations

- **Seam pattern correctness.** The `getCwd` seam must restore `os.Getwd` after each test. Use the `t.Cleanup(func() { getCwd = os.Getwd })` pattern from M2's `withSeam` helper (`pkg/golist/seam_test.go`). Mitigation: copy the M2 helper's exact shape; lint will catch missing restores via `t.Cleanup` discipline if a future reviewer adds a seam test without it.
- **`os.Getwd` introduces io into a "pure-data" package.** Per the spec's intent, M4 was supposed to be pure-data. Q2's resolution (default to `os.Getwd`) is a deliberate trade-off: friendliness at the call site (M5 CLI doesn't need to compute moduleRoot itself) at the cost of a single io edge. The seam keeps the io testable. Documentation: package doc.go's stub already says "pure" — update inline doc on Resolve to clarify the io-edge fallback.
- **Symlinks in `pkg.Dir` vs `changedFiles`.** `golist` reports resolved paths; `git diff` typically emits as-checked-in. If a contributor's checkout has symlink-divergence, lookup may miss. Per spec mitigation: document the limitation in Resolve's godoc; do not follow symlinks in M4.
- **Cross-platform path separators.** `pkg.Dir` may use `\` on Windows; `filepath.Clean` and `filepath.Dir` handle this natively. Don't normalise via `filepath.ToSlash` — work in OS-native form throughout. CI is Linux-only so any Windows-specific bug surfaces at first Windows-contributor checkout.
- **Lint-cache drift recurrence.** OUT-1 (M3's fresh-cache fix) is in place; M4 inherits the protection. No new mitigation needed.
- **Coverage gate at 100% with the io edge.** The seam test pattern is exactly what closes this branch. Without the seam, the os.Getwd-success path (or the os.Getwd-error path) would be untestable in-process and the gate would drop below 100%.

## Out of scope

- Forward closure (packages → files). Spec out-of-scope.
- File-content inspection. Path-only logic.
- Build-tag awareness in change-set logic. Tag selection happens at `golist.Run` time; `Resolve` operates on whatever the input pkg list says.
- Symlink resolution.
- Cross-module change-sets.
- Renames as two events (handled trivially by independent per-path mapping).
- Error returns from `Resolve`. The whole API is graceful.
- A `WithIgnore(pattern)` option to skip files matching some glob. Speculative.
- A `ResolveFromAbsolute` variant that requires absolute paths only. Non-breaking future addition if perf surfaces.

## Open items deferred to closing retrospective

- Final pre-PR CC self-check on F-1..F-14 evidence-vs-criterion text.
- Confirmation that the F-2 spec-drift (positional → functional option) is correctly captured as a disclosed amendment, mirroring M3's F-20 handling.
- Whether the seam pattern ended up cleaner than expected, or surfaced any subtleties (e.g. parallel test interactions on the global `getCwd` var). M2's seam was clean; M4 should mirror.

## Carry-forward expected to land in M4 retrospective

- **Seam pattern reuse evidence.** M4 is the second use of M2's `getCwd`-style function-variable seam. Worth noting whether the pattern generalised cleanly (it should — the shape is mechanical).
- **Pure-data + io-edge tension.** M4 is the first cascade package that's *mostly* pure but has *one* documented io edge. Document the trade-off the spec amendment introduced (Q2's user override) so future packages know the convention: if a single io edge is needed and the rest of the package is pure, factor it through a function-variable seam and document the option-pattern opt-out in the godoc.
- **Plan-and-PR sequencing.** PR #8 (refactor) must merge before this PR opens, per the predecessor relationship. Confirm the merge happened before branching off.
- **End-to-end pipeline ready for M5.** Once M4 closes, the full `golist.Run → depgraph.Build → changeset.Resolve → g.RevDepClosure` chain is functional in-library. M5 wires it to a CLI; this retrospective should explicitly confirm the chain composes without adapter code (e.g. via a small integration test that runs the four steps against the sample-module fixture).
