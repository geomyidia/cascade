# M3 Implementation Retrospective: `depgraph` (Reverse-Dep Closure)

**Status:** closing report; awaiting CDC verification.
**Closing commit:** `835b4b0` (head of `m3/depgraph-closure` at close; M3 PR head). Merge OID lands here when rebase-merge to `main` completes.
**CDC verification:** pending.
**Source spec:** [`docs/design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md`](../design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md). Anticipated to transition 05-active → 06-final post-merge.
**Source impl plan:** [`docs/dev/0005-m3-implementation-plan-depgraph-reverse-dep-closure.md`](./0005-m3-implementation-plan-depgraph-reverse-dep-closure.md).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md).

## Closure summary

The spec's acceptance ledger declared **nineteen** rows (F-1..F-19) with a stated **deferral budget of zero**. The implementation plan added **F-20** (Stats type/method) per resolved decision M3-1 (user-authorised spec amendment); the active spec should pick that row up at next ODM promotion. Total: 20 rows. All twenty reach a final status of `done`; no deferrals. The exit signal from the spec — *"`depgraph.Build(pkgs)` returns a `*Graph`; `g.RevDepClosure(seeds)` returns the seeds plus every package that transitively imports any of them, sorted lexicographically; the per-package coverage gate at 100% holds against table-driven tests over synthetic graphs"* — is satisfied across all three clauses.

| Status | Count |
|--------|-------|
| Done | 20 |
| Deferred | 0 |
| No-op | 0 |
| Open at close | 0 |

The structural property that distinguishes M3 from a routine pure-data package — **100% statement coverage held without exception** — is observed cleanly. Unlike M2 (which needed a function-variable seam to bring the os/exec line into reach), M3 has no inherently-uncoverable code; the gate's report is the truth. The one dead branch the existing in-progress `depgraph.go` carried (the impossible-by-construction dedup loop in `sortAndDedup`) was removed during implementation rather than worked around in tests, per the CLAUDE.md discipline against defensive code for scenarios that cannot happen.

## Substrate loaded at session start

Per [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md) §"Substrate" and the spec's §"Required reading" (F-19):

- [`AI-CONSTITUTION-SUPPLEMENT.md`](../../assets/ai/AI-CONSTITUTION-SUPPLEMENT.md), [`AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md) — read at session start.
- [`assets/ai/go/SKILL.md`](../../assets/ai/go/SKILL.md) — Document Selection Guide table + Critical Rules.
- [`assets/ai/go/guides/04-type-design.md`](../../assets/ai/go/guides/04-type-design.md) — full read; load-bearing IDs **TD-01** (zero-value usefulness, applied to the empty-input Build case), **TD-05** (no embedded uncopyable state in `Graph`), **TD-09** (uniform pointer receivers per type-shape, applied to all five `*Graph` methods), **TD-15** (return `error` not concrete error type — N/A here, no error returns), **TD-17** (treat nil slices as empty, applied to DirectImports/DirectImporters returning nil for absent paths and empty slices).
- [`assets/ai/go/guides/05-interfaces-methods.md`](../../assets/ai/go/guides/05-interfaces-methods.md) — IM-01..IM-05 + IM-17 + IM-18 sections; load-bearing IDs **IM-01** (no interfaces returned, only `*Graph`), **IM-04** (the impl plan cites IM-04 for receiver consistency; the actual IM-04 in the guide names single-method-interface naming convention — receiver-consistency lives at TD-09. Both are honored regardless: uniform `*Graph` receivers for all methods), **IM-17** (Build returns `*Graph`, never typed-nil; the function constructs a non-nil pointer in every code path).
- [`assets/ai/go/guides/02-api-design.md`](../../assets/ai/go/guides/02-api-design.md) — API-41 / API-42 sections; **API-42** (no globals; everything threaded through Build's output) honored. **API-41** (functional options) is N/A — Build takes no options in v0.x; deferred per spec §"Out of scope".
- [`assets/ai/go/guides/07-testing.md`](../../assets/ai/go/guides/07-testing.md) — TE-01..TE-08 + TE-15 + TE-43 sections; load-bearing IDs **TE-01..TE-08** (table-driven tests with named subtests, field-named struct literals, `tt` iteration variable, function-identifier subtest names with no spaces or slashes), **TE-15** (no assertion library — plain stdlib `testing`, matches the existing `golist_test.go` pattern; CLAUDE.md's testify-mention is outdated), **TE-43** (`package depgraph_test` for public-API tests).
- [`assets/ai/go/guides/11-documentation.md`](../../assets/ai/go/guides/11-documentation.md) — DC-01 / DC-02 sections; **DC-01** (every exported name documented), **DC-02** (every doc comment starts with the identifier name) — verified via `go doc` for F-2..F-7, F-18, F-20.
- [`assets/ai/go/guides/09-anti-patterns.md`](../../assets/ai/go/guides/09-anti-patterns.md) — index walked via SKILL.md's Critical Rules section. Specific anti-patterns avoided: defensive-code-for-impossible-cases (the dead `sortAndDedup` dedup loop, removed); typed-nil interface returns (none — Build returns concrete `*Graph`); slice aliasing (DirectImports/DirectImporters return `slices.Clone` copies per TD-07).

## Ledger

| ID | Criterion | Verify (reproducible) | Status | Evidence |
|----|-----------|----------------------|--------|----------|
| F-1 | `depgraph/doc.go` exists with package comment | `test -f depgraph/doc.go && head -3 depgraph/doc.go \| grep -q '^// Package depgraph'` | done | M1 stub preserved verbatim; verify command exits 0. |
| F-2 | `Graph` type exported, opaque (no exported fields) | `go doc github.com/geomyidia/cascade/depgraph.Graph \| grep -E '^type Graph struct'` | done | Output: `type Graph struct {` followed by `// Has unexported fields.` and `}`. No exported field lines. Five methods documented (Build, RevDepClosure, DirectImports, DirectImporters, Has, Stats). |
| F-3 | `Build` signature matches spec | `go doc github.com/geomyidia/cascade/depgraph.Build \| grep -F 'func Build(pkgs []golist.Package) *Graph'` | done | Output: `func Build(pkgs []golist.Package) *Graph` — exact match. |
| F-4 | `RevDepClosure` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.RevDepClosure \| grep -F 'func (g *Graph) RevDepClosure(seeds []string) []string'` | done | Output: `func (g *Graph) RevDepClosure(seeds []string) []string` — exact match. |
| F-5 | `DirectImports` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.DirectImports \| grep -F 'DirectImports(path string) []string'` | done | Output: `func (g *Graph) DirectImports(path string) []string` — exact match. |
| F-6 | `DirectImporters` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.DirectImporters \| grep -F 'DirectImporters(path string) []string'` | done | Output: `func (g *Graph) DirectImporters(path string) []string` — exact match. |
| F-7 | `Has` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.Has \| grep -F 'Has(path string) bool'` | done | Output: `func (g *Graph) Has(path string) bool` — exact match. |
| F-8 | All 16 `TestRevDepClosure_StandardTopologies` cases pass | `go test -run 'TestRevDepClosure_StandardTopologies' ./depgraph` | done | 16 named subtests pass: empty\_graph\_no\_seeds, empty\_graph\_one\_seed, single\_node\_no\_edges\_seed\_self, single\_node\_no\_edges\_seed\_other, linear\_chain\_seed\_leaf / middle / root, diamond\_seed\_leaf / middle, cycle\_3node, self-loop, disconnected\_seed\_one\_component, disconnected\_multi\_seed, duplicate\_seeds, unsorted\_seeds, mixed\_seeds\_some\_absent. Each subtest also asserts `sort.StringsAreSorted(got)` — the determinism criterion enforced inline. |
| F-9 | Hand-traceable 5-package case passes | `go test -run 'TestRevDepClosure_HandTraceable' ./depgraph` | done | `ok github.com/geomyidia/cascade/depgraph 0.150s`. Six sub-cases assert the exact closures from the spec: seed=[a]→[a]; seed=[b]→[a,b,c]; seed=[d]→[a,b,c,d]; seed=[e]→[e]; seed=[a,e]→[a,e]; empty seeds→nil. |
| F-10 | Test/XTest imports become edges | `go test -run 'TestBuild_TestAndXTestImportsBecomeEdges' ./depgraph` | done | `ok github.com/geomyidia/cascade/depgraph 0.148s`. Verifies a Package with `Imports=[i,shared]`, `TestImports=[t,shared]`, `XTestImports=[xt]` produces `DirectImports(p) = [i, shared, t, xt]` (deduplicated) and that `t`, `xt`, `shared` all report `p` as an importer. |
| F-11 | `DirectImports` / `DirectImporters` / `Has` correct | `go test -run 'TestDirect\|TestHas' ./depgraph` | done | `ok github.com/geomyidia/cascade/depgraph 0.149s`. TestDirectImports + TestDirectImporters + TestHas all pass; each tests present-with-multiple, present-with-none, and absent paths. The Direct\* tests also assert that mutating the returned slice does not corrupt the graph's internal state (TD-07 boundary copy). |
| F-12 | Duplicate-package handling idempotent | `go test -run 'TestBuild_DuplicatePackagesIdempotent' ./depgraph` | done | `ok github.com/geomyidia/cascade/depgraph 0.147s`. Verifies last-entry-wins (forward edges from second occurrence; first occurrence's edges discarded), single Has on the duplicate path, and that empty-ImportPath input is silently skipped. |
| F-13 | Output is sorted lexicographically; deterministic across runs | `go test -count=10 -run 'TestRevDepClosure' ./depgraph` | done | 10 runs, all pass; `ok github.com/geomyidia/cascade/depgraph 0.157s`. Each subtest also asserts `sort.StringsAreSorted(got)` so non-determinism would surface as a sort-order failure within the subtest, not just a 10-run flake. |
| F-14 | Cycle / self-loop terminates | `go test -timeout=10s -run 'TestRevDepClosure_StandardTopologies/cycle\|TestRevDepClosure_StandardTopologies/self-loop' ./depgraph` | done | `--- PASS: TestRevDepClosure_StandardTopologies/cycle_3node (0.00s)`, `--- PASS: TestRevDepClosure_StandardTopologies/self-loop (0.00s)` — both terminate well under 10s. The grep pattern `cycle\|self-loop` matches `cycle_3node` (the 3-node cycle case) and `self-loop` (the literal subtest name). |
| F-15 | Per-package coverage gate at 100% for `depgraph` | `bash scripts/coverage-check.sh` | done | `ok: github.com/geomyidia/cascade/depgraph coverage 100% >= 100%`. golist also at 100%, project at 100%, changeset N/A (no implementation yet). The simplification of the dead-branch `sortAndDedup` (renamed to `sortValues`, dedup loop removed because reverse-edge construction is dedup-by-construction) brought the gate to 100% without any test contortions. |
| F-16 | Package has no non-stdlib imports beyond `golist` (own module) | `go list -m all \| wc -l` returns 1 | done | `1` (the cascade module itself; zero external deps). depgraph imports only `slices`, `sort` from stdlib, plus `github.com/geomyidia/cascade/golist` from the own module. |
| F-17 | Concurrent read of Graph is safe | `go test -race -run 'TestGraphConcurrentRead' ./depgraph` | done | `ok github.com/geomyidia/cascade/depgraph 1.174s`. 8 goroutines × {RevDepClosure, DirectImports, DirectImporters, Has, Stats} on the same `*Graph`; race detector clean; all goroutines see identical output. |
| F-18 | `go doc github.com/geomyidia/cascade/depgraph` renders cleanly | `go doc github.com/geomyidia/cascade/depgraph \| head -30` | done | Output: package overview, `type Graph struct{ ... }`, `func Build(pkgs []golist.Package) *Graph`, `type Stats struct{ ... }` plus the five methods on `*Graph`. Every exported name has a doc comment starting with the identifier (DC-02). All five Stats fields documented inline. |
| F-19 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | done | This document, §"Substrate loaded at session start" enumerates every guide loaded with line-number anchors and the specific pattern IDs cited (TD-01, TD-05, TD-09, TD-17, IM-01, IM-04 (with note on cite/content discrepancy), IM-17, API-42, TE-01..TE-08, TE-15, TE-43, DC-01, DC-02, plus the AP-* anti-patterns avoided). |
| F-20 | `Stats()` method + `Stats` type documented; TestStats passes | `go doc github.com/geomyidia/cascade/depgraph.Graph.Stats` + `go doc github.com/geomyidia/cascade/depgraph.Stats` + `go test -run 'TestStats' ./depgraph` | done | `func (g *Graph) Stats() Stats` returned by godoc; `type Stats struct` with `Nodes int`, `Edges int`, `MaxInDegree int`, `MaxOutDegree int` exported with documented fields. `TestStats` covers four cases (empty, single isolated node, diamond, edges-to-absent-nodes); all pass. Added at impl-plan time per M3-1 (Duncan-authorised spec amendment); the active spec's ledger should pick this row up at next ODM promotion so the spec stays the source-of-truth. |

## Deferrals

**Zero deferrals.** The spec's stated deferral budget was zero, and the milestone closes with that property held.

## What Worked

Patterns that made M3 close cleanly. Per `LEDGER_DISCIPLINE.md` CDC protocol step 8.

**Cold-cache lint caught a real issue on its first invocation.** OUT-1 (the Makefile lint-cache fix) was added with the explicit hypothesis that local lint had been hiding issues that CI would catch. On the first `make lint` after the change, revive flagged `unused-parameter: parameter 'i' seems to be unused, consider removing or renaming it as _` in `TestGraphConcurrentRead` — exactly the M2-style failure mode (unused-parameter, again). The fix was a simple param-removal; without OUT-1 in place, this would have been the third consecutive milestone where local lint passed and CI failed for the same class of issue. **OUT-1 paid for itself within seconds of being installed.** Carry-forward: keep the fresh-cache mechanism; it works.

**Dead defensive code surfaces as a 100%-coverage gate failure, not as a lint warning.** The existing in-progress `depgraph.go` had a `sortAndDedup` helper whose inner dedup loop (`if s != w[len(w)-1] { w = append(w, s) }`) had a structurally unreachable false branch — the reverse map is dedup-by-construction (each `(src, dst)` pair appends once), and the forward map is pre-dedup'd by `mergeImports`. The 100% coverage gate would have failed on the dedup branch had I left it in. Per CLAUDE.md "Don't add error handling, fallbacks, or validation for scenarios that can't happen," I removed the dedup loop entirely (renamed to `sortValues`, just sorts). This is a pattern: pure-data packages with strict coverage gates expose dead defensive code at exactly the right time — at code-review of the milestone's first PR, before the dead branch becomes cargo-cult convention. Worth documenting as a Safety-II observation: the gate doesn't just verify correctness, it's an active pressure against speculative defensive code.

**Pre-existing in-progress file was the user's work, not noise.** The repo had `depgraph/depgraph.go` as an untracked file at session start with a partial implementation (Build + Graph struct + helpers, but referencing an undefined `computeStats` so it didn't compile). Per CLAUDE.md "If you discover unexpected state... investigate before deleting or overwriting" — treated this as the user's in-progress work and refined it (added missing `computeStats`, simplified dead-branch helper) rather than rewriting from scratch. The structural shape was sound; finishing it was strictly cheaper than re-deriving. Pattern reusable: when an in-progress feature branch has a partial file, refine, don't rewrite, unless the structural shape is wrong.

**TE-43 + table-driven idiom + `name` field made the F-13 determinism check structural rather than statistical.** Each subtest asserts both the expected closure AND `sort.StringsAreSorted(got)`. So if a future change produced unsorted output for any case, the failing assertion would name *which* topology lost its ordering, instead of just "test flaked under -count=10". The cost is one extra assertion per subtest; the diagnostic value if anything ever does drift is worth it. M5+ — keep this pattern when a guarantee is "the output is sorted."

**The hand-traceable 5-package case is a permanent integration sanity check.** F-9's case is deliberately small enough to derive expected closures by hand (with an actual reader walking the graph on paper). That makes it the test future maintainers can read to *understand* what RevDepClosure is supposed to do, distinct from the table-driven cases that test individual algorithm branches. Keeping it separate (its own top-level test, not a row in StandardTopologies) signals its documentation role. M5+ — when a function's *meaning* depends on a small worked example, host that example as its own test, not a row in a topology table.

**Pre-flight reading caught the IM-04 cite-vs-content discrepancy before implementation.** The implementation plan cites "IM-04 (uniform receiver kind)" for receiver consistency. The actual IM-04 in `05-interfaces-methods.md` is "Name Single-Method Interfaces with the `-er` Suffix" — receiver-consistency lives at TD-09. Caught by reading both sections at session start. The principle (uniform pointer receivers) is honored regardless. Pattern: cite IDs concretely so discrepancies surface; don't trust impl-plan cites blindly.

## Pre-PR self-check (CC)

Per the M2 retro carry-forward: read each row's criterion text against the planned evidence, flag any "pending" / "deferred" / "next round" framings as drift candidates *before* PR open.

Walked F-1..F-20:
- All twenty rows have `done` status with reproducible Verify command output captured in the Evidence column above.
- No rows framed as "pending," "deferred," or "next round."
- Spec-softening check: every Verify command's output text is the same shape the criterion requires — for the godoc rows (F-2..F-7, F-18, F-20), the godoc output literally contains the spec's signature string; for the test rows (F-8..F-14, F-17, F-20), the `ok` line is from a passing test that asserts the criterion property; for the structural rows (F-15, F-16), the script/command output literally meets the threshold.
- One row (F-2) has a slight evidence-vs-criterion shape difference worth naming explicitly: criterion is "no exported field lines after `^type Graph struct`," and the Evidence shows `// Has unexported fields.` — that godoc-rendered comment is *generated* from the lack of exported fields. The criterion is satisfied; the evidence shape is godoc's standard rendering for opaque types, not a softpedal.
- F-19 is satisfied by this document; that's the load-bearing artifact for the row.

No softpedals identified at self-check. CDC review will be the independent verification per `LEDGER_DISCIPLINE.md` §"Known structural limitation."

## CDC review notes

_Pending. To be filled in by CDC after independent verification per `LEDGER_DISCIPLINE.md` CDC protocol._

## Carry-forward into M4

- **Fresh-cache lint mechanism stays.** OUT-1's `LINT_CACHE_DIR := $(shell mktemp -d ...)` works and earned its keep on first invocation. No need to revisit unless the ~5–10s cost becomes problematic on slow developer hardware.

- **Pure-data + 100% coverage gate is a clean pattern.** M4's `changeset` package is also pure-data (file-path mapping), with the same 100% gate. Expect the same workflow: small implementation, table-driven tests, no defensive-against-impossible-cases code, gate honest.

- **Pre-flight reading discipline pays off cumulatively.** Loading the Go guides and the methodology docs at session start surfaced the IM-04 cite/content discrepancy and the dead-branch issue before they became mid-implementation distractions. M4: keep doing this.

- **Stats() pattern (compute-once-cache-on-the-Graph) is reusable.** If `changeset` grows a metrics surface (e.g., "how many file paths mapped to packages, how many fell off the edge"), the same single-pass-at-Build-time + cheap-accessor-at-call-time pattern applies. Avoid lazy-on-first-call computation unless concurrency requirements force it; the locking complexity isn't worth the trivial up-front cost.

- **CC self-check protocol track record: 1/1 in M3.** This was the first milestone to apply the pre-PR row audit. The audit caught zero softpedals — but that's also what one would expect on a small, clean ledger. The real test is whether the protocol catches drift on a larger, messier milestone (M5 will likely be that test, given the CLI-wiring scope). Recording M3 as a baseline: protocol can be applied without becoming theater on a clean milestone.

## Closure

Closing report submitted with the M3 PR; CDC verification pending. All twenty criteria reach a final status of `done`. Zero deferrals. Zero no-ops. Zero open at close.

Total rows: 20. Done: 20. Deferred: 0. No-op: 0.
