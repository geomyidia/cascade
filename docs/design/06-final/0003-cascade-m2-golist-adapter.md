---
number: 3
title: "M2: `golist` Adapter"
author: "these patterns"
component: All
tags: [change-me]
created: 2026-05-06
updated: 2026-05-06
state: Final
supersedes: null
superseded-by: null
version: 1.0
---

# cascade — M2: `golist` Adapter

**Status:** draft. Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md). Predecessor: M1 (repo scaffold + CI baseline). Successor: M3 (dep graph + reverse-dep closure).

## Goal

Build the `golist` package — the io shell that converts `go list -deps -json -tags=<union> <patterns...>` output into a typed `[]Package` slice. This is the only place in cascade that talks to the `go` toolchain.

The exit signal is: `golist.Run(ctx, tags, patterns)` returns a parsed, sorted `[]golist.Package` for a real Go module; CI runs the per-package coverage gate at 100% on the algorithmic surface; every io error has a typed return path with enough context that callers can act on it (the structural rejection of gta's silent-failure mode).

## Why M2 is its own milestone

Two reasons rather than one. First, it isolates the *only* component in cascade that does io against the `go` toolchain — getting this layer's contract right means M3, M4, and M5 are pure-data transformations over a stable typed input. Second, it captures the lesson from gta: silent failure from `packages.Load` is what bit us. M2's design constraints — explicit error returns, typed error chains, fixture-driven parser tests at 100% coverage — exist specifically to make that failure mode structurally impossible here.

## Required reading (before implementation)

This section is non-optional. Per the substrate pillar of [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md) (Part II), the Go knowledge base is the distilled, auditable record of how cascade is meant to be written. Implementing M2 without loading the relevant guides means rederiving the conventions from training-data instinct rather than from the project's source of truth — which is exactly the failure mode the substrate pillar exists to prevent.

**Load order — do this first, before writing any code:**

1. **Index, always:** [`assets/ai/go/SKILL.md`](../../../assets/ai/go/SKILL.md). Read the "Document Selection Guide" table and the "Critical Rules" section. The SKILL file is the entry point; the chapters are loaded on demand.

2. **Anti-patterns, always:** [`assets/ai/go/09-anti-patterns.md`](../../../assets/ai/go/09-anti-patterns.md). Walk AP-01…AP-15 in particular — those are the error-handling and concurrency-lifecycle traps that map directly to M2's surface area. Note the IDs of any patterns you find yourself reaching for; cite them in commit messages and the closing report.

3. **Topic-specific for M2** (load these, in this order):

   - [`assets/ai/go/03-error-handling.md`](../../../assets/ai/go/03-error-handling.md) — M2's typed errors, the `Is`/`Unwrap` chain semantics, sentinel pattern, and `%w`-wrapping discipline live here. Load-bearing IDs: **EH-01** (wrap with `%w`), **EH-04** (`errors.Is` for sentinel comparison), **EH-07** (`errors.As` for typed extraction), **EH-36** (sentinel naming conventions). Anti-patterns to avoid: AP-01 (dropped errors), AP-02 (string-checked errors), AP-03 (`%v` losing chain), AP-04 (sentinel-by-string-compare).

   - [`assets/ai/go/06-concurrency.md`](../../../assets/ai/go/06-concurrency.md) — `Run` takes `context.Context`; cancellation must terminate the spawned `go list` process and the streaming decoder cleanly. Load-bearing IDs: **CC-08…CC-12** (context propagation; ctx as first parameter; never store ctx in a struct; `defer cancel()` immediately after `WithCancel`/`WithTimeout`), **CC-23** (per-goroutine recover for the decoder reader if a separate goroutine ends up there), **CC-42** (panic recovery in goroutines surfaces as a returnable error).

   - [`assets/ai/go/02-api-design.md`](../../../assets/ai/go/02-api-design.md) — `golist` is a public package; the API surface defined in §"Public API surface" is the contract. Load-bearing IDs: **API-41** (functional options over huge parameter lists — exactly what `WithDir`/`WithEnv`/`WithGoBin` follow), **API-42** (DI over package-level globals — no global config in `golist`).

   - [`assets/ai/go/05-interfaces-methods.md`](../../../assets/ai/go/05-interfaces-methods.md) — `Run` returns concrete types (`[]Package`, `*ExitError`, `*ParseError`); accept-interfaces-return-concrete is the operative principle. Load-bearing ID: **IM-17** (typed-nil traps when returning interface-typed errors — `Run` must return untyped `nil`, not a typed `*ExitError(nil)`, on the success path).

   - [`assets/ai/go/07-testing.md`](../../../assets/ai/go/07-testing.md) — Layer-1 fixtures and Layer-2 sample-module test live and die by these patterns. Load-bearing IDs: **TE-01…TE-15** (table-driven idiom with subtests + `t.Helper()` + `cmp.Diff`/`errors.Is` assertions), **TE-42** (`t.Context()` for the test-scoped context — used in `TestRun_ContextCancellation`), **TE-43** (write public-API tests in `package golist_test` so we exercise the contract from the outside, not the internals).

   - [`assets/ai/go/11-documentation.md`](../../../assets/ai/go/11-documentation.md) — Ledger row F-17 gates on `go doc` rendering cleanly. Load-bearing IDs: **DC-01** (every doc comment starts with the identifier name), **DC-02** (every exported name has a doc comment).

   - [`assets/ai/go/10-project-structure.md`](../../../assets/ai/go/10-project-structure.md) — `golist/` is a top-level package, not under `internal/` (per M1's library-API decision). Skim the package-boundary discipline; the relevant rule is "one package per directory, clear boundary, `doc.go` or package comment on `package golist`."

4. **Skim, don't load fully** (only if you find yourself reaching for them):

   - `01-core-idioms.md` — declarations, naming, control flow, gofmt discipline. The `golangci-lint` config from M1 already enforces most of this; loading the chapter only matters if you hit a style question the linter hasn't already answered.

   - `04-type-design.md` — `Package` is a value type with useful zero values; if you find yourself reaching for `func NewPackage(...)`, stop and read TD-01…TD-05 first.

   - `08-performance.md` — not relevant for M2's correctness work; revisit only if profiling becomes interesting in M5+.

**Closing-report expectation:** the M2 closing ledger (CC's responsibility) names the guides loaded at session start, and any pattern IDs cited during implementation. This is not compliance theater — it's the record that lets CDC review against specific patterns when checking the code, and lets future contributors understand which conventions were load-bearing for which decisions. A line like *"Implemented per EH-01 + EH-07; AP-04 avoided in error comparisons"* in a commit message is exactly the artifact that makes this discipline cumulative rather than every-session-from-scratch.

## Public API surface

`golist` is a public package (per the M1 milestone's library-API decision). Three exported types plus one exported function plus a small set of options.

### `Package`

The parsed shape of a single entry from `go list -deps -json` output. Fields are a deliberate subset of `go list`'s full schema — adding a field is a deliberate decision driven by a downstream need, not a passthrough.

```go
package golist

// Package mirrors the subset of `go list -deps -json` output that cascade's
// downstream packages (depgraph, changeset) consume. Fields not consumed are
// deliberately omitted — adding a field is a decision, not a default.
//
// All path fields are absolute. All slice fields are nil-safe (a package with
// no test imports has TestImports == nil, not []string{}; callers should not
// distinguish).
type Package struct {
    // Identity
    ImportPath string // canonical import path, e.g. "github.com/geomyidia/cascade/golist"
    Dir        string // absolute filesystem path to the package directory

    // Source files in this package, by category
    GoFiles        []string // .go files in package (excluding _test.go and tag-excluded)
    TestGoFiles    []string // _test.go files with `package foo` (internal tests)
    XTestGoFiles   []string // _test.go files with `package foo_test` (external tests)
    IgnoredGoFiles []string // .go files excluded by build tags or other filters

    // Direct imports, by source category
    Imports      []string // imports of GoFiles
    TestImports  []string // imports added by TestGoFiles
    XTestImports []string // imports added by XTestGoFiles

    // Categorisation (used downstream to filter stdlib / external deps)
    Standard bool   // true if this is a Go standard library package
    Module   *Module // module ownership; nil for stdlib
}

// Module identifies which Go module a Package belongs to.
type Module struct {
    Path string // module path, e.g. "github.com/geomyidia/cascade"
    Main bool   // true if this is the main module being built
}
```

**Decided:**

- `Module` is a pointer-to-struct rather than embedded value. `nil` cleanly represents "stdlib, no module" without needing a sentinel string.
- `Standard` is captured even though it duplicates information available via `Module == nil`, because downstream code reads `pkg.Standard` more naturally than `pkg.Module == nil`.

**Not exposed in M2** (deliberately deferred):

- `CgoFiles`, `SFiles`, `EmbedFiles`, etc. — non-Go source variants. Cascade doesn't compute affected sets from these in M3/M4. Adding later is non-breaking.
- `Deps` (transitive deps as a flat list) — `go list -deps` already gives us each transitive dep as a separate Package entry, so we don't need the flat list.
- `BuildID`, `Stale`, `Target` — build-system metadata not needed for affected-set computation.

### `Run`

The single io entry point. Shells out to `go list`, parses the streaming JSON output, returns a slice or an error.

```go
// Run shells out to `go list -deps -json -tags=<tags> <patterns...>` in the
// configured working directory, parses the streaming JSON output, and returns
// the parsed packages in encounter order (which is `go list`'s order — alphabetical
// by import path within each module).
//
// Behaviour on error:
//   - If `go` is not on PATH: returns an error wrapping exec.ErrNotFound.
//     errors.Is(err, ErrGoNotFound) is true.
//   - If `go list` exits non-zero: returns an *ExitError with stderr captured.
//   - If JSON parsing fails: returns a *ParseError with the offending payload.
//   - If ctx is cancelled or deadline expires: the spawned `go list` process is
//     killed (via exec.CommandContext); returns ctx.Err() wrapped.
//
// Concurrency: Run may be called concurrently from multiple goroutines. Each call
// spawns its own subprocess. The returned []Package is safe for concurrent reads
// after Run returns; callers must not mutate it.
//
// Preconditions:
//   - tags: each element is a single build-tag name (no commas, no whitespace).
//     Run joins them with commas before passing to `go list -tags=`.
//   - patterns: each element is a `go list`-compatible pattern (e.g. "./...",
//     "github.com/foo/bar/...", "all"). Empty slice is invalid; callers should
//     pass at least []string{"./..."}.
func Run(ctx context.Context, tags []string, patterns []string, opts ...Option) ([]Package, error)
```

**Decided:**

- `tags` is `[]string`, not `string`. Caller-side typing wins over implementation convenience.
- `patterns` is `[]string`. Common case is `[]string{"./..."}`; multi-pattern queries are real (e.g., `cmd/foo/... ./internal/...`) so we accept a slice.
- Context is the first parameter (per Go convention; per AP-08 in `go-guidelines/09-anti-patterns.md`).
- Returns `[]Package` (not `[]*Package`). Packages are value types; slices of values are simpler and the `Package` struct is small enough that copying is cheap.
- Run is a free function, not a method on a `Lister` struct or similar. There is no per-call state worth carrying; functional options handle everything.

### `Option`

Functional options for `Run`. Empty by default — the common case is `golist.Run(ctx, tags, patterns)` with no options.

```go
// Option configures a Run call. Apply options after the required positional args.
type Option func(*runConfig)

// WithDir sets the working directory for the spawned `go list` process.
// Defaults to the caller's current working directory.
func WithDir(dir string) Option

// WithEnv overrides the environment for the spawned `go list` process.
// Defaults to os.Environ().
func WithEnv(env []string) Option

// WithGoBin overrides the `go` binary path. Useful for testing against
// non-default toolchains. Defaults to "go" (resolved via $PATH).
func WithGoBin(bin string) Option
```

**Decided:**

- Functional options over a `Config` struct (per API-41 in `go-guidelines/02-api-design.md`). Future options don't break the signature.
- `WithDir` is the most likely option in practice (CI invocations frequently want to run from a specific module root).
- `runConfig` is unexported; callers compose configuration only via `Option` constructors.

### Error types

Two typed errors plus three sentinels. Callers use `errors.As` to extract details, `errors.Is` for category matching.

```go
// ErrGoNotFound is returned when the `go` binary cannot be found on $PATH
// (or at the path configured via WithGoBin). Wraps exec.ErrNotFound.
var ErrGoNotFound = errors.New("go binary not found")

// ErrGoListFailed is returned when `go list` exits with a non-zero status.
// Use errors.As to extract the *ExitError for stderr capture.
var ErrGoListFailed = errors.New("go list failed")

// ErrParseFailed is returned when streaming JSON decoding of `go list` output
// fails partway through. Use errors.As to extract the *ParseError for the
// offending byte offset and payload.
var ErrParseFailed = errors.New("go list output parse failed")

// ExitError captures the diagnostic context when `go list` exits non-zero.
type ExitError struct {
    Cmd      []string // full argv as passed to exec, for reproduction
    Dir      string   // working directory the command was run in
    ExitCode int
    Stderr   string   // captured stderr (verbatim, not truncated)
}

func (e *ExitError) Error() string  // "go list <args>: exit <N>: <first-line-of-stderr>"
func (e *ExitError) Is(target error) bool // matches ErrGoListFailed
func (e *ExitError) Unwrap() error  // returns *exec.ExitError when applicable

// ParseError captures the diagnostic context when JSON decoding fails.
type ParseError struct {
    Offset  int64  // byte offset in the stream where decoding failed
    Payload string // the offending payload, truncated to ParseErrorMaxPayload bytes
    Cause   error  // the underlying json error (typically *json.SyntaxError)
}

func (e *ParseError) Error() string  // "go list output parse failed at offset <N>: <cause>"
func (e *ParseError) Is(target error) bool // matches ErrParseFailed
func (e *ParseError) Unwrap() error  // returns Cause

// ParseErrorMaxPayload is the maximum number of bytes captured in
// ParseError.Payload. Larger payloads are truncated with "... (truncated)".
const ParseErrorMaxPayload = 4096
```

**Decided:**

- Both typed errors implement `Is` for sentinel matching and `Unwrap` for chain traversal. Callers can use either form.
- `ExitError.Stderr` is captured verbatim, not truncated. `go list`'s stderr on real failures (`go: updates to go.mod needed`, etc.) is small and the diagnostic value is high.
- `ParseError.Payload` is truncated at 4KB to cap memory in pathological cases. The exported `ParseErrorMaxPayload` constant lets callers know what to expect.
- `Cmd` is `[]string` (the argv) rather than a single joined string. Easier to construct, and callers can reproduce-by-exec without re-parsing.

## Out of scope (deferred to later milestones)

- **Building the import graph.** That's M3. `golist.Package` is the input to `depgraph.Build`; M2 stops at returning the slice.
- **File-to-package mapping.** That's M4. M2 captures `Dir` and the file lists; M4 implements the lookup.
- **CLI integration.** That's M5. M2 has no `cmd/` involvement.
- **Filtering stdlib or external module packages.** Returned `[]Package` includes everything `go list -deps` reports. M3 may filter using `Standard` and `Module.Main`, but M2 doesn't presume.
- **Build-tag inference.** Cascade requires the caller to pass the union explicitly via `Run`'s `tags` parameter (per the high-level plan's minimal-scope discipline).
- **On-disk caching of `go list` output.** Per the high-level plan: CI starts fresh each run; saved-state correctness is harder than recomputing.
- **Concurrent invocation of multiple `go list` calls within one `Run`.** Each `Run` call is one subprocess. Callers wanting parallelism call `Run` from multiple goroutines.

## Test strategy

Two layers, mirroring the M1 placeholder pattern.

### Layer 1 — fixture-driven parser tests (in-process; coverage-counted)

Unit tests over the streaming JSON decoder, using fixture files under `golist/testdata/`. The decoder operates on any `io.Reader`, so tests don't need a subprocess — they read fixture files directly. This is where 100% coverage is enforced.

**Fixtures to ship in M2** (each a JSON file with `\n`-delimited `go list -json` records):

- `single-package.json` — one package, with imports, no tests, in main module.
- `multi-package.json` — three packages with cross-imports, mixed main/external modules.
- `with-tests.json` — package with `TestGoFiles`, `XTestGoFiles`, and corresponding test imports.
- `build-tag.json` — package with `IgnoredGoFiles` populated (representing tag-excluded files).
- `stdlib-mixed.json` — a small selection where `Standard: true` packages appear alongside non-stdlib.
- `empty.json` — empty stream (zero records). Valid input; should return `nil, nil`.
- `truncated.json` — well-formed JSON cut off mid-record (simulates a `go list` that crashed). Should return `nil, *ParseError`.
- `malformed.json` — syntactically invalid JSON. Should return `nil, *ParseError`.

**Coverage target:** 100% statement coverage on `golist/` excluding the literal `cmd.Run()` line in the io shell (the os/exec invocation), which is gated by Layer 2 instead.

### Layer 2 — subprocess smoke test against an embedded sample module

A single end-to-end test, `TestRun_SampleModule`, that:

1. Skips under `testing.Short()`.
2. Sets `WithDir("testdata/sample-module")`.
3. Calls `golist.Run(t.Context(), nil, []string{"./..."})`.
4. Asserts the returned slice has the expected packages with the expected imports.

The sample module lives at `golist/testdata/sample-module/` and contains:

- `go.mod` declaring `module example.test/sample` (a name that cannot collide with cascade or any real module).
- `pkga/a.go` (package pkga, no imports).
- `pkgb/b.go` (package pkgb, imports pkga).
- `pkgc/c.go` (package pkgc, imports pkgb; has a `_test.go` file that imports an external stdlib package).

This test proves the build → exec → stream → parse chain works end-to-end against real `go list` output. Coverage of the os/exec line happens here (via real subprocess invocation) but is not gated.

### Layer 3 — manual sanity check (not automated)

After Layer 1 + Layer 2 pass: invoke `Run()` against a real Go project that previously broke gta. Confirm a non-empty `[]Package` is returned, with sane data. Document the result in the M2 closing ledger. This is the verification gate that distinguishes cascade from gta — proving on the actual codebase that bit gta. Not a CI test (depends on having that codebase locally), but a closure requirement.

## Acceptance ledger

Per [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md). Every row reaches a final status (`done`, `deferred`, `no-op`) before M2 closes. Evidence column is filled in by CC at the commit where each row lands.

| ID | Criterion | Verify | Significance | Origin | Status | Evidence | Notes |
|----|-----------|--------|--------------|--------|--------|----------|-------|
| F-1 | `golist/` package exists with `doc.go` containing the package comment from M1 §1.4. | `test -f golist/doc.go && head -3 golist/doc.go \| grep -q '^// Package golist'` | serious | M1 §1.4 carry-over | open | | |
| F-2 | `Package` struct exists with the exact field set defined in §"Public API surface". | `go doc github.com/geomyidia/cascade/golist.Package \| grep -q 'ImportPath\\s*string'` (and similar for each field) | serious | M2 §"Public API surface" | open | | All 11 fields must be present and exported. |
| F-3 | `Module` struct exists with `Path string` and `Main bool`. | `go doc github.com/geomyidia/cascade/golist.Module \| grep -E 'Path string\|Main bool'` | serious | M2 §"Public API surface" | open | | |
| F-4 | `Run` function signature matches the spec. | `go doc github.com/geomyidia/cascade/golist.Run \| grep -F 'Run(ctx context.Context, tags []string, patterns []string, opts ...Option) ([]Package, error)'` | serious | M2 §"Public API surface" | open | | |
| F-5 | `Option`, `WithDir`, `WithEnv`, `WithGoBin` exported. | `for s in Option WithDir WithEnv WithGoBin; do go doc github.com/geomyidia/cascade/golist.$s \|\| exit 1; done` | serious | M2 §"Public API surface" | open | | |
| F-6 | `ExitError`, `ParseError`, `ParseErrorMaxPayload` exported with the documented fields. | `go doc github.com/geomyidia/cascade/golist.ExitError \| grep -E 'Cmd\|Dir\|ExitCode\|Stderr'` (and similar for ParseError) | serious | M2 §"Error types" | open | | |
| F-7 | `ErrGoNotFound`, `ErrGoListFailed`, `ErrParseFailed` exported as `error` sentinels. | `go doc github.com/geomyidia/cascade/golist \| grep -E '^var Err(GoNotFound\|GoListFailed\|ParseFailed)'` | serious | M2 §"Error types" | open | | |
| F-8 | `*ExitError` chains correctly via `errors.Is(err, ErrGoListFailed)` and `errors.As(err, &e)`. | `go test -run 'TestRun_ExitErrorChain' ./golist` | serious | M2 §"Error types" | open | | |
| F-9 | `*ParseError` chains correctly via `errors.Is(err, ErrParseFailed)` and `errors.As(err, &e)`. | `go test -run 'TestRun_ParseErrorChain' ./golist` | serious | M2 §"Error types" | open | | |
| F-10 | All eight Layer-1 fixtures present and tested. | `for f in single-package multi-package with-tests build-tag stdlib-mixed empty truncated malformed; do test -f golist/testdata/$f.json \|\| exit 1; done` | serious | M2 §"Layer 1" | open | | |
| F-11 | Sample module present at `golist/testdata/sample-module/` with the four documented files. | `test -f golist/testdata/sample-module/go.mod && test -f golist/testdata/sample-module/pkga/a.go && test -f golist/testdata/sample-module/pkgb/b.go && test -f golist/testdata/sample-module/pkgc/c.go` | serious | M2 §"Layer 2" | open | | |
| F-12 | `TestRun_SampleModule` passes (skipped under `-short`). | `go test -run 'TestRun_SampleModule' ./golist` | serious | M2 §"Layer 2" | open | | |
| F-13 | Per-package coverage gate at 100% for `golist`. | `bash scripts/coverage-check.sh` | serious | M1 §1.5 carry-over | open | | The Layer-2 io line is the documented exception; coverage tooling counts it as 1 uncovered line at most. |
| F-14 | Cancelled context kills the subprocess and returns `ctx.Err()`. | `go test -run 'TestRun_ContextCancellation' ./golist` | serious | M2 §"Run" semantics | open | | |
| F-15 | Concurrent `Run` calls are safe (verified under `-race`). | `go test -race -run 'TestRun_Concurrent' ./golist` | correctness | M2 §"Run" concurrency note | open | | |
| F-16 | `golist` package has no non-stdlib imports. | `[ "$(go list -m all \| wc -l \| tr -d ' ')" = "1" ]` | correctness | minimal-deps discipline | open | | Cascade module + zero deps. |
| F-17 | Package documentation renders cleanly via `go doc`. | `go doc github.com/geomyidia/cascade/golist \| head -30` returns a useful overview | polish | DC-01 in `go-guidelines/11-documentation.md` | open | | |
| F-18 | Manual sanity check on a real Go module that previously broke gta. | Documented in closing report; commit SHA + commands run + the first 10 lines of output. | serious | M2 §"Layer 3" | open | | Not CI-automatable. |

**Closure expectations:**

- F-1 through F-13 are CI-verifiable. CC must produce green Verify-command output for each in the closing report.
- F-14 and F-15 are tests — they must exist and pass.
- F-16 and F-17 are spot-checks; closing report includes the command output.
- F-18 is the only row that depends on local, not-in-repo state. CC describes the test invocation and pastes the first ~10 lines of output. CDC verifies the description is plausible and the output looks right.

**Deferral budget:** zero rows expected to be deferred. If any row needs to be deferred, the closing report must give a re-entry condition (e.g., "F-15 deferred to M3 because race-detector currently flags X; M3 will land Y which removes the source of the race").

## Risks & mitigations

**`go list -deps -json` output format drift across Go versions.** The JSON schema is part of the `go list` interface; Go's compatibility promise covers the field set we depend on. Mitigation: capture the schema in fixtures, keep the matrix testing 1.25.3 and 1.26.x. If 1.27 introduces a field-rename, M2's tests will catch it.

**Streaming decoder partial reads.** `go list -json` writes records as a stream of `{...}` objects separated by whitespace, not a JSON array. `json.Decoder.Decode()` in a loop handles this, but a network-style "decoder buffered partial bytes when subprocess died" case needs explicit handling — the Layer-1 `truncated.json` fixture exercises this.

**Cancellation race.** `exec.CommandContext` kills the subprocess when ctx is cancelled, but the decoder may still be mid-Decode on the pipe. Make sure Run drains+closes pipes cleanly even under cancellation. The `TestRun_ContextCancellation` test exercises this with `t.Context()` + a deliberate cancel mid-stream. Run with `-race` to catch goroutine leaks (the reader/decoder goroutines must terminate, not orphan).

**`go list` slowness on large modules.** Not a correctness issue but a UX one. Mitigation: M2 doesn't try to optimise. M3 may add a context-deadline knob; M5 may add a `--quiet` / progress reporter. Out of scope for M2.

**Module-proxy / network dependency.** `go list -deps` may consult `proxy.golang.org` to resolve modules not yet in the local cache. In CI this is fine. In tests against the embedded sample module, the module has no external dependencies, so no network traffic happens — the smoke test is hermetic. Document this in the test comment to prevent future regressions.

**Fixture drift.** When a future Go release adds fields to `go list -json` output, our existing fixtures still parse (we ignore unknown fields by default). When Go *renames* a field we depend on, our tests fail loudly. Both behaviours are correct.

**`bash 3.2` portability of Verify commands in the ledger.** All Verify commands above are POSIX-portable. Confirmed by inspection: no `[[`, no `declare -A`, no `mapfile`. CDC can run them in any sh-family shell.

## Open questions

These are the remaining decisions Duncan needs to weigh in on (or explicitly delegate). Each is named, not implied — per the methodology's "disclosed deferral" discipline.

1. **`Run` parameter ordering — should `tags` precede `patterns` or vice versa?** I've put `tags` first because tags scope-the-query and patterns name-the-query. Other libraries put query first. Both are defensible; I'll align with whichever you prefer before CC starts.

2. **`WithEnv` — is it actually useful?** I included it for completeness, but in practice a caller wanting to override env can call `os.Setenv` before `Run`. If you'd rather not have it in v0.1's API surface (and add it later if a real need emerges), it's clean to drop. My weak preference: drop for now; add when needed.

3. **Empty `tags` — pass `-tags=""` or omit the flag entirely?** Behaviourally these may differ in edge cases (Go's tag handling has historically had quirks). My recommendation: omit the flag when `tags` is nil/empty. Confirm.

4. **`patterns == nil` (caller passes nothing)** — should that be an error, or default to `["./..."]`? I've specced it as an error (caller bug). Defending the user against their own laziness here might be friendlier; a default of `./...` would do that. Tell me which.

5. **Test file count.** Eight Layer-1 fixtures might be overkill, or might be insufficient (cgo files, embed files, etc., aren't covered). My read: eight is the right floor; CC may add more during implementation if coverage gaps surface. Confirm or trim.

6. **Sample module package layout.** I've sketched `pkga` / `pkgb` / `pkgc` (a chain with one cross-import and one test file). Is that enough surface for the smoke test, or should the sample module also exercise build tags (which would need a `pkgd_linux.go` and a `pkgd_darwin.go` or similar)? Easy to add; just want explicit sign-off.

7. **`Layer 3` — manual sanity check.** Which real Go module do you want CC to test against locally before closing M2? You've said you don't want the project named in the OSS docs, so the closing-ledger evidence for F-18 should describe the test in generic terms (module size, package count, test invocation, output shape) without identifying the project. Confirm that framing is acceptable.

8. **Ledger file location.** I've embedded the ledger in this design doc. The LEDGER_DISCIPLINE.md template suggests `milestones/<N>-<name>-ledger.md` as a separate file. Embedding keeps the spec and ledger together (one source of truth); separating gives CC a smaller artifact to update during implementation. Tell me which you prefer; I'll standardise on that for M3+.

## Cross-references

- Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md), §"M2 — `go list` adapter".
- Predecessor: M1 design ([`docs/design/05-active/0002-m1-repo-scaffold-ci-baseline.md`](../05-active/0002-m1-repo-scaffold-ci-baseline.md)) and M1 implementation plan ([`docs/dev/0001-m1-implementation-plan-repo-scaffold-ci-baseline.md`](../../dev/0001-m1-implementation-plan-repo-scaffold-ci-baseline.md)).
- Methodology: [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/SUBAGENT-DELEGATION-POLICY.md`](../../../assets/ai/SUBAGENT-DELEGATION-POLICY.md).
- Go canon for the io shell: `go-guidelines` skill, especially `03-error-handling.md` (EH-01 wrap with `%w`, EH-04 `errors.Is`, EH-07 `errors.As`), `06-concurrency.md` (CC-08…CC-12 context propagation), `09-anti-patterns.md` (AP-01…AP-08 error discipline; AP-09…AP-15 concurrency lifecycle), and `02-api-design.md` (API-41 functional options).
- Test discipline: [`assets/ai/CLAUDE-CODE-COVERAGE.md`](../../../assets/ai/CLAUDE-CODE-COVERAGE.md) (Go-flavoured; 95% baseline, with cascade's per-package 100% override on the algorithmic surface).
