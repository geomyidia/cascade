# M2 Implementation Retrospective: `golist` Adapter

**Status:** closed.
**Closing commit:** `00dcc51` (head of `m2/golist-adapter` at close; PR #6 head). Merge OID lands here when rebase-merge to `main` completes.
**Adjacent commit:** `5897017` (Stage 1 — build-info fallback / pseudo-version extraction; landed via PR #5 ahead of M2 to keep the gap-closure work atomic).
**CDC verification:** Claude (Opus 4.7, 2026-05-06 session); F-18 evidence verified live against `/Users/oubiwann/lab/private-repo/api`.
**Source spec:** [`docs/design/05-active/0003-cascade-m2-golist-adapter.md`](../design/05-active/0003-cascade-m2-golist-adapter.md). Anticipated to transition 05-active → 06-final post-merge.
**Source impl plan:** [`docs/dev/0002-implementation-plan-project-build-info-fallback-m2-golist-adapter.md`](./0002-implementation-plan-project-build-info-fallback-m2-golist-adapter.md).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md).

## Closure summary

The spec's acceptance ledger declared **eighteen** rows — the largest ledger to date — and a stated **deferral budget of zero**. All eighteen rows reach a final status of `done`; no deferrals. The exit signal from the spec — *"`golist.Run(ctx, tags, patterns)` returns a parsed, sorted `[]golist.Package` for a real Go module; CI runs the per-package coverage gate at 100% on the algorithmic surface; every io error has a typed return path with enough context that callers can act on it"* — is satisfied across all three clauses.

| Status | Count |
|--------|-------|
| Done | 18 |
| Deferred | 0 |
| No-op | 0 |
| Open at close | 0 |

The structural property that distinguishes cascade from gta — non-empty, fully-populated `[]Package` against the same real-world codebase whose `packages.Load` invocations gta swallowed silently — is **observed in production-scale conditions** (F-18). This is the load-bearing closure for the milestone, not the synthetic fixtures.

## Ledger

| ID | Criterion | Verify (reproducible) | Status | Evidence |
|----|-----------|----------------------|--------|----------|
| F-1 | `golist/` package has `doc.go` with the M1-carryover package comment | `head -3 golist/doc.go \| grep -q '^// Package golist'` | done | M1 stub preserved verbatim. |
| F-2 | `Package` struct exists with the spec's exact 11-field set | `go doc github.com/geomyidia/cascade/golist.Package` | done | All 11 fields exported with json struct tags + godoc; CI matrix builds `Package`-consuming tests on Go 1.25.3 and 1.26.x. |
| F-3 | `Module` struct: `Path string` + `Main bool` | `go doc github.com/geomyidia/cascade/golist.Module` | done | Exact shape; pointer-from-`Package.Module` so stdlib packages can have `Module == nil` per spec. |
| F-4 | `Run` signature matches spec | `go doc github.com/geomyidia/cascade/golist.Run` | done | `func Run(ctx context.Context, tags []string, patterns []string, opts ...Option) ([]Package, error)`. |
| F-5 | `Option`, `WithDir`, `WithEnv`, `WithGoBin` exported | `for s in Option WithDir WithEnv WithGoBin; do go doc github.com/geomyidia/cascade/golist.$s; done` | done | `WithEnv` retained per resolved Q2 (spec author's lean was to drop; maintainer kept). |
| F-6 | `*ExitError`, `*ParseError`, `ParseErrorMaxPayload` exported with documented fields | `go doc` per type | done | `ParseErrorMaxPayload = 4096`. `ExitError` carries `underlying *exec.ExitError` reachable via `Unwrap`. |
| F-7 | `ErrGoNotFound`, `ErrGoListFailed`, `ErrParseFailed` sentinels exported | `go doc github.com/geomyidia/cascade/golist \| grep ^var` | done | Three `errors.New(...)` sentinels at package scope. |
| F-8 | `*ExitError` chains via `errors.Is`/`errors.As` | `go test -run 'TestRun_ExitErrorChain' ./golist` | done | Test passes; `errors.As` reaches the wrapped `*exec.ExitError` on real subprocess invocation. |
| F-9 | `*ParseError` chains via `errors.Is`/`errors.As` | `go test -run 'TestParseStream_ErrorFixtures' ./golist` | done | Test fixture-driven; both `truncated.json` and `malformed.json` cases exercised. |
| F-10 | All eight Layer-1 fixtures present + tested | `for f in single-package multi-package with-tests build-tag stdlib-mixed empty truncated malformed; do test -f golist/testdata/$f.json; done` | done | All eight present; six success cases + two error cases under `TestParseStream_*`. |
| F-11 | Sample module at `golist/testdata/sample-module/` with the documented files | `test -f` per file | done | Five files: `go.mod`, `pkga/a.go`, `pkgb/b.go`, `pkgc/c.go` + `c_test.go`. **Spec extension:** added `pkgd/pkgd_linux.go` + `pkgd/pkgd_darwin.go` per resolved Q6 (build-tag exercise) — extends the smoke test surface; documented inline. |
| F-12 | `TestRun_SampleModule` passes (skipped under `-short`) | `go test -run 'TestRun_SampleModule' ./golist` | done | Layer-2 subprocess invocation; build-tag selection asserted per `runtime.GOOS`. |
| F-13 | Per-package coverage gate at 100% on `golist` | `bash scripts/coverage-check.sh` | done | `ok: github.com/geomyidia/cascade/golist coverage 100% >= 100%`. The single os/exec line in `defaultRunGoList` is statement-covered by Layer-2 (no exception needed in practice — the function-variable seam refactor made every other branch in-process testable). |
| F-14 | Cancelled context kills subprocess and returns ctx.Err() | `go test -run 'TestRun_ContextCancellation' ./golist` | done | Test pre-cancels then invokes `Run`; `errors.Is(err, context.Canceled)` true. |
| F-15 | Concurrent `Run` calls safe under `-race` | `go test -race -run 'TestRun_Concurrent' ./golist` | done | 4 goroutines × full `./...` against the sample module; race detector clean; all return equivalent slices. |
| F-16 | `golist` package has no non-stdlib imports | `go list -m all \| wc -l` | done | Output: `1` (the cascade module itself; zero external deps). |
| F-17 | Package documentation renders cleanly via `go doc` | `go doc github.com/geomyidia/cascade/golist \| head -30` | done | godoc surface includes package overview, `Package`, `Module`, `Run`, options, error types, and sentinels with full doc comments per DC-01/DC-02. |
| F-18 | Manual sanity check on a real Go module that previously broke gta | One-off harness against the target codebase | done | Cascade `00dcc51` against the target module: `golist.Run` returned **2422 packages** (2179 non-stdlib, 231 in main module) in ~3 seconds wall clock with no error. **The gta failure mode (silent zero-package output from `packages.Load`) is structurally absent.** Generic-framing evidence at PR #6 comment (per resolved Q7); precise figures captured in this retrospective only. |

## Deferrals

**Zero deferrals.** The spec's stated deferral budget was zero, and the milestone closes with that property held. F-18 was *initially* softpedalled in the PR description ("deferred to closing-report evidence in the next round") but was caught by CDC and re-executed in-session — see § CDC review notes.

## What Worked

Patterns that made M2 close cleanly. Per `LEDGER_DISCIPLINE.md` CDC protocol step 8.

**Function-variable seam over the os/exec call.** The original implementation had `Run` directly invoking `exec.CommandContext(...).Run()`, which left two branches uncoverable from in-process tests (the `parseStream`-error-after-successful-exec path inside `Run`, and the "unexpected exec failure" fallthrough in `classifyRunError`). Per-package gate dropped to 98.75%. Refactor: extract the subprocess invocation into a function-variable `runGoList` whose default implementation does the exec, and let tests substitute it via a `withSeam` helper. Result: every branch in `Run` and `classifyRunError` is in-process testable; the only non-coverable line becomes `cmd.Run()` itself, covered by the Layer-2 sample-module test. Coverage closes to 100%. **The pattern is a precedent for M3+:** when an io shell needs in-process testability without subprocess overhead, the seam is the textbook structural answer.

**`exec.ErrNotFound` is for PATH lookup only.** `TestRun_GoNotFound` initially failed because `WithGoBin("/abs/path/that/does/not/exist")` returns `*os.PathError` matching `fs.ErrNotExist`, *not* `exec.ErrNotFound` — that latter sentinel is what `exec.LookPath` returns when a name without slashes can't be found on PATH. Fix in `classifyRunError`: match either sentinel before classifying as `ErrGoNotFound`. Multi-line comment block at the call site documents the distinction so the next reader doesn't relearn it. Same substrate-pattern lesson as M1's golangci-lint pin: capture the discovered constraint at the artifact.

**Bounded-payload capture for `ParseError`.** The streaming decoder's `dec.Buffered()` exposes the bytes it had read but not yet consumed when decoding failed — combined with `io.MultiReader` against the rest of the input, that's the cleanest way to capture diagnostic context without unbounded buffering. The `TestParseStream_PayloadCappedAtMax` test feeds 8KB of garbage after a valid record and asserts `len(Payload) <= ParseErrorMaxPayload`. Cap is structurally enforced; documented in the package comment.

**Pre-PR build-info gap closure (Stage 1).** PR #5 landed the pseudo-version commit-extraction work *before* M2 started, closing the proxy-install diagnostic gap discovered during M1's verification. Sequencing the small-and-orthogonal fix as a separate PR kept the M2 diff focused on the spec's surface, avoided the temptation to bundle "while we're in there" work, and gave each piece its own clean closure boundary. The plan that interleaved this (`0002-implementation-plan-...`) explicitly called for two PRs; following it as written paid off.

**Spec ledger embedded in the design doc from day one.** M1's retrospective was the *first* artifact to formalise the ledger structure. M2's spec ships with F-1..F-18 already in place, with grep-verifiable Verify columns. Closing the milestone became a row-by-row exercise rather than a synthesis effort. Carry-forward note from M1's CDC review (resolved Q8) — affirmed as the M3+ default.

**F-18 as the load-bearing closure.** The 18 ledger rows are mostly structural (function exists, sentinel chains, fixture parses). F-18 is the *only* row that proves the gta failure mode is absent at production scale. The `~3s` wall clock + 2422 packages + zero error returned + sane field population is the real signal — everything else is necessary scaffolding. Worth treating F-18-style external-codebase evidence as the gold-standard verification pattern for any future milestone whose closure depends on a structural claim about real-world behavior.

**Mid-stream regex relaxation in the build-info fallback.** While building Stage 1's `parsePseudoVersion`, the initial regex `-(\d{14})-([a-f0-9]{12})$` matched `v0.0.0-...-COMMIT` but rejected `v0.1.1-0.YYYYMMDDHHMMSS-COMMIT` (the patched-tag pseudo-version form). Caught at test-write time, not in CI. Relaxed to `[-.](\d{14})-([a-f0-9]{12})$`. Lesson: when emulating a published format, write a test case for *each form the format documentation specifies*, not just the canonical one.

## CDC review notes

Three observations affecting how M3+ should be reviewed.

**F-18 was initially softpedalled — and CDC caught it in real time.** The first PR description framed F-18 as "deferred to closing-report evidence in the next round." That phrasing dodged the criterion: F-18 is required for closure, not a follow-up. The maintainer flagged it directly: *"has the manual sanity check been run against a real Go module?"* — which is exactly the per-row evidence-against-criterion compare that `LEDGER_DISCIPLINE.md` calls for. CC re-executed F-18 in the same session, captured the 2422-package result, posted generic-framing evidence to PR #6, and now closes F-18 cleanly. **This is the second consecutive milestone where CC's initial closure included a softpedalled row that CDC caught.** The countermeasure stays the same — per-row walk, evidence text compared against criterion text — and now has empirical track record across two milestones. Future CC self-assessment should pre-emptively read each row's criterion text against the planned evidence and flag any "pending" / "deferred" / "next round" framings as drift candidates *before* PR open.

**Local lint cache drift bit M2 the same way it bit M1.** Initial M2 push: lint failed on `revive` stutter (`version.VersionString`) that local missed because of stale `golangci-lint` cache. Push 2 (after Stage 1 → M2 transition): lint failed on `gosec G204` + `revive unused-parameter` that local missed for the same reason. Both were genuine issues that the code review caught and the local cache hid. M1's carry-forward called this out and proposed `--no-cache` on local `make lint` — the proposal **was not adopted**. Recommend codifying for M3: add `GOLANGCI_LINT_CACHE=$(mktemp -d)` (or `--no-cache` if the action's lint step preserves cache) to the local `make lint` recipe so contributors don't push lint-clean-locally that fails CI. Adding to carry-forward.

**Generic-framing discipline held.** F-18's public PR comment uses placeholders (`<thousands>`, `<hundreds>`, `<cloud-provider>.<sdk-prefix>/...`) without the precise counts or the project name. The precise figures (2422 / 2179 / 231 / specific import-path family) live only in this retrospective and the PR's private review thread. CDC verification: read the public PR comment text, confirm no fingerprinting. This works; reuse the pattern when future F-N rows depend on private-codebase evidence.

## Carry-forward into M3

- **Local lint cache discipline.** Codify `--no-cache` (or equivalent) in the Makefile's `lint` recipe, *or* configure the cache to invalidate on `.golangci.yml` mtime. Two milestones is a pattern; M3 shouldn't be a third. The fix is a 1-line Makefile change.

- **Function-variable seam pattern is the precedent for M3.** M3's depgraph package is mostly pure (no io between packages), so it may not need the seam. M4's changeset package has a small io edge for file-path canonicalization that *will* — pre-design with this pattern in mind.

- **Ledger formalism is M3+ default.** Q8 affirmed by experience: spec carries the ledger, CC executes against rows, retrospective documents closure. Don't relax this when M3's ledger is smaller or the work feels routine.

- **F-18-style external-codebase evidence pattern.** Reusable any time a milestone's closure depends on real-world behavior (not just synthetic fixture conformance). M3 (depgraph) and M4 (changeset) are pure-data packages, so they probably don't need this. M5 (CLI) does — running cascade end-to-end against the same target as F-18 will be the M5 analogue.

- **CC self-checks on row closure framings before PR open.** Read each row's criterion text against the planned evidence text; flag any "pending" / "deferred" / "next round" wording as a drift candidate before pushing. This is the structural fix for the F-12 / F-18 pattern; without it, the CDC review keeps doing what CC self-review should have done.

- **Pseudo-version forms documentation.** When future builds-info-related work touches `parsePseudoVersion`, remember Go publishes *three* pseudo-version shapes (`vX.0.0-...`, `vX.Y.Z-pre.0...`, `vX.Y.(Z+1)-0...`); the regex `[-.]\d{14}-[a-f0-9]{12}$` covers all three because the leading char is either `-` or `.`. Don't tighten without re-checking all forms.

## Closure

Closed at PR #6 head `00dcc51`; merge OID lands here when rebase-merge to `main` completes. All eighteen criteria reach a final status. Zero deferrals. CDC verification: this document.

Total rows: 18. Done: 18. Deferred: 0. No-op: 0.
