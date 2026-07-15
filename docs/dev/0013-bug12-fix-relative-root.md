# Bug Fix Retrospective: #12 — silent-empty output when `--root` is relative

**Status:** closing report; awaiting CDC verification.
**Closing commit:** _TBD_ (head of `fix/bug12-relative-root` at PR open).
**Adjacent commits:** the bug-fix work lands in two commits — implementation+tests, and docs+retro.
**CDC verification:** pending.
**Source:** GitHub issue [#12](https://github.com/geomyidia/cascade/issues/12) (filed by maintainer 2026-05-07).
**Source plan:** [`docs/dev/0011-implementation-plan-m5-cli-main-wiring.md`](./0011-implementation-plan-m5-cli-main-wiring.md) — wait, no — plan-mode plan at `~/.claude-private/plans/please-brush-up-on-joyful-pearl.md` (the plan was approved via ExitPlanMode prior to implementation; not committed to the repo per the convention for transient bug-fix planning).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md).
**Target version:** v0.1.1 (patch release post-merge).
**Verification target:** a PR against an internal Go monorepo — the cascade-fix verification PR that should green once v0.1.1 ships.

## Bug summary

cascade v0.1.0's documented default invocation — `cascade --base=origin/main --head=HEAD` (no `--root`, taking the documented default of `.`) — silently emitted an empty affected-package list and exited 0 against any real Go monorepo. **The exact "silent-zero-packages" failure mode that drove the gta → cascade pivot.**

Root cause: the CLI passed the literal string `"."` through `changeset.WithModuleRoot`. Inside `changeset.Resolve`, `filepath.Join(".", "rel/path")` returned a relative path; `filepath.Dir` of that yielded a relative parent; the `dirMap` (keyed on absolute `golist.Package.Dir` values) missed every lookup; `RevDepClosure([])` returned nil; cascade exited 0 with empty stdout.

The bug lived in a parameter-space cell that no test happened to hit: relative moduleRoot × absolute pkg.Dir keys. Both M4 and M5 had 100% statement coverage but the pairing was untested in either layer. **Coverage didn't catch it because coverage measures statement reachability, not behavioral coverage across the parameter space.**

## What changed

**Five deliverables, all in this PR.**

### 1. Library fix (`pkg/changeset/changeset.go`)

```go
// After the moduleRootSet check, before dirMap-build:
if abs, err := filepath.Abs(cfg.moduleRoot); err == nil {
    cfg.moduleRoot = abs
}
```

Defensive at the library level — protects ANY current/future caller (including the deferred `pkg/cascade.Run`) from the bug class. `filepath.Abs("")` and `filepath.Abs(".")` both resolve to the cwd, so this also handles the empty-string case gracefully.

### 2. CLI fix (`internal/cli/cli.go`)

```go
// After parseFlags returns successfully, before validateConfig:
if abs, err := filepath.Abs(cfg.root); err == nil {
    cfg.root = abs
}
```

Surgical fix at the CLI layer: the four downstream consumers (`runGitDiff`, `classifyGitDiffError`, `golist.WithDir`, `changeset.WithModuleRoot`) all receive a path absolutized exactly once. Defense in depth alongside the library fix.

### 3. Diagnostic warning (`internal/cli/cli.go`)

```go
// After runPipeline returns:
if len(affected) == 0 {
    nGoFiles := 0
    for _, f := range changedFiles {
        if strings.HasSuffix(f, ".go") {
            nGoFiles++
        }
    }
    if nGoFiles > 0 {
        fmt.Fprintf(stderr, "cascade: %d changed Go file(s) did not resolve to any package; check --root\n", nGoFiles)
    }
}
```

Surfaces the suspicious-zero-result case loudly. Filters on `.go`-suffix so docs-only PRs (legitimate empty case) stay silent. Doesn't change exit code; just adds a stderr breadcrumb. The issue author's lean: *"would have caught this in seconds during the private-codebase integration."*

### 4. Regression tests (4 new tests, three layers)

- **`pkg/changeset/changeset_test.go`** → `TestResolve_RelativeModuleRoot_AbsolutizedInternally` (2 subtests: `"."` and `""`). Pre-fix this test fails with empty result; post-fix it passes. Library-layer regression coverage.
- **`internal/cli/seam_test.go`** → `TestRun_DefaultRootResolvesCWD`. End-to-end CLI invocation with no `--root` flag (default `"."`) + relative changedFiles + cwd-rooted absolute pkg.Dir values. Pre-fix produced the empty-output bug; post-fix produces the expected affected-set on stdout with no diagnostic.
- **`internal/cli/seam_test.go`** → `TestRun_DiagnosticOnSuspiciousEmpty`. Drives a pipeline where the changedFile path doesn't match any pkg.Dir; asserts the stderr diagnostic line fires with the correct count.
- **`internal/cli/seam_test.go`** → `TestRun_NoDiagnosticOnDocsOnlyEmpty`. Asserts the diagnostic does NOT fire on docs-only changesets (no `.go` files). Pins the `.go`-suffix filter behaviour.

All five cascade packages remain at 100% coverage post-fix; no test contortions needed.

### 5. Documentation updates

- **`pkg/changeset/changeset.go`** — `WithModuleRoot` godoc: now explicitly documents that the supplied value is absolutized via `filepath.Abs`, with explicit reference to bug #12. Test-purity story preserved (passing an absolute path is still a no-op for absolutize).
- **`README.md`** — `--root` flag row gains: *"Absolute or relative; cascade absolutizes via `filepath.Abs` before use, so the default `.` resolves to the process cwd."*

## Verification

**Pre-merge** (in this PR):
- `make check-all` green: build + lint (cold cache via OUT-1) + race tests + per-package coverage gate at 100% on all 5 cascade packages (golist, depgraph, changeset, internal/project, internal/cli).
- Local smoke against the cascade repo itself reproducing the issue's exact failure case:
  ```
  $ ./bin/cascade-fixed (built from this branch)
  $ echo "pkg/golist/golist.go" | ./bin/cascade-fixed --changed-files=-
  github.com/geomyidia/cascade/cmd/cascade
  github.com/geomyidia/cascade/internal/cli
  github.com/geomyidia/cascade/pkg/changeset
  github.com/geomyidia/cascade/pkg/depgraph
  github.com/geomyidia/cascade/pkg/golist
  ```
  Pre-fix (v0.1.0): empty output. Post-fix: 5-package affected-set, sorted lexicographically. **The bug is closed.**

**Post-merge** (closes after v0.1.1 tag + GH release):
- `go install github.com/geomyidia/cascade/cmd/cascade@v0.1.1` against `proxy.golang.org` — confirm tag indexed cleanly (M5 F-18 lineage).
- **The internal-Go-monorepo verification PR** CI greens up. This is the load-bearing real-codebase closure for the bug — same shape as M2 F-18 and M5 F-19 (real-codebase verification proves the fix works at production scale).

## What Worked

**The five-seam pattern's testability paid off again.** All four new tests use existing seams (`runGitDiff`, `runGoListWrapper`, `signalContext`) to drive the bug + diagnostic scenarios in-process, no real subprocess invocations. The bug's cell-in-the-test-matrix (relative moduleRoot × absolute pkg.Dir) is now explicitly exercised at both the library layer (where the bug literally lived) and the CLI integration layer (where the user-facing surface is). M5's seam-pattern claim — *"established convention; future packages with io edges should reach for it by default"* — generalised cleanly to bug-regression coverage too: if a future change re-introduces the bug class at either layer, the layer-specific test fails first.

**Defense-in-depth at both layers (Q1 user decision) was the right call.** The CLI fix is surgical and immediate; the library fix protects future consumers (the deferred `pkg/cascade.Run`, third-party code importing `pkg/changeset` directly). If only the CLI was fixed, a future library consumer would re-discover the bug. If only the library was fixed, the CLI's `cfg.root` flowing through `runGitDiff` etc. would still be relative — not buggy in those callers, but inconsistent with the absolutization-once-at-the-edge story.

**The diagnostic stderr line (Q2 user decision) is the right kind of "loudness."** Doesn't change exit code; doesn't fire on legitimate-empty cases (docs-only PRs); fires loudly on suspicious-empty cases. **The bug was discovered exactly the way the diagnostic would have surfaced it: a real-world user noticed the empty output didn't match expectations.** With this diagnostic in place, future cascade users catch this bug class in their first real-world run, not after digging through CI logs.

**Plan-mode `AskUserQuestion` for the two material decisions was load-bearing.** Both Q1 (defense-in-depth scope) and Q2 (diagnostic shape) had multiple defensible answers. Asking surfaced the user's preference (both at the comprehensive end) rather than me applying a default and getting it slightly wrong. Same pattern that worked for M4's Q1+Q2 and M5's plan-mode session.

**100% coverage maintained without contortion.** The new `if abs, err := filepath.Abs(...); err == nil` branches in both layers don't introduce a separately-coverable error path because `filepath.Abs` only errors when `os.Getwd` fails — a system-level event that's fundamentally unreliable to test in-process. The `err == nil` branch is the happy path; the implicit "leave as-is on error" branch is structurally dead in any test environment where `os.Getwd` works (which is all of them). Coverage still reads 100% because the error branch's "do nothing" body has no statements to count.

## What didn't work (worth carrying forward)

**Coverage discipline didn't catch a behavioral-space gap.** M5's retro claimed *"100% coverage on first try, with no test contortions"* and that's true for line coverage. But coverage measured statement reachability, not behavioral coverage across the parameter space. The bug lived in a cell of the parameter space (relative moduleRoot × absolute pkg.Dir) that no test happened to hit, even though every line in the production code was reached.

**Carry-forward into M7+ test design:** when designing tests for new features (especially library APIs that take typed-string arguments like paths, refs, or import patterns), explicitly enumerate the parameter-space matrix rather than assuming line coverage implies behavioral coverage. For path-handling code specifically: relative-vs-absolute × empty-vs-non-empty × symlink-vs-not × OS-native-separator-vs-other. For each cell, document either a test row OR a deliberate "out of scope" decision. The point isn't to write 16x as many tests; it's to think the matrix through explicitly so the unexamined cell becomes a documented decision rather than a silent gap.

**The CC self-check protocol's track record (5-for-5 zero softpedals from M3/refactor/M4/M5/M6) does NOT extend to behavioral-space gaps.** The audit catches textual softpedals (evidence-vs-criterion mismatches in the row walk). It doesn't catch "this row is structurally `done` but the assertion space is incomplete." Worth recording as a known limitation: the audit is necessary, not sufficient. The mature-field discipline this approximates (aviation 14 CFR 121.563, NRC inspector pattern) catches the kind of drift the audit is good at; behavioral-space gaps are the M5-retro-style "first stress test against integration-shaped scope" failure mode that needs different protection.

**The Makefile's `release-dry-run` target's go.sum bug surfaced again** when M6 tagged v0.1.0 — wasn't fixed in M6 because release work didn't include Makefile changes. That bug is still open; same workaround applies for v0.1.1 (manual `git tag` + `git push origin v0.1.1` skipping the Makefile target).

## Open items / carry-forward

- **v0.1.1 release** — tag + push + GH release after this PR merges. Standard M6 flow (manual tag + push to bypass the Makefile's go.sum bug). Release notes should call out: (a) bug #12 fix, (b) reproduction case, (c) verification via a private downstream consumer's CI, (d) recommendation that v0.1.0 users upgrade.
- **Real-world verification** — re-trigger the internal-Go-monorepo verification PR's CI against the v0.1.1 binary. **This is the load-bearing closure for bug #12.** Synthetic tests pass; production-scale evidence is the structural proof.
- **Should v0.1.0 be `retract`-ed?** Plan deferred to data-driven decision. If real-world consumers report the bug beyond #12, add `retract v0.1.0` to go.mod in v0.2. If only the maintainer hit it, leave v0.1.0 available with the v0.1.1 release-notes recommending upgrade.
- **Methodology note on coverage-vs-behavior gap.** Documented above; worth surfacing in any future M-spec's "test strategy" section so milestone leads explicitly enumerate the parameter-space matrix.
- **The Makefile's `release-dry-run` go.sum bug.** Not blocking v0.1.1 (manual workaround works) but should be fixed before the next release flow. One-line fix: `git diff --quiet -- go.mod && { ! test -f go.sum || git diff --quiet -- go.sum; }`. Could ride alongside any v0.2 release-prep work.

## Closure

All five deliverables landed; bug #12's reproduction case verified-fixed locally; all five cascade packages at 100% coverage; `make check-all` green. Awaiting CDC verification. Real-world-codebase closure pending v0.1.1 release + internal-monorepo verification PR re-trigger.
