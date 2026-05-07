# Implementation Plan: M5 — CLI + Main Wiring

**Source spec:** [`docs/design/05-active/0006-cascade-m5-cli-main-wiring.md`](../design/05-active/0006-cascade-m5-cli-main-wiring.md) (v1.0).
**Predecessor:** M4 — `pkg/changeset` (PR #9, merged at `9b010e7`).
**Successor:** M6 — `v0.1.0` release.

## Context

M5 is the integration-not-invention milestone. The pure-data trio (`pkg/golist` from M2, `pkg/depgraph` from M3, `pkg/changeset` from M4) is feature-complete. M5 wires those four-line pipeline calls behind a usable CLI binary with real flag parsing, real error handling, and a documented exit-code contract. It also adds cascade's second `os/exec` boundary (`git diff --name-only`) — sibling to M2's `go list` shell — and exposes the user-facing surface (`cascade --help`, `cascade --version`, error messages, exit codes, signal handling).

Why now: with M4 closed, the trio composes cleanly (the M4 retro confirmed at the structural level: `Resolve`'s `[]string` return is exactly `RevDepClosure`'s `seeds []string` parameter). The remaining work to ship a v0.1.0-quality CLI is integration discipline plus one new io edge — both well-precedented patterns in the cascade codebase.

The intended outcome: `cascade --tags=… --base=… --head=…` against a real Go module emits the affected-package set on stdout, exits 0 on success (including empty results), exits non-zero with a typed-error stderr message on every failure path. `cascade --help` prints the flag reference and exit-code table cleanly. `go install …@<sha>` produces a runnable binary. M2's F-18-style real-codebase sanity check (cascade against the gta target module) closes the milestone with non-empty, fully-populated output.

## Decisions resolved

The spec author left seven open questions. All seven are resolved here per the spec's leans; surfaces are flagged for user override during plan review. The two material questions (Q1 — public convenience function; Q5 — cancellation exit code) are noted explicitly as "lean applied; override candidates."

| # | Question | Decision |
|---|---|---|
| Q1 | Add `pkg/cascade.Run(opts) ([]string, error)` convenience function? | **Defer.** No confirmed library consumer; the four-line pipeline composes without adapters. Re-evaluate at v0.2 if a real consumer surfaces. **Material** — flag for user override if they want it sooner. |
| Q2 | `--help`'s exit-code table inline vs externalised? | **Inline** in the flag-set's `Usage` function. Cost: ~30 lines of constant strings in the CLI source; benefit: single source of truth for what the binary actually prints. README mirrors via copy with a `// keep in sync with cascade --help` comment. |
| Q3 | `--root` default value | **`.`** (the literal string). Documented in `--help` as "interpreted relative to cwd by `filepath.Abs` before passing to `golist.Run` and `changeset.WithModuleRoot`." Empty-default-equals-cwd magic avoided. |
| Q4 | `--help` output to stdout vs stderr | **Stdlib `flag` convention applied.** When `--help` is the explicit cause, the FlagSet's output is set to stdout (GNU convention). When help is triggered by a parse error, output goes to stderr (stdlib `flag`'s default). |
| Q5 | Cancellation exit code | **New exit code 5: cancelled/interrupted.** Documents intent (user pressed Ctrl-C, or parent killed cascade) without conflating with internal logic errors (4). The exit-code table in §"CLI flag set" gains a fifth row. **Material** — flag for user override if they want to reuse 4 instead. |
| Q6 | Layer 2 stdin shape | **`cmd.Stdin = strings.NewReader(...)`.** Simpler than `os.Pipe` and equally testable. |
| Q7 | F-18 (`go install …@<sha>`) verification pattern | **Same as M1's F-12.** CC re-runs `go install` against `proxy.golang.org` post-merge, captures the resulting binary's `--version` output, documents in the closing retrospective. Confirms the install path remains clean post-pipeline-wiring. |

## Implementation approach

### File layout (post-M5)

```
cmd/cascade/
├── main.go              Modified: shrunk to <20 lines; delegates to cli.Run
└── main_test.go         Modified: M1 in-process `run()` tests retired; replaced with delegation smoke + Layer-2 binary tests

internal/cli/            NEW — package cli
├── doc.go               Package overview + role + exit-code table reference
├── cli.go               Run + parseFlags + loadChangeSet + runPipeline + mapError + Version/Help printers
├── errors.go            GitDiffError struct + ErrGitDiffFailed sentinel + Error/Is/Unwrap methods
├── seam.go              runGitDiff function-variable seam + defaultRunGitDiff + gitDiffResult type
├── cli_test.go          Layer-1 public-API tests (package cli_test, per TE-43)
└── seam_test.go         Layer-1 internal tests (package cli, for runGitDiff seam) + withGitDiffSeam helper
```

`cli.go` is the largest file (~250–350 lines including comments); `errors.go` is ~50 lines; `seam.go` is ~60 lines. Split decision is structural — same shape as `pkg/golist/{golist.go, errors.go, parse.go, seam_test.go}`.

If `cli.go` exceeds ~400 lines once written, factor `parseFlags` into `flags.go`. Decide at write time, not pre-emptively.

### Public API surface — `internal/cli`

```go
package cli

// Run is the testable entry point for the cascade CLI. ... [full doc per spec]
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int

// GitDiffError captures the diagnostic context when `git diff` exits non-zero.
type GitDiffError struct {
    Cmd      []string
    ExitCode int
    Stderr   string
    underlying error // unexported; reachable via Unwrap
}

func (e *GitDiffError) Error() string
func (e *GitDiffError) Is(target error) bool
func (e *GitDiffError) Unwrap() error

// ErrGitDiffFailed is the sentinel for non-zero `git diff` exit.
var ErrGitDiffFailed = errors.New("git diff failed")
```

Mirrors `pkg/golist`'s `*ExitError` + `ErrGoListFailed` exactly. `Is(target)` returns true for both `target == ErrGitDiffFailed` (via local check) and via the underlying error chain (`errors.Is(e.underlying, target)`). The `underlying` field carries the `*exec.ExitError` for callers who want to walk further.

### Internal helpers — unexported

```go
// config holds the parsed flag values plus pipeline-step inputs.
type config struct {
    tags          []string
    base          string
    head          string
    changedFiles  string  // path or "-" for stdin
    root          string
    showVersion   bool
    showHelp      bool
}

// parseFlags parses args using a fresh FlagSet wired to stderr/stdout per the
// help-output convention. Returns config + error; flag.ErrHelp is handled
// explicitly to drive --help to stdout.
func parseFlags(args []string, stdout, stderr io.Writer) (*config, error)

// loadChangeSet returns the list of changed file paths from one of three
// sources, in priority order:
//   1. --changed-files=<path>: read newline-separated entries from the file
//   2. --changed-files=-: read newline-separated entries from stdin
//   3. otherwise: invoke runGitDiff(ctx, base, head, root) and parse its stdout
// Lines are trimmed; empty lines after trim are skipped.
func loadChangeSet(ctx context.Context, cfg *config, stdin io.Reader) ([]string, error)

// runPipeline composes golist.Run + depgraph.Build + changeset.Resolve +
// g.RevDepClosure and returns the affected-package list. Pure assembly;
// no side effects beyond what the four primitives produce.
func runPipeline(ctx context.Context, cfg *config, changedFiles []string) ([]string, error)

// mapError maps any error from parseFlags / loadChangeSet / runPipeline to the
// process exit code per the M5 contract. Unmapped errors return 4 (internal).
func mapError(err error) int
```

Each helper is independently table-test-able. Coverage gate forces every branch.

### Pipeline algorithm

```
Run(args, stdin, stdout, stderr) int:
    ctx := signalNotifyContext()                  // SIGINT, SIGTERM
    defer cancel()

    cfg, err := parseFlags(args, stdout, stderr)
    if err != nil:
        return mapError(err)                      // 1 for parse errors

    if cfg.showVersion:
        printVersion(stdout)
        return 0

    if cfg.showHelp:
        printHelp(stdout)
        return 0

    changedFiles, err := loadChangeSet(ctx, cfg, stdin)
    if err != nil:
        fmt.Fprintln(stderr, err)
        return mapError(err)                      // 2 for git diff failures

    affected, err := runPipeline(ctx, cfg, changedFiles)
    if err != nil:
        fmt.Fprintln(stderr, err)
        return mapError(err)                      // 3 for go list failures, 5 for cancellation

    for _, path := range affected:
        fmt.Fprintln(stdout, path)
    return 0
```

All io errors flow through `mapError` for exit-code translation. `runPipeline`'s body is the four-line pipeline directly:

```go
pkgs, err := golist.Run(ctx, cfg.tags, []string{"./..."}, golist.WithDir(cfg.root))
if err != nil { return nil, err }
g := depgraph.Build(pkgs)
seeds := changeset.Resolve(changedFiles, pkgs, changeset.WithModuleRoot(cfg.root))
return g.RevDepClosure(seeds), nil
```

### Git-diff seam (mirrors M2's `runGoList`)

```go
// gitDiffResult carries the subprocess outcome through the seam.
type gitDiffResult struct {
    stdout io.Reader
    err    error
    stderr string
}

// runGitDiff is the function-variable seam over the git-diff exec.
// Production builds use defaultRunGitDiff; tests replace it via
// withGitDiffSeam to drive specific branches without spawning a subprocess.
var runGitDiff = defaultRunGitDiff

// defaultRunGitDiff shells out to `git diff --name-only <base>..<head>` in
// the configured working directory and captures stdout/stderr.
//
// gosec G204 (subprocess from variable) is suppressed here for the same
// reason as pkg/golist's defaultRunGoList: argv[0] is "git" (a constant);
// argv[1:] is constructed from caller-supplied refs that are documented as
// caller-controlled.
func defaultRunGitDiff(ctx context.Context, base, head, dir string) gitDiffResult {
    cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", base+".."+head) //nolint:gosec
    cmd.Dir = dir
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    err := cmd.Run()
    return gitDiffResult{stdout: &stdout, err: err, stderr: stderr.String()}
}

// classifyGitDiffError converts an exec.Cmd.Run error into a *GitDiffError or
// a context-cancellation error. Mirrors pkg/golist's classifyRunError.
func classifyGitDiffError(err error, argv []string, dir, stderr string, ctx context.Context) error {
    if ctxErr := ctx.Err(); ctxErr != nil {
        return fmt.Errorf("git diff: %w", ctxErr)
    }
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        return &GitDiffError{
            Cmd:        append([]string(nil), argv...),
            ExitCode:   exitErr.ExitCode(),
            Stderr:     stderr,
            underlying: exitErr,
        }
    }
    return fmt.Errorf("%w: %w", ErrGitDiffFailed, err)
}
```

Test-side helper in `seam_test.go` (internal package):

```go
func withGitDiffSeam(t *testing.T, fn func(ctx context.Context, base, head, dir string) gitDiffResult) {
    t.Helper()
    saved := runGitDiff
    t.Cleanup(func() { runGitDiff = saved })
    runGitDiff = fn
}
```

### Context + signal handling

```go
// In Run, near the top:
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer cancel()
```

`signal.NotifyContext` (Go 1.16+) wires a context that's cancelled when the binary receives SIGINT or SIGTERM. The `cancel()` defer ensures the signal handler is unregistered before `Run` returns. `pkg/golist`'s `exec.CommandContext` already kills the subprocess on context cancellation; the same applies to `runGitDiff`'s `defaultRunGitDiff`. F-11 verifies this: a stub seam that blocks on `<-ctx.Done()` and returns a context-cancellation result.

### Existing patterns reused

- **M2's `runGoList` seam** — exact structural mirror for `runGitDiff`. File-by-file:
  - `pkg/golist/golist.go:96-100` (seam variable declaration) → `internal/cli/seam.go:runGitDiff` declaration.
  - `pkg/golist/golist.go:110-123` (`defaultRunGoList`) → `defaultRunGitDiff` (different argv shape but identical structure).
  - `pkg/golist/golist.go:90-94` (`runResult` struct) → `gitDiffResult` (3 fields, same shape).
  - `pkg/golist/seam_test.go:11-17` (`withSeam` helper) → `withGitDiffSeam` in `seam_test.go`.
- **M2's `classifyRunError`** (`pkg/golist/golist.go:205-229`) — algorithm mirror for `classifyGitDiffError`: ctx-error first, exec.ExitError extraction second, fall-through wrapping last.
- **M2's `*ExitError` shape** (`pkg/golist/errors.go`) — exact mirror for `*GitDiffError` (Cmd + ExitCode + Stderr + underlying; Error/Is/Unwrap methods).
- **M1's `func run(args, stdout, stderr) int` indirection** (`cmd/cascade/main.go:15-55`) — preserved; M5 just changes what `run()` delegates to. The M1 binary-test pattern (build with ldflags, exec, assert) is preserved verbatim for the Layer-2 test in M5.
- **M3 + M4's table-driven test layout with inline `sort.StringsAreSorted` assertions** — applied to the integration tests for F-12 (sorted/dedup-output).
- **M3 retro's "What Worked" insight: bundle plan-mode user decisions as Decisions resolved with explicit override flags.** Applied above for Q1 and Q5.

## Test strategy

Three layers, owning distinct coverage bands.

### Layer 1 — `internal/cli` unit tests (in-process; coverage-gated to 100%)

**Public-API tests in `cli_test.go` (package `cli_test`):**

| Test function | Cases | Spec ledger row |
|---|---|---|
| `TestRun_Version` | `--version` prints metadata to stdout, exit 0 | F-16 (binary) + Layer-1 |
| `TestRun_Help` | `--help` prints usage to stdout including all flags + exit-code table; exit 0 | F-15 (binary) + Layer-1 |
| `TestRun_FlagParsing` | missing required flag, unknown flag, --base without --head, etc. → exit 1 | F-5 |
| `TestRun_PipelineIntegration` | synthetic golist seam returns sample-module pkgs; assert affected-set on stdout | F-6, F-12 |
| `TestRun_StdinChangedFiles` | `--changed-files=-` reads stdin, trims whitespace, skips empty lines | F-9 |
| `TestRun_FileChangedFiles` | `--changed-files=<tmpfile>` reads from a file | F-9 (sibling) |
| `TestRun_GitDiffFails` | seam returns `*GitDiffError`; exit 2; stderr contains captured git stderr | F-7 |
| `TestRun_GoListFails` | golist seam returns `*golist.ExitError`; exit 3 | F-8 |
| `TestRun_EmptyResult` | no .go files in change-set; empty stdout; exit 0 | F-10 |
| `TestRun_ContextCancellation` | seam blocks on ctx.Done; signal cancellation; exit 5 | F-11 |
| `TestRun_OutputSortedAndDeduped` | unsorted/duplicate seeds → output sorted + deduped (covered by F-6 inline assertion) | F-12 |

**Internal seam tests in `seam_test.go` (package `cli`):**

| Test function | Cases |
|---|---|
| `TestRunGitDiff_HappyPath` | seam returns stdout = "pkga/a.go\npkgb/b.go\n", err = nil → loaded change-set matches |
| `TestRunGitDiff_NonZeroExit` | seam returns *exec.ExitError with stderr → classifyGitDiffError yields *GitDiffError |
| `TestRunGitDiff_ContextCancelled` | seam returns ctx.Err() → classifyGitDiffError wraps with %w |
| `TestRunGitDiff_OtherExecError` | seam returns synthetic non-ExitError → classifyGitDiffError wraps with ErrGitDiffFailed |
| `TestGitDiffError_IsAndUnwrap` | errors.Is(err, ErrGitDiffFailed) true; errors.As(err, &ge) extracts; e.Unwrap() reaches *exec.ExitError |

Each table-driven test inline-asserts `sort.StringsAreSorted(out)` where applicable (F-12 inline-protection per M3 retro carry-forward).

### Layer 2 — end-to-end binary smoke (subprocess; not coverage-counted)

Single test in `cmd/cascade/main_test.go`, extending M1's existing `TestCascadeBinaryVersion`:

`TestCascadeBinaryEndToEnd`:
1. Skip under `testing.Short()`.
2. Build the cascade binary with `-ldflags` injecting test version metadata (carry-over from M1).
3. Invoke the binary against `pkg/golist/testdata/sample-module/` with synthetic stdin (echo of changed files via `cmd.Stdin = strings.NewReader(...)` per Q6) and a synthetic tag set.
4. Assert: exit code 0, stdout contains the expected affected packages (`example.test/sample/pkga` etc.), stderr is clean.

Confirms the build → exec → wire → output chain works end-to-end against real `go list`. Coverage-incidental.

### Layer 3 — manual sanity check (not automated, in closing report)

M2's F-18 pattern. Invoke merged-to-`main` cascade against the gta target module:
1. Capture commit SHA, command run, exit code, wall-clock time, first ~10 lines of stdout (generic-framed per the M2 retro convention).
2. Document in M5 closing retrospective with a "non-empty, fully-populated affected-set output observed at production scale" attestation.

This is the load-bearing closure for M5 — proves the integrated pipeline solves the original problem on a real codebase, not just on synthetic tests. Mirrors M2 F-18's "the gta failure mode is structurally absent" attestation.

### Coverage discipline

- **`internal/cli/` gated at 100%** statement coverage. The `runGitDiff` seam keeps the os/exec line testable in-process; the rest is pure wiring.
- **`cmd/cascade/main.go` not gated** — one-liner; Layer 2 covers it incidentally.
- **`scripts/coverage-check.sh` PACKAGES gets a new entry** at index 4 for `internal/cli` with threshold 100. THRESHOLDS array gets a matching `100`. Comment block updated to mention `internal/cli/  100  (M5 wiring)`.

## Implementation order

1. **Pre-flight reading.** Refresh on the spec's load list against the substrate already in context. Confirm:
   - 03-error-handling EH-01/04/07/08/15/16/18 (refreshed via Phase-1 Explore).
   - 06-concurrency CC-08/09/10/11/12/13/31 (refreshed).
   - 09-anti-patterns AP-01..15 (refreshed).
   - 02-api-design API-42 (already in context from M3/M4).
   - 07-testing TE-01..15, TE-43 (already in context).
   - 11-documentation DC-01, DC-02 (already in context).

2. **Branch `m5/cli-main-wiring` off post-M4-merge `main`.**

3. **Skeleton commit (no-op):** create `internal/cli/{doc.go, cli.go, errors.go, seam.go}` as empty stubs with package declarations + `// TODO m5` markers. Verify `go build ./...` passes (the empty package compiles). This isolates the directory-creation churn from the implementation churn.

4. **Implement `internal/cli/errors.go`** — `GitDiffError` struct + `ErrGitDiffFailed` + `Error/Is/Unwrap` methods. Mirror `pkg/golist/errors.go`'s `*ExitError` shape.

5. **Implement `internal/cli/seam.go`** — `gitDiffResult`, `runGitDiff = defaultRunGitDiff`, `defaultRunGitDiff`, `classifyGitDiffError`. Mirror `pkg/golist/golist.go`'s seam patch (lines 96-123) + `classifyRunError` (lines 205-229).

6. **Implement `internal/cli/cli.go`** — `config`, `parseFlags`, `loadChangeSet`, `runPipeline`, `mapError`, `printVersion`, `printHelp`, and `Run` (the orchestrator). Wire `signal.NotifyContext` for SIGINT/SIGTERM cancellation.

7. **Refactor `cmd/cascade/main.go`** — shrink to <20 lines: `os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))` with the `Version`/`GitCommit`/etc. ldflags-target vars unchanged (still consumed by `internal/project`).

8. **Refactor `cmd/cascade/main_test.go`** — retire the M1 in-process `run()` tests (those now live in `internal/cli/cli_test.go`). Preserve `TestCascadeBinaryVersion` as the existing F-16 reference. Add `TestCascadeBinaryEndToEnd` for F-17.

9. **Write `internal/cli/cli_test.go`** (Layer 1 public API).

10. **Write `internal/cli/seam_test.go`** (Layer 1 internal seam) with `withGitDiffSeam` helper.

11. **Update `scripts/coverage-check.sh`** — add `internal/cli` entry at index 4; THRESHOLDS array gets a matching `100`. Update comment block.

12. **Update `README.md`** — CLI usage section gets the full flag reference + exit-code table; Library section already documents the four-line pipeline (no change). The exit-code table is the single F-20 verify target.

13. **Update `internal/cli/doc.go`** — package doc with role + a one-line pointer to the exit-code table.

14. **Local verification:**
    - `make tidy` (no-op expected)
    - `make format` (gofmt + goimports clean)
    - `make lint` (cold cache via OUT-1)
    - `go test -race -count=1 ./...` (all packages green)
    - `go test -race -count=10 ./internal/cli/...` (F-11 + F-12 + F-7..10)
    - `bash scripts/coverage-check.sh` (F-13)
    - `make build && ./bin/cascade --help` (F-15 substring assertion possible)
    - `make build && ./bin/cascade --version` (F-16)
    - `make check-all` (full bundle)

15. **Pre-PR self-check** on each ledger row F-1..F-22 (M3-retro carry-forward).

16. **Commit + push + PR.** Per CLAUDE.md, push and PR-open are public-facing actions; pause for user confirmation.

17. **Layer 3 manual sanity check** — captured in M5 closing retrospective.

18. **F-18 (`go install …@<sha>`) verification** — post-merge, after the PR closes; CC re-runs against `proxy.golang.org` and pastes output into a follow-up retrospective entry.

## Critical files

| Path | Action | Notes |
|---|---|---|
| `internal/cli/doc.go` | Create | Package overview + exit-code table reference. |
| `internal/cli/cli.go` | Create | Run + parseFlags + loadChangeSet + runPipeline + mapError + Version/Help printers. |
| `internal/cli/errors.go` | Create | `*GitDiffError` + `ErrGitDiffFailed` + Error/Is/Unwrap. Mirrors `pkg/golist/errors.go`. |
| `internal/cli/seam.go` | Create | `gitDiffResult`, `runGitDiff` seam, `defaultRunGitDiff`, `classifyGitDiffError`. |
| `internal/cli/cli_test.go` | Create | Layer-1 public-API tests; package `cli_test` (TE-43). |
| `internal/cli/seam_test.go` | Create | Layer-1 internal seam tests; package `cli`; `withGitDiffSeam` helper. |
| `cmd/cascade/main.go` | Modify | Shrink to <20 lines; delegate to `cli.Run`. |
| `cmd/cascade/main_test.go` | Modify | Retire M1 in-process `run()` tests; preserve `TestCascadeBinaryVersion`; add `TestCascadeBinaryEndToEnd`. |
| `scripts/coverage-check.sh` | Modify | Add `internal/cli` entry to PACKAGES + matching THRESHOLDS at index 4; update comment block. |
| `README.md` | Modify | CLI usage section: add full flag reference + exit-code table. Library section unchanged. |
| `docs/dev/0012-implementation-retrospective-m5-cli-main-wiring.md` | Create at close | Closing report; F-1..F-22 row walk; Layer-3 sanity-check evidence; F-18 post-merge install verification follow-up. |

`CLAUDE.md` and `CONTRIBUTING.md` need no changes — both already cover `internal/` packages structurally.

## Verification (mapping to spec ledger F-1..F-22)

| ID | Verify | Planned evidence |
|---|---|---|
| F-1 | `test -f internal/cli/doc.go && head -3 \| grep '^// Package cli'` | doc.go starts with `// Package cli` |
| F-2 | `go doc …/internal/cli.Run \| grep -F 'Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int'` | Exact match |
| F-3 | `go doc …/internal/cli.GitDiffError` shows `Cmd`/`ExitCode`/`Stderr`; `go doc …/internal/cli.ErrGitDiffFailed` exists | Both godoc'd |
| F-4 | `wc -l cmd/cascade/main.go` < 20; `grep 'cli.Run' cmd/cascade/main.go` returns the delegation line | One-liner shape verified |
| F-5 | `go test -run TestRun_FlagParsing ./internal/cli` | Subtests for each flag-error case pass |
| F-6 | `go test -run TestRun_PipelineIntegration ./internal/cli` | Synthetic seam returns sample-module pkgs; affected-set on stdout |
| F-7 | `go test -run TestRun_GitDiffFails ./internal/cli` | Exit 2 + stderr contains git stderr |
| F-8 | `go test -run TestRun_GoListFails ./internal/cli` | Exit 3 |
| F-9 | `go test -run TestRun_StdinChangedFiles ./internal/cli` | Stdin lines trimmed; empties skipped |
| F-10 | `go test -run TestRun_EmptyResult ./internal/cli` | Empty stdout; exit 0 |
| F-11 | `go test -race -run TestRun_ContextCancellation ./internal/cli` | Cancellation returns within bounded time; race detector clean; exit 5 |
| F-12 | `go test -count=10 -run TestRun_PipelineIntegration ./internal/cli` | 10 runs identical output (sorted + deduped) |
| F-13 | `bash scripts/coverage-check.sh` | `ok: github.com/geomyidia/cascade/internal/cli coverage 100% >= 100%` |
| F-14 | `go list -m all \| wc -l` returns 1 | Module-only; no external deps |
| F-15 | Layer-2 binary test asserts substrings: each flag name + each exit-code line | `cascade --help` output contains all flag names + exit-code table |
| F-16 | Layer-2 binary test (carry-over from M1's `TestCascadeBinaryVersion`) | `cascade --version` output contains injected ldflags metadata |
| F-17 | `go test -run TestCascadeBinaryEndToEnd ./cmd/cascade` | Sample-module integration green |
| F-18 | Post-merge: `go install …@<sha>` + `cascade --help` | Install + binary functional |
| F-19 | Layer-3 manual sanity check, in closing retro | Real-codebase output captured (generic-framed) |
| F-20 | `grep -c 'exit code' README.md` returns ≥ 1 | Exit-code table present in README |
| F-21 | `go doc …/internal/cli \| head -30` | Useful overview; all exports documented |
| F-22 | reviewer reads closing report | Substrate enumeration with cited pattern IDs |

## Risks & mitigations

- **Signal handling correctness.** Per spec. Mitigation: F-11 test stub seam blocks on `<-ctx.Done`; assert return within bounded time. Race detector catches goroutine leaks. The `signal.NotifyContext` + `defer cancel()` pattern is stdlib-canonical and avoids the goroutine-leak failure mode of manual `signal.Notify`.

- **Error-code assignment ambiguity** (stdin-read failure when `--changed-files=-`). Per spec. Mitigation: stdin-read failure maps to exit 1 (input/flag-related); typed wrapper makes the mapping explicit; mapError walks every typed error against an exit code; closing report walks the mapping table once.

- **Stdin format ambiguity** (whitespace, CRLF, blanks). Per spec. Mitigation: F-9 covers each whitespace edge case explicitly. `bufio.Scanner` with default behaviour handles CRLF; explicit `strings.TrimSpace` handles other whitespace; empty post-trim → skip.

- **`cli.Run` getting too big.** Per spec. Mitigation: factor into `parseFlags`, `loadChangeSet`, `runPipeline`, `mapError` — each independently testable. If the file still exceeds ~400 lines after the factoring, split `parseFlags` into `flags.go`. Decide at write time, not pre-emptively.

- **README drift between `--help` output and CLI section.** Per spec. Mitigation: F-20 verifies the README contains the exit-code table; F-15 verifies the binary's `--help` output contains the same; manual review at PR time catches drift in the Library section. Add a `// keep in sync with README.md` comment near the inline help-text constant in `cli.go`.

- **Cancellation exit-code design (Q5 lean).** Adding exit code 5 commits to the contract. If the user prefers reusing 4 (internal logic error) for cancellation, the change is a one-line `mapError` swap and one F-11 test assertion update. Cheap to revisit during plan review or PR review.

- **Convenience `pkg/cascade.Run` deferral (Q1 lean).** Closing the v0.x window without a single-call entry point may surface friction for early library consumers. Mitigation: the README Library section's four-line pipeline is the documented composition; the M2-M4 retros confirm the trio composes without adapters; if a real library consumer surfaces during M6 release prep, adding `pkg/cascade.Run` is a non-breaking v0.2 addition.

- **Layer 3 sanity check depends on a private codebase.** Per M2 F-18 convention. Mitigation: generic-framed evidence in the closing retro (matches M2's `<thousands>` / `<hundreds>` placeholders); precise figures stay private. CDC review verifies the public PR comment text contains no fingerprinting.

- **Lint-cache drift recurrence.** OUT-1 (M3) is in place; M5 inherits the protection. No new mitigation needed.

## Out of scope (per spec + plan additions)

- `pkg/cascade.Run(...)` convenience function (Q1 deferred).
- Caching the dep graph; watch mode; daemon; configuration files; alternative VCS; concurrent multi-`go-list` invocations; plugins; build-tag inference; custom output formats. (All per spec.)
- `--go-bin` flag exposing `pkg/golist.WithGoBin`. (Per spec; defer to v0.2.)
- A `--quiet` flag suppressing all stderr output. (Plan addition: not specced; CI users typically want the diagnostic stream.)
- Auto-detection of `--base`/`--head` from the current branch (e.g., `--base=$(git merge-base HEAD origin/main)`). (Plan addition: out-of-scope; callers compose this in their CI workflow scripts.)

## Open items deferred to closing retrospective

- Final pre-PR CC self-check on F-1..F-22 evidence-vs-criterion text (M3-retro carry-forward).
- Layer 3 manual sanity check evidence on a real Go module (F-19).
- Post-merge `go install …@<sha>` verification (F-18).
- Whether the cancellation-exit-code-5 decision (Q5) ended up confusing in practice or paid off as documentation. Worth an explicit retro line.
- Whether the four-line pipeline in `runPipeline` actually stays a four-liner, or whether the integration surfaced any adapter-need that the M4 retro's "composes without adapter code" claim missed.

## Carry-forward expected to land in M5 retrospective

- **Second-use of the `runX` seam pattern.** M4 used `getCwd` (M2's mechanism, second occurrence); M5 uses `runGitDiff` (third occurrence). The pattern is now mature across three milestones. Future packages with io edges should reach for it by default; no further documentation needed.

- **Trio-composition claim verified at the integration layer.** The M4 retro claimed "the trio composes without adapter code"; M5's `runPipeline` four-liner is the structural verification. Carry forward only if any adapter-need surfaced — otherwise the claim is settled.

- **CC self-check protocol track record (5-for-5 if M5 closes clean).** M3, refactor, M4 each closed without softpedals at audit. M5 is the first non-pure-data milestone; the audit's first stress test against integration-shaped scope. Worth recording the outcome explicitly.

- **Layer 3 sanity-check pattern's ongoing utility.** M2 F-18 observed the gta failure mode is structurally absent on a real codebase. M5 F-19 is the integrated-pipeline analogue: does the full cascade-against-real-codebase invocation match the synthetic-test predictions? Worth a brief retro entry on whether real-codebase scale (M2's 2422 packages) revealed any cli-layer surprises (e.g., `bufio.Scanner` token-size limits at unusual git-diff outputs).

- **`pkg/cascade.Run` deferred-or-needed status.** Q1's lean was "defer." If the M5 retrospective has any user reporting friction with the four-line composition, that's the signal to add `pkg/cascade.Run` in v0.2. Document either way.

- **Cancellation-exit-code-5 outcome.** Q5's lean was "add exit code 5." If shell-callers find it intuitive (`cascade && pipeline-step` semantics with explicit cancellation surface), that's a win; if they find 5 confusing relative to the 1-4 cluster, that's a candidate for v0.2 rollback.

## Cross-references

- Source spec: [`docs/design/05-active/0006-cascade-m5-cli-main-wiring.md`](../design/05-active/0006-cascade-m5-cli-main-wiring.md).
- Predecessor M4 retro: [`docs/dev/0010-implementation-retrospective-m4-changeset.md`](./0010-implementation-retrospective-m4-changeset.md) — §"Closure summary" claims the trio composes without adapter code; M5's `runPipeline` is the structural verification.
- M2 patterns reused: `pkg/golist/golist.go:90-123` (seam pattern + result type + default impl), `pkg/golist/seam_test.go:11-17` (`withSeam` helper), `pkg/golist/errors.go` (`*ExitError` shape). Plus `pkg/golist/golist.go:205-229` (`classifyRunError` algorithm).
- M1 pattern preserved: `cmd/cascade/main.go:15-55` (`func run()` indirection); `cmd/cascade/main_test.go` (binary smoke + ldflags injection pattern, carried into M5's Layer 2).
- Methodology: [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md).
- Go canon refreshed at session start: 03-error-handling (EH-01/04/07/08/15/16/18), 06-concurrency (CC-08..13, CC-31), 09-anti-patterns (AP-01..15), 02-api-design (API-42), 07-testing (TE-01..15, TE-43), 11-documentation (DC-01, DC-02).

## Surfaces flagged for user override during plan review

- **Q1 (`pkg/cascade.Run` defer-or-add).** Lean applied: defer. If you want it sooner, the addition is ~30 lines in a new `pkg/cascade/cascade.go` file plus matching tests; non-blocking on M5's other ledger rows.
- **Q5 (cancellation exit code 5).** Lean applied: add. If you'd prefer reusing 4, a one-line `mapError` swap and an F-11 test-assertion update.

Both flags are low-cost to flip during PR review if the implementation surfaces a reason to.
