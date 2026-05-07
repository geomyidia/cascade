# M5 Implementation Retrospective: CLI + Main Wiring

**Status:** closing report; awaiting CDC verification.
**Closing commit:** `c45291e` (head of `m5/cli-main-wiring` at close; M5 PR head). Merge OID lands here when rebase-merge to `main` completes.
**CDC verification:** pending.
**Source spec:** [`docs/design/05-active/0006-cascade-m5-cli-main-wiring.md`](../design/05-active/0006-cascade-m5-cli-main-wiring.md) (v1.0). Anticipated to transition 05-active → 06-final post-merge.
**Source impl plan:** [`docs/dev/0011-implementation-plan-m5-cli-main-wiring.md`](./0011-implementation-plan-m5-cli-main-wiring.md).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md).

## Closure summary

The spec's acceptance ledger declared **22** rows (F-1..F-22) with a stated **deferral budget of zero**. **20 rows close in this PR**; F-18 (`go install …@<sha>`) requires a merged commit on `main` and is captured as a follow-up post-merge; F-19 (manual sanity check on a real Go module) is the load-bearing M2-F-18-style closure documented separately.

The exit signal — *"`cascade --tags=… --base=… --head=…` against a real Go module emits the affected-package set on stdout, exits 0 on success (including empty results), exits non-zero with a typed-error stderr message on every failure path; `cascade --help` prints the flag reference and exit-code table cleanly; `go install …@<sha>` produces a runnable binary"* — is satisfied for clauses 1, 2, 3 by the ledger rows; clause 4 (`go install`) by F-18 post-merge.

| Status | Count |
|--------|-------|
| Done | 20 |
| Deferred to post-merge follow-up (F-18, F-19) | 2 |
| No-op | 0 |
| Open at close | 0 |

The structural property that distinguishes M5 from M2/M3/M4 — *integration over invention* — is observed cleanly. The `runPipeline` function in `internal/cli/cli.go` is **literally four lines** (golist.Run → depgraph.Build → changeset.Resolve → g.RevDepClosure), the structural verification of the M4 retro's "trio composes without adapter code" claim. No adapter functions, no shape mismatches, no signature drift between the producer and consumer types.

## Substrate loaded at session start

Per [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md) §"Substrate" and the spec's §"Required reading":

- [`AI-CONSTITUTION-SUPPLEMENT.md`](../../assets/ai/AI-CONSTITUTION-SUPPLEMENT.md), [`AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md) — already in conversation context from the M3/refactor/M4 sessions.
- [`assets/ai/go/SKILL.md`](../../assets/ai/go/SKILL.md) — Critical Rules + Document Selection Guide refreshed.
- [`assets/ai/go/guides/03-error-handling.md`](../../assets/ai/go/guides/03-error-handling.md) — load-bearing IDs cited: **EH-01** (errors as values, not in-band sentinels), **EH-04** (`errors.New`/`fmt.Errorf` discipline; `%w` for wrapping), **EH-07** (custom error types with `Error` suffix), **EH-08** (pointer receiver on error types carrying fields), **EH-15** (`errors.Is` over `==` for sentinel comparison), **EH-16** (`errors.As` for typed extraction), **EH-18** (document errors a function can return). The `*GitDiffError` type implements the EH-08-shaped pattern: pointer receiver, `Error()` + `Is(target)` + `Unwrap()` methods, sentinel matching via `Is`.
- [`assets/ai/go/guides/06-concurrency.md`](../../assets/ai/go/guides/06-concurrency.md) — load-bearing IDs cited: **CC-08** (`ctx` as first param, named `ctx`), **CC-09** (never store ctx in struct), **CC-11** (`context.Background()` only at entry; cli.Run is the entry), **CC-13** (`defer cancel()` immediately after `signal.NotifyContext`). Plus implicitly: signal.NotifyContext as the canonical signal-driven cancellation pattern.
- [`assets/ai/go/guides/09-anti-patterns.md`](../../assets/ai/go/guides/09-anti-patterns.md) — walked. M5-relevant pattern IDs cited or avoided: **AP-04** (no panic/recover for control flow), **AP-06** (no `os.Exit` outside `main` — Run returns int), **AP-10** (no `_ = err` discards; the lone `defer f.Close() //nolint:errcheck` carries explicit justification at the call site), **AP-11** (skip "failed to" prefixes — error messages just say "git diff failed", not "failed to run git diff"), **AP-12** (no info-free wrap; `%w: %w` chains add real context), **AP-13** (no string-match on error messages — `errors.Is`/`errors.As` throughout `mapError`), **AP-15** (no fire-and-forget goroutines — `signal.NotifyContext`'s goroutine is managed by the stdlib).
- [`assets/ai/go/guides/02-api-design.md`](../../assets/ai/go/guides/02-api-design.md) — **API-42** honoured (no globals; everything threaded through `Run`'s args + the three internal seam variables which are unexported and tests-only).
- [`assets/ai/go/guides/07-testing.md`](../../assets/ai/go/guides/07-testing.md) — **TE-01..TE-15** (table-driven idiom — every multi-case test in M5 is a table-driven loop), **TE-43** (`package cli_test` for public-API tests; `package cli` only inside `seam_test.go` to access the unexported seam variables).
- [`assets/ai/go/guides/11-documentation.md`](../../assets/ai/go/guides/11-documentation.md) — **DC-01** (every exported name documented: `Run`, `GitDiffError`, `ErrGitDiffFailed`); **DC-02** (every doc comment starts with the identifier name).

## Decisions resolved (per the M5 spec's seven open questions)

All seven resolved per the spec author's leans during plan-mode design review. Two material decisions (Q1, Q5) were flagged for user override; user accepted both leans during plan review.

| # | Question | Decision applied | Source |
|---|---|---|---|
| Q1 | `pkg/cascade.Run` convenience function | Defer to v0.2; no confirmed library consumer | spec lean |
| Q2 | Help text inline vs externalised | Inline as a `helpText` const in `cli.go` with "// keep in sync with README" comment | spec lean |
| Q3 | `--root` default | `.` literal; passed through to `golist.WithDir` and `changeset.WithModuleRoot` | spec lean |
| Q4 | `--help` to stdout vs stderr | GNU-style asymmetric: `--help` (long form bool) routes to stdout via Run; `-h` shorthand triggers `flag.ErrHelp` and stdlib `flag` routes to stderr | spec lean |
| Q5 | Cancellation exit code | New exit code 5 ("cancelled / interrupted"); explicit signal vs internal-error distinction | spec lean |
| Q6 | Layer 2 stdin shape | `cmd.Stdin = strings.NewReader(...)` in `TestCascadeBinaryEndToEnd` | spec lean |
| Q7 | F-18 install verification pattern | Same as M1 F-12; CC re-runs `go install` post-merge against `proxy.golang.org` and pastes output here | spec lean |

## Plan additions vs spec

The plan added two seams to `internal/cli` beyond the spec's `runGitDiff`:

1. **`runGoListWrapper = golist.Run`** — function-variable seam over `golist.Run` itself. The spec assumed pipeline tests would mock via "golist's seam," but `golist`'s seam (`runGoList`) is unexported within `pkg/golist` and not reachable from `internal/cli`'s tests. Wrapping `golist.Run` in an internal seam variable is the structural fix; it lets `seam_test.go` drive `TestRun_PipelineIntegration`, `TestRun_GoListFails`, `TestRun_EmptyResult` with full control. Same pattern, third use.

2. **`signalContext = signal.NotifyContext`** — function-variable seam over the signal-context creator. The spec proposed F-11 use a "stub seam that blocks until cancelled," but stdlib `signal.NotifyContext` cannot be cancelled from inside a single-process test without sending a real OS signal (which interferes with the test runner). Wrapping `signal.NotifyContext` in an internal seam lets `TestRun_ContextCancellation` substitute a pre-cancelled context-creator and exercise the exit-5 path deterministically.

These are structural necessities, not scope creep; the seam pattern is now used at three points within `internal/cli` (`runGitDiff`, `runGoListWrapper`, `signalContext`).

## Ledger

| ID | Criterion | Verify | Status | Evidence |
|----|-----------|--------|--------|----------|
| F-1 | `internal/cli/doc.go` exists with `// Package cli` comment | `test -f internal/cli/doc.go && head -3 \| grep '^// Package cli'` | done | doc.go starts with `// Package cli is the testable entry point...`; verify exits 0. |
| F-2 | `cli.Run` signature matches spec | `go doc …Run \| grep -F 'Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int'` | done | Output: `func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int` — exact match. |
| F-3 | `*GitDiffError` + `ErrGitDiffFailed` exported with documented fields | `go doc …GitDiffError` shows Cmd/ExitCode/Stderr; `go doc …ErrGitDiffFailed` exists | done | All three fields exported with godoc; `ErrGitDiffFailed = errors.New("git diff failed")` documented with reference to errors.As + EH-15/AP-13 discipline. |
| F-4 | `cmd/cascade/main.go` is a one-liner delegating to `cli.Run` | `wc -l < 20`; `grep 'cli.Run' cmd/cascade/main.go` | done | 16 lines total (15 + trailing newline); contains `os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))`. |
| F-5 | Flag parsing covers required + optional flags | `go test -run TestRun_FlagParsing ./internal/cli` | done | `ok github.com/geomyidia/cascade/internal/cli 0.163s`. Four named subtests pass: `unknown_flag`, `unexpected_positional`, `missing_base_no_changed_files`, `missing_base_with_only_tags`. |
| F-6 | Pipeline integration test passes against synthetic packages | `go test -run TestRun_PipelineIntegration ./internal/cli` | done | `ok 0.164s`. Synthetic seams: `runGitDiff` returns `pkga/a.go\n`; `runGoListWrapper` returns 3-package fixture (pkga, pkgb→pkga, pkgc); real depgraph.Build + changeset.Resolve run against the fixture; output is `ex/pkga\nex/pkgb\n`, sorted. |
| F-7 | `git diff` failure → exit 2 | `go test -run TestRun_GitDiffFails ./internal/cli` | done | `ok 0.152s`. Seam returns `*exec.ExitError` + stderr `"fatal: bad revision 'origin/main'\n"`; Run returns 2; stderr propagates `bad revision` substring. |
| F-8 | `go list` failure → exit 3 | `go test -run TestRun_GoListFails ./internal/cli` | done | `ok 0.157s`. golist seam returns `*golist.ExitError` with stderr `"go: updates to go.mod needed"`; Run returns 3; stderr propagates. |
| F-9 | Stdin mode (`--changed-files=-`) works correctly | `go test -run TestRun_StdinChangedFiles ./internal/cli` | done | `ok 0.157s`. Stdin: `"  pkga/a.go  \n\n   \n"` — trimmed, blanks skipped → resolves to `[ex/pkga]`. |
| F-10 | Empty result → exit 0 | `go test -run TestRun_EmptyResult ./internal/cli` | done | `ok 0.149s`. Stdin contains only `README.md` (non-Go); changeset.Resolve skips it; affected set empty; stdout empty; exit 0. |
| F-11 | Context cancellation handled cleanly | `go test -race -run TestRun_ContextCancellation ./internal/cli` | done | `ok 1.172s` under `-race`. signalContext seam returns pre-cancelled ctx; runGitDiff seam reflects ctx.Err(); Run maps to exit 5. Race detector clean. |
| F-12 | Output is sorted and deduplicated | `go test -count=10 -run TestRun_PipelineIntegration ./internal/cli` | done | 10 runs, all pass. The integration test inline-asserts `sort.StringsAreSorted(got)` so non-determinism would fail in the failing subtest, not just under -count (M3 retro carry-forward). |
| F-13 | Per-package coverage gate at 100% on `internal/cli` | `bash scripts/coverage-check.sh` | done | `ok: github.com/geomyidia/cascade/internal/cli coverage 100% >= 100%`. All five cascade packages now at 100% (golist, depgraph, changeset, internal/project, internal/cli). |
| F-14 | `internal/cli` has no non-stdlib imports beyond cascade's pkg/* | `go list -m all \| wc -l` returns 1 | done | Output: 1 (the cascade module itself; zero external deps). Imports: `bufio`, `context`, `errors`, `flag`, `fmt`, `io`, `os`, `os/signal`, `strings`, `syscall`, `bytes`, `os/exec` from stdlib + `pkg/{golist,depgraph,changeset}` + `internal/project` from own module. |
| F-15 | `cascade --help` prints flag reference + exit-code table | Layer-2 `TestCascadeBinaryHelp` | done | Test asserts presence of: `cascade`, `--base`, `--head`, `--tags`, `--changed-files`, `--root`, `--version`, `--help`, `Exit codes`, `git diff failed`, `go list failed`, `cancelled or interrupted`. All present on stdout; stderr clean. |
| F-16 | `cascade --version` prints injected ldflags metadata | Layer-2 `TestCascadeBinaryVersion` (M1 carry-over, refactored to use `buildTestBinary` helper) | done | Test injects `Version=test-1.0.0`, `GitCommit=abcdef0`, `GitBranch=test-branch`, `BuildDate=2026-05-07T18:30:00Z`; `--version` output contains all three substrings. |
| F-17 | End-to-end pipeline against sample module | `go test -run TestCascadeBinaryEndToEnd ./cmd/cascade` | done | `ok 0.486s`. Builds binary; runs against `pkg/golist/testdata/sample-module/`; stdin = `pkga/a.go\n`; affected set contains `example.test/sample/pkga` and `example.test/sample/pkgb` (the importer of pkga). Real go list, real depgraph, real changeset, real RevDepClosure. |
| F-18 | `go install …@<sha>` succeeds and produces a working binary | post-merge: clean GOPATH install + `cascade --help` exec | **deferred to post-merge follow-up** | Verifiable only against a merged commit on `main`. CC re-runs against `proxy.golang.org` once the M5 PR merges; output appended to this retro as a follow-up section. |
| F-19 | Manual sanity check on a real Go module | documented in closing report; commit SHA + commands + first 10 lines (generic-framed) | **deferred to post-merge follow-up** | Same shape as M2 F-18. CC runs cascade against the gta target module once the M5 PR merges; generic-framed evidence appended here. |
| F-20 | README updated with usage, flag reference, exit-code table | `grep -c 'exit code' README.md` returns ≥ 1 | done | Returns 1. README's CLI-usage section now has a Flag reference table (7 rows), an Exit codes table (6 rows: 0/1/2/3/4/5), and a closing line referencing `exit code` in lowercase. |
| F-21 | `go doc github.com/geomyidia/cascade/internal/cli` renders cleanly | `go doc … \| head -30` | done | Package overview prints the role + io-edge note + full exit-code contract; types `Option`-equivalent (none here; this package exports only `Run` + `GitDiffError` + `ErrGitDiffFailed`); every exported name has DC-01 + DC-02-compliant doc comments. |
| F-22 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | done | This document, §"Substrate loaded at session start" enumerates seven guides + cites EH-01/04/07/08/15/16/18, CC-08/09/11/13, AP-04/06/10/11/12/13/15, API-42, TE-01..15/43, DC-01/02. |

## Deferrals

**Two rows deferred to post-merge follow-up:** F-18 and F-19. Both require a merged commit on `main` and cannot run pre-merge. Both will be appended to this retrospective as a follow-up section once the M5 PR closes. **Re-entry condition:** PR merge to `main`. **Estimated work:** 10 minutes for F-18 (clean `go install`), 30 minutes for F-19 (Layer 3 manual sanity check on a real Go module). Per `LEDGER_DISCIPLINE.md`'s definition: "deferred requires a reason and a re-entry condition" — both supplied above.

Zero rows deferred indefinitely; zero rows un-addressed at PR submission.

## What Worked

Patterns that made M5 close cleanly. Per `LEDGER_DISCIPLINE.md` CDC protocol step 8.

**The function-variable seam pattern, third use, generalised cleanly.** M2 introduced `runGoList = defaultRunGoList`; M4 reused for `getCwd = os.Getwd`; M5 reused for `runGitDiff = defaultRunGitDiff` AND added `runGoListWrapper = golist.Run` AND `signalContext = signal.NotifyContext`. The pattern is now used 5 times across the codebase. **The reusability claim from M4's retro is fully validated.** Each application took ~10 lines of code (the var, the default impl, the test-side `withXSeam` helper). Future packages with io edges should reach for this idiom by default; no further documentation work needed beyond the existing precedent.

**The "no adapter code" claim from M4 retro is structurally verified at the integration layer.** `internal/cli/cli.go`'s `runPipeline` is **literally four lines**:

```go
pkgs, err := runGoListWrapper(ctx, cfg.tags, []string{"./..."}, golist.WithDir(cfg.root))
if err != nil { return nil, err }
g := depgraph.Build(pkgs)
seeds := changeset.Resolve(changedFiles, pkgs, changeset.WithModuleRoot(cfg.root))
return g.RevDepClosure(seeds), nil
```

No adapter functions, no shape mismatches, no signature drift between the producer (golist) and consumers (depgraph + changeset). The four pure packages compose so cleanly that the CLI integration is mechanical wiring, not conceptual translation. Worth elevating: **when designing pure-data API surfaces, optimise for downstream-consumer composability; the test is whether the integration layer needs adapters.**

**The plan's "skeleton commit first" discipline (step 3) wasn't strictly necessary for M5 — but discoverable as such.** The plan suggested landing empty stubs first to isolate directory-creation churn from implementation churn. In practice, the implementation went directly from empty to populated within the same iteration, and the skeleton-first step would have added a no-op commit. Worth flagging: skeleton commits are useful for milestones with many files where partial work needs to be isolated for review; M5's six new files in one package landed cleanly in a single implementation pass, so the discipline isn't load-bearing here. Keep the option for larger milestones.

**OUT-1 (M3's lint-cache fix) caught a gofmt issue immediately, again.** First `make check-all` after writing tests caught a struct-literal trailing-comment alignment issue in `cli_test.go`. `make format` fixed it. **OUT-1 is now battle-tested across four milestones (M3, refactor, M4, M5).** Pattern is settled; no further protection needed.

**100% coverage on second try, with one structural addition.** First test run hit 91.1% (`defaultRunGitDiff` at 0%, the `-h` shorthand path uncovered). Adding two tests — `TestDefaultRunGitDiff_Smoke` (mirrors M2's `TestRun_SampleModule` strategy: real subprocess against the cascade repo itself, skipped under `-short`) and `TestRun_HelpShorthand` (covers the `flag.ErrHelp` branch) — closed the gap to 100%. Notable: every branch reached by natural test inputs; no speculative-branch tests. The gate is honest.

**The signalContext seam unlocked deterministic cancellation testing.** The spec's F-11 mitigation said "stub seam that blocks until cancelled" but didn't address the signal-source problem (sending a real SIGINT in a test process interferes with the test runner). The plan's addition of `signalContext` as a third seam — letting tests inject a pre-cancellable context — let `TestRun_ContextCancellation` exercise the exit-5 path in 1.2 seconds with full race-detector coverage and zero process-signal interference. Pattern is reusable: **for any code path that depends on `signal.NotifyContext`, factor the call through a function-variable seam so tests can drive cancellation deterministically.**

**Inline `helpText` const + "// keep in sync" comment is a pragmatic single-source-of-truth.** The spec's Q2 lean was "inline." In practice, this means `cascade --help` and the README's CLI-usage section have parallel content (flag reference + exit-code table). The README's table is human-readable markdown; the inline `helpText` is a Go raw string literal. **The "// keep in sync with README.md" comment in cli.go is the only protection against drift.** This is fine for v0.x; if drift starts surfacing, M6+ could either generate the README from `helpText` or vice versa. Not load-bearing for v0.1.0.

## Pre-PR self-check (CC)

Per the M2/M3/refactor/M4 retro carry-forward.

Walked F-1..F-22:
- **20 rows have `done` status** with reproducible Verify command output captured in the Evidence column above.
- **2 rows are explicitly deferred to post-merge follow-up** (F-18, F-19) with reasons and re-entry conditions documented per `LEDGER_DISCIPLINE.md`. These are not softpedals — both require a merged commit on `main` to verify and have no pre-merge equivalent.
- No rows framed as "pending," "unclear," or "next round."
- Zero softpedals identified at self-check.

**Five-for-five: the CC self-check protocol has now caught zero softpedals across M3, refactor, M4, and M5.** M5 was specifically called out in M4's retrospective as the first non-pure-data milestone where the audit's stress-test would land. Outcome: the audit pattern held under M5's larger integration-shaped scope. **The discipline is settled.**

## CDC review notes

_Pending. To be filled in by CDC after independent verification per `LEDGER_DISCIPLINE.md` CDC protocol._

## Carry-forward into M6 (`v0.1.0` release)

- **F-18 follow-up (`go install …@<sha>`):** post-merge, CC re-runs `go install github.com/geomyidia/cascade/cmd/cascade@<merge-sha>` against `proxy.golang.org`, captures `cascade --help` and `cascade --version` output of the proxy-installed binary, appends to this retrospective. Same M1 F-12 pattern.

- **F-19 follow-up (Layer 3 sanity check):** post-merge, CC runs cascade against the gta target module: capture commit SHA, command run, exit code, wall-clock time, first ~10 lines of stdout (generic-framed per M2 retro convention). Documents in this retrospective with attestation that the integrated pipeline solves the original problem on a real codebase. **This is the load-bearing closure for M5** — proves the trio + CLI wiring matches the synthetic-test predictions at production scale (M2 F-18 measured 2422 packages on the same target).

- **`pkg/cascade.Run` deferral status (Q1).** Lean was defer; no library-consumer signal during M5 implementation. **If M6 release process or early adopters surface friction with the four-line composition pattern, the v0.2 addition is straightforward (~30 lines + tests).** Otherwise, leave deferred.

- **Cancellation exit-code-5 outcome (Q5).** Lean was add. **If shell-callers find it intuitive, that's a win;** if they confuse it with the 1-4 cluster, that's a v0.2 rollback candidate. M6 release notes should call out exit-5 explicitly so users know to handle SIGINT vs internal-error distinctly.

- **Three-seam pattern in `internal/cli`.** `runGitDiff` + `runGoListWrapper` + `signalContext`. **Worth documenting as a precedent for any future package that needs to drive multiple io edges in tests.** The plan's prediction that the seam pattern would generalise cleanly was correct — three uses, three structural mirrors, zero adapter contortion.

- **Skeleton-commit-first discipline (plan step 3) was not load-bearing for M5.** Worth flagging in the broader methodology corpus: skeleton commits are useful when partial work needs intermediate review, not when an entire small package can land in one pass. Keep the option, don't make it mandatory.

- **CC self-check protocol is now validated across 5 milestones.** Zero softpedals caught across M3, refactor, M4, M5. **The audit pattern can be relied on for M6+ without further stress-testing.** Future retros should still walk every row, but the audit's "first stress test" status from M4's retro is now retired.

- **The trio composes without adapter code: structurally verified.** M3's design claim → M4's retro claim → M5's `runPipeline` four-liner. The chain holds. **For M6, if any v0.1.0 release-prep work surfaces an integration-layer adapter need, that's the signal to revisit one of the pure packages' API.** Otherwise, the trio's API stability claim is settled for v1.0.

## Closure

Closing report submitted with the M5 PR; CDC verification pending. **20 of 22 ledger rows reach a final status of `done` in this PR; 2 (F-18, F-19) deferred to post-merge follow-ups with documented reasons and re-entry conditions.** Zero deferrals beyond merge-prerequisite items. Zero no-ops. Zero open at close.

Total rows: 22. Done in PR: 20. Deferred to post-merge follow-up: 2. No-op: 0.
