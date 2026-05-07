---
number: 6
title: "M5: CLI + Main Wiring"
author: "parse error"
component: All
tags: [change-me]
created: 2026-05-07
updated: 2026-05-07
state: Active
supersedes: null
superseded-by: null
version: 1.0
---

# cascade — M5: CLI + Main Wiring

**Status:** draft. Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md). Predecessor: M4 ([`0005-cascade-m4-changeset-changed-files-to-packages-mapping.md`](./0005-cascade-m4-changeset-changed-files-to-packages-mapping.md)). Successor: M6 (`v0.1.0` release).

## Goal

Wire M2 (`pkg/golist`) + M3 (`pkg/depgraph`) + M4 (`pkg/changeset`) into a usable CLI binary with real flag parsing, real error handling, and an explicit exit-code contract. The pipeline:

```
git diff --name-only <base>..<head>   →   changed files
   │
   │  (or: read changed files from stdin)
   ▼
golist.Run(ctx, tags, []string{"./..."}, opts...)   →   []golist.Package
   │
   ├──►  changeset.Resolve(changedFiles, pkgs, WithModuleRoot(root))   →   []string seeds
   │
   └──►  depgraph.Build(pkgs)                                          →   *Graph
                                                                              │
                                                                              ▼
                                                                  g.RevDepClosure(seeds)   →   sorted []string
                                                                              │
                                                                              ▼
                                                                       stdout, one path/line
```

The exit signal: `cascade --tags=… --base=… --head=…` against a real Go module emits the affected-package set on stdout, exits 0 on success (including empty results), exits non-zero with a typed error message on every failure path. `cascade --help` prints the flag reference and exit-code table cleanly. `go install …@<sha>` produces a runnable binary.

## Why M5 is its own milestone

Three things make M5 distinct from M2/M3/M4:

1. **Integration over invention.** The pure-data trio is feature-complete after M4; M5 doesn't introduce new algorithms. The risk profile shifts from "can we get the algorithm right?" to "can we wire the pieces without leaking errors?" The methodology's spec-keeping discipline is the load-bearing protection here.
2. **A second io edge.** M2 introduced `os/exec` for `go list`. M5 adds `git diff --name-only` as a sibling exec. Same seam pattern (function-variable seam over `runGitDiff`), same error-discipline rules (every failure surfaces, never swallowed — the gta lesson applies to both shells).
3. **The user-facing surface.** `cascade --help`, error messages, exit codes, signal handling — all of these are first-impression artifacts for downstream consumers. They need to be coherent, documented, and testable in-process so regressions surface in CI rather than in the field.

## Required reading (before implementation)

Per the substrate pillar of [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md). M5's load list is closer to M2's than to M3/M4's — error chains and io-shell patterns dominate.

**Load order:**

1. **Index, always:** [`assets/ai/go/SKILL.md`](../../../assets/ai/go/SKILL.md). Critical Rules + Document Selection Guide.

2. **Anti-patterns, always:** [`assets/ai/go/09-anti-patterns.md`](../../../assets/ai/go/09-anti-patterns.md). Walk AP-01..AP-15 (error and concurrency clusters); AP-44 (no `log+return`); AP-30s (subprocess argv discipline).

3. **Topic-specific for M5:**

   - [`assets/ai/go/03-error-handling.md`](../../../assets/ai/go/03-error-handling.md) — every io call's error must be wrapped with `%w` and returned. Exit-code mapping is by `errors.Is`/`errors.As` against typed errors from `pkg/golist` plus a new `*GitDiffError` for the git seam. Load-bearing IDs: **EH-01** (`%w` wrap), **EH-04** (`errors.Is` for sentinel), **EH-07** (`errors.As` for typed extraction).
   - [`assets/ai/go/06-concurrency.md`](../../../assets/ai/go/06-concurrency.md) — context propagates from `main` through `cli.Run` into `golist.Run` and the git seam; signal handling cancels the context cleanly. Load-bearing IDs: **CC-08..CC-12** (context propagation discipline), **CC-23** (signal-driven cancellation idiom).
   - [`assets/ai/go/02-api-design.md`](../../../assets/ai/go/02-api-design.md) — `cli.Run` is the testable seam between `main` and the pipeline; no exported state. Load-bearing IDs: **API-42** (no globals).
   - [`assets/ai/go/07-testing.md`](../../../assets/ai/go/07-testing.md) — three test layers (unit-of-pipeline, subprocess-of-binary, manual sanity). Load-bearing IDs: **TE-01..TE-15**, **TE-43** (external test package for cli's public-from-cmd surface, internal `seam_test.go` for git-diff seam).
   - [`assets/ai/go/11-documentation.md`](../../../assets/ai/go/11-documentation.md) — `--help` text is documentation; the exit-code table appears both in the binary's `--help` output and in README. Load-bearing IDs: **DC-01**, **DC-02**.

4. **Skim only if needed:**

   - `01-core-idioms.md`, `04-type-design.md`, `05-interfaces-methods.md`, `08-performance.md`, `10-project-structure.md` — informational; M5 doesn't introduce new types or interfaces beyond the `*GitDiffError` typed error.

Closing report names guides loaded + pattern IDs cited.

## Public API surface

M5 is primarily a **binary**, not a library. The exported surface is intentionally minimal:

### `cmd/cascade/` (the binary entry)

```go
package main

func main() {
    os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
```

One-liner that delegates to `internal/cli`. The pattern from M1's placeholder + M2's seam testability extends here.

### `internal/cli/` (private; the pipeline)

Not exported beyond the cascade module — Go's `internal/` rule enforces this. The package contains:

```go
package cli

// Run is the testable entry point for the cascade CLI. It parses args,
// runs the pipeline, writes the affected-package set to stdout, and
// returns the appropriate process exit code per the exit-code contract.
//
// stdin is read only when --changed-files=- is passed; otherwise ignored.
// stderr is used for diagnostic and error output; never for primary output.
//
// Errors are wrapped, never swallowed. Every failure path maps to a
// specific exit code per the contract; unmapped errors map to exit 4
// (internal logic error).
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int

// GitDiffError captures the diagnostic context when `git diff` exits
// non-zero. errors.Is(err, ErrGitDiffFailed) returns true; errors.As
// extracts the typed error for the captured argv + stderr text.
type GitDiffError struct {
    Cmd      []string
    ExitCode int
    Stderr   string
}

func (e *GitDiffError) Error() string
func (e *GitDiffError) Is(target error) bool
func (e *GitDiffError) Unwrap() error

// ErrGitDiffFailed is returned when `git diff` exits with a non-zero
// status. Mirrors pkg/golist's ErrGoListFailed semantics.
var ErrGitDiffFailed = errors.New("git diff failed")
```

**No `pkg/cascade` "convenience" function ships in M5.** The four-line pipeline (golist.Run → depgraph.Build + changeset.Resolve → RevDepClosure) composes without adapters and is documented in the README's Library section. Adding `pkg/cascade.Run(...)` is a v0.x-or-later API surface commitment without a confirmed library consumer; deferring keeps the public surface minimal. Surfaced as Open Question 1 if you want it sooner.

### CLI flag set

| Flag | Type | Required | Default | Purpose |
|---|---|---|---|---|
| `--tags` | comma-separated string | yes (when no stdin pipe) | none | build-tag union passed to `go list -tags=` |
| `--base` | string | yes (unless `--changed-files=-`) | none | base git ref (e.g., `origin/main`) |
| `--head` | string | yes (unless `--changed-files=-`) | `HEAD` | head git ref |
| `--changed-files` | string | no | empty | path to a file with one change-set entry per line; `-` reads from stdin |
| `--root` | string | no | `.` | working directory for `go list` and module-root for `changeset.Resolve` |
| `--help` | bool | no | false | print usage and exit 0 |
| `--version` | bool | no | false | print version metadata and exit 0 |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | success (output may be empty if no Go files changed) |
| 1 | flag-parse error or missing required flags |
| 2 | `git diff` failed (`*GitDiffError` was returned somewhere) |
| 3 | `go list` failed (`*golist.ExitError` or wrapper) |
| 4 | internal logic error (should never occur — surface as a real bug) |

The exit-code table is documented in three places: the active spec (this doc), `--help`'s output, and the README's CLI-usage section.

## Out of scope (deferred)

- **`pkg/cascade.Run(...)` convenience function.** Kept as an Open Question; lean is "defer, the four-line pipeline composes."
- **Caching the dep graph.** Per high-level-plan minimal-scope discipline.
- **Watch mode / daemon.** Same.
- **Custom output formats** (JSON, line-prefixed, etc.). Newline-separated-paths-to-stdout is the contract.
- **Alternative VCS** (mercurial, fossil, etc.). Git only.
- **Configuration files.** Flags only.
- **Concurrent multi-`go-list` invocations.** Single subprocess per cascade invocation.
- **Plugins / extensibility hooks.**
- **Build-tag inference.** Caller passes union explicitly.

## Test strategy

Three layers, each owning a coverage band.

### Layer 1 — `internal/cli` unit tests (in-process; coverage-counted)

Tests live in `internal/cli/cli_test.go` (external `package cli_test` per TE-43) and `internal/cli/seam_test.go` (internal `package cli` to access the `runGitDiff` seam). Both files coexist by file-suffix convention; `go test` runs them together.

The `runGitDiff` seam mirrors M2's `runGoList` and M4's `getCwd`:

```go
// runGitDiff is the function-variable seam over the git-diff exec.
// Production builds use defaultRunGitDiff; tests replace it via
// withGitDiffSeam to drive specific branches without spawning a
// subprocess.
var runGitDiff = defaultRunGitDiff
```

**Layer 1 cases (table-driven):**

- `--version` → prints metadata to stdout, exit 0
- `--help` → prints usage including exit-code table to stderr (or stdout, conventional choice — TBD per Open Question 4); exit 0
- Missing required flag → typed error to stderr, exit 1
- Unknown flag → typed error to stderr, exit 1
- `--changed-files=-` reads from stdin; non-Go entries skipped per `changeset.Resolve` semantics
- `--changed-files=<path>` reads from a file
- `git diff` fails (seam returns `*GitDiffError`) → exit 2; stderr contains the captured stderr from git
- `go list` fails (mocked via golist's seam) → exit 3
- Successful pipeline against a synthetic in-memory package set → expected affected paths on stdout
- Empty result (no Go files in change-set) → empty stdout, exit 0
- Context cancellation mid-pipeline → returns cleanly with appropriate exit code (TBD per Open Question 5: which exit code?)
- Output is sorted lexicographically; deduplicated

### Layer 2 — end-to-end binary smoke (subprocess; not coverage-counted)

Single test in `cmd/cascade/main_test.go` (extending the M1 `TestCascadeBinaryVersion`):

`TestCascadeBinaryEndToEnd`:
1. Skip under `testing.Short()`.
2. Build the cascade binary with ldflags injecting test version metadata (carry-over from M1's pattern).
3. Invoke the binary against `pkg/golist/testdata/sample-module/` with synthetic stdin (echo of changed files) and a synthetic tag set.
4. Assert exit code 0, stdout contains the expected affected packages, stderr is clean.

Confirms the build → exec → wire → output chain works end-to-end against real `go list`. Coverage-incidental.

### Layer 3 — manual sanity check (not automated)

Same shape as M2's F-18: invoke the merged-to-main cascade binary against a real Go module that previously broke gta. Capture: commit SHA, command run, first ~10 lines of output, exit code, wall-clock time. Document in the closing retrospective with generic framing per the established convention.

This is the load-bearing closure for M5 — proving the integrated pipeline actually solves the original problem on a real codebase, not just on synthetic test fixtures.

### Coverage discipline

- `internal/cli/` gated at 100% statement coverage. The `runGitDiff` seam keeps the os/exec line testable in-process; the rest of the pipeline is pure wiring.
- `cmd/cascade/main.go` is a one-liner; not gated (Layer 2 covers it incidentally).
- Per-package gate (`scripts/coverage-check.sh`) needs a new entry for `internal/cli` at threshold 100. Carry-over from the M1 protocol: any new public/gated package gets a matching `PACKAGES` + `THRESHOLDS` entry at the same array index.

## Acceptance ledger

Per [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md). Every row reaches a final status before M5 closes.

| ID | Criterion | Verify (reproducible) | Significance | Status |
|----|-----------|----------------------|--------------|--------|
| F-1 | `internal/cli/` package exists with `doc.go` describing role | `test -f internal/cli/doc.go && head -3 internal/cli/doc.go \| grep -q '^// Package cli'` | serious | open |
| F-2 | `cli.Run` signature matches spec | `go doc github.com/geomyidia/cascade/internal/cli.Run \| grep -F 'Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int'` | serious | open |
| F-3 | `*GitDiffError` and `ErrGitDiffFailed` exported with documented fields | `go doc github.com/geomyidia/cascade/internal/cli.GitDiffError \| grep -E 'Cmd\|ExitCode\|Stderr'` and `go doc … ErrGitDiffFailed` | serious | open |
| F-4 | `cmd/cascade/main.go` is a one-liner delegating to `cli.Run` | `wc -l cmd/cascade/main.go` returns < 20 lines; `grep -q 'cli.Run' cmd/cascade/main.go` | serious | open |
| F-5 | Flag parsing covers all required + optional flags | `go test -run 'TestRun_FlagParsing' ./internal/cli` | serious | open |
| F-6 | Pipeline integration test passes against synthetic packages | `go test -run 'TestRun_PipelineIntegration' ./internal/cli` | serious | open |
| F-7 | `git diff` failure → exit 2 | `go test -run 'TestRun_GitDiffFails' ./internal/cli` | serious | open |
| F-8 | `go list` failure → exit 3 | `go test -run 'TestRun_GoListFails' ./internal/cli` | serious | open |
| F-9 | Stdin mode (`--changed-files=-`) works correctly | `go test -run 'TestRun_StdinChangedFiles' ./internal/cli` | serious | open |
| F-10 | Empty result → exit 0 | `go test -run 'TestRun_EmptyResult' ./internal/cli` | serious | open |
| F-11 | Context cancellation handled cleanly | `go test -race -run 'TestRun_ContextCancellation' ./internal/cli` | correctness | open |
| F-12 | Output is sorted and deduplicated | `go test -count=10 -run 'TestRun_PipelineIntegration' ./internal/cli` | serious | open |
| F-13 | Per-package coverage gate at 100% on `internal/cli` | `bash scripts/coverage-check.sh` | serious | open |
| F-14 | `internal/cli` has no non-stdlib imports beyond cascade's own pkg/* | `[ "$(go list -m all \| wc -l \| tr -d ' ')" = "1" ]` | correctness | open |
| F-15 | `cascade --help` prints flag reference + exit-code table | Layer-2 binary test asserts substrings: each flag name, each exit-code line | serious | open |
| F-16 | `cascade --version` prints injected ldflags metadata | Layer-2 binary test (carry-over from M1's `TestCascadeBinaryVersion`) | serious | open |
| F-17 | End-to-end pipeline against sample module | `go test -run 'TestCascadeBinaryEndToEnd' ./cmd/cascade` | serious | open |
| F-18 | `go install github.com/geomyidia/cascade/cmd/cascade@<sha>` succeeds and produces a working binary | clean GOPATH install + `cascade --help` exec | serious | open |
| F-19 | Manual sanity check on a real Go module | documented in closing report; commit SHA + commands run + first 10 lines of output (generic framing) | serious | open |
| F-20 | README updated with usage, flag reference, exit-code table | `grep -c 'exit code' README.md` returns ≥ 1 | polish | open |
| F-21 | `go doc github.com/geomyidia/cascade/internal/cli` renders cleanly | `go doc github.com/geomyidia/cascade/internal/cli \| head -30` returns useful overview | polish | open |
| F-22 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | polish | open |

22 rows. Larger than M2's 18 because M5's surface is the integration of three pure packages plus a new io edge plus user-facing CLI ergonomics. Zero deferrals expected.

## Risks & mitigations

**Signal handling correctness.** SIGINT during `go list` should kill the subprocess and exit cleanly. `exec.CommandContext` (already used by `pkg/golist`) handles the subprocess kill. cascade's `main` wires a context with `signal.NotifyContext(ctx, os.Interrupt)`. Mitigation: F-11 test uses a stub seam that blocks until the context is cancelled, asserts the function returns within a small bound. Race detector catches goroutine leaks.

**Error-code assignment ambiguity.** Some failures don't fit cleanly into 1/2/3/4. Example: stdin-read failure when `--changed-files=-` is passed. Lean: 1 (input/flag-related). Document explicitly in `--help`. Mitigation: a typed error wrapper for stdin failures gets mapped explicitly; closing report walks every error path against an exit code.

**Stdin format ambiguity.** Whitespace-only lines, leading/trailing whitespace, blank lines, CRLF — all need consistent handling. Spec: trim each line; skip empty lines; treat as a path otherwise. Mitigation: F-9 covers each whitespace edge case explicitly.

**`cli.Run` getting too big.** Wiring the pipeline plus flag handling plus stdin reading plus error mapping risks a 200-line function. Mitigation: factor into smaller helpers (`parseFlags`, `loadChangeSet`, `runPipeline`, `mapError`) — all unexported but each independently testable. Coverage gate enforces all branches.

**README drift.** The README's Library section currently shows the four-line pipeline; M5's CLI-usage section will need updating to match the actual flag set. Mitigation: F-20 test checks the README contains the exit-code table; manual review catches Library-section drift at PR time.

**`pkg/golist`'s `WithGoBin` interplay.** `cli.Run` doesn't currently expose `--go-bin`; if a downstream user wants to point at a non-default `go`, they can't without an env-var workaround (e.g., `PATH` manipulation). Defer to v0.2.

## Open questions

1. **Add `pkg/cascade.Run(opts) ([]string, error)` convenience function?** The four-line pipeline composes without adapters, but a single-call entry point is friendlier for library consumers. Lean: defer, no confirmed consumer; library users with the M2-M4 docs can compose themselves. Confirm or override.

2. **Where does `--help`'s exit-code table live in the source?** Inline in the flag-set's `Usage` function (verbose but self-contained), or externalised to a constant string (cleaner code but dual-source-of-truth with README)? Lean: inline, with a comment pointing at the README's mirror. Confirm.

3. **`--root` default = `.` or `os.Getwd()`?** Stdlib `flag` accepts `.`; `os.Getwd` returns absolute. Internal handling is consistent (we pass the value through `changeset.WithModuleRoot` which uses it directly). Lean: default `.`, document that the flag-as-default-string is interpreted relative to cwd. Confirm.

4. **`--help` output goes to stdout or stderr?** GNU convention: stdout when explicitly invoked (`--help`), stderr when triggered by parse error. Stdlib `flag` has its own conventions. Lean: follow stdlib `flag`'s convention (which goes to the FlagSet's configured output, defaulting to stderr); set output to stdout when `--help` is the explicit cause. Confirm.

5. **Context cancellation maps to which exit code?** Cancelled context isn't quite a flag error (1), git failure (2), go list failure (3), or internal error (4). Add a new exit code 5 for "cancelled/interrupted"? Or reuse 4 (internal)? Or map to the underlying io's exit code (2 if cancelled mid-git-diff, 3 if mid-go-list)? Lean: new exit code 5 for "cancelled"; documents intent and avoids confusing "internal logic error" with "user pressed Ctrl-C". Confirm.

6. **Layer 2 end-to-end smoke test against the sample module — should the test use `os.Pipe` for stdin, or `cmd.Stdin = strings.NewReader(...)`?** Both work; the latter is simpler. Lean: `cmd.Stdin = strings.NewReader(...)`. Confirm.

7. **`go install …@<sha>` verification (F-18) — same closing-evidence pattern as M1's F-12 (CC re-runs against the proxy and pastes output) or different?** Lean: same pattern; this is a regression check that the cmd/cascade entry point still installs cleanly post-pipeline-wiring. Confirm.

## Cross-references

- Parent: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md), §"M5 — CLI + main wiring".
- Predecessor M2 design: [`0003-cascade-m2-golist-adapter.md`](../06-final/0003-cascade-m2-golist-adapter.md) — sets the io-shell pattern (function-variable seam, typed errors with `Is`/`Unwrap`, `os/exec` discipline) that M5 reuses for the git-diff edge.
- Predecessor M3 design: [`0004-cascade-m3-depgraph-reverse-dep-closure.md`](../06-final/0004-cascade-m3-depgraph-reverse-dep-closure.md) — `RevDepClosure` is the final operation in the pipeline.
- Predecessor M4 design: [`0005-cascade-m4-changeset-changed-files-to-packages-mapping.md`](../05-active/0005-cascade-m4-changeset-changed-files-to-packages-mapping.md) — `Resolve` is the file-to-package mapping; M5 calls it with `WithModuleRoot` per M4's option-pattern decision.
- Methodology: [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md).
- Go canon: `go-guidelines` skill, especially `03-error-handling.md` (EH-01/04/07), `06-concurrency.md` (CC-08..12, CC-23 for signal-driven cancellation), `02-api-design.md` (API-42), `07-testing.md` (TE-01..15, TE-43), `09-anti-patterns.md` (AP-01..15), `11-documentation.md` (DC-01, DC-02 for `--help` output).
- Composition reference: M4 retro [`docs/dev/0010-implementation-retrospective-m4-changeset.md`](../../dev/0010-implementation-retrospective-m4-changeset.md), §"Closure summary" — the trio of pure packages composes without adapter code; M5 verifies this claim at the integration layer.
- Scale evidence: M2 F-18 closure measured ~2400 packages on a real corpus in ~3s for `golist.Run` alone; M5's full pipeline at the same scale is expected to add only milliseconds-to-tens-of-milliseconds for `depgraph.Build` + `changeset.Resolve` + `RevDepClosure`. Total wall-clock under 5s for a typical PR-scale invocation.
