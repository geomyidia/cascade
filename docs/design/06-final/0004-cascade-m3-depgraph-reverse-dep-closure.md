---
number: 4
title: "M3: `depgraph` (Reverse-Dep Closure)"
author: "walking importers"
component: All
tags: [change-me]
created: 2026-05-06
updated: 2026-05-07
state: Final
supersedes: null
superseded-by: null
version: 1.0
---

# cascade — M3: `depgraph` (Reverse-Dep Closure)

**Status:** draft. Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md). Predecessor: M2 ([`golist` adapter](./0003-cascade-m2-golist-adapter.md)). Successor: M4 (changed-files-to-packages mapping).

## Goal

Build the `depgraph` package — pure-data graph construction and reverse-transitive closure traversal over `golist.Package` values. No io, no syscalls. This is the algorithmic heart of cascade: M2 produces the input (the parsed package list), M4 produces the seed set (changed packages), and `depgraph` computes the affected set by walking importers backwards.

The exit signal is: `depgraph.Build(pkgs)` returns a `*Graph`; `g.RevDepClosure(seeds)` returns the seeds plus every package that transitively imports any of them, sorted lexicographically; the per-package coverage gate at 100% holds against table-driven tests over synthetic graphs.

## Why M3 is its own milestone

Two reasons. First, this is the only place in cascade that does graph algorithms — keeping it isolated means the next time someone wants a different traversal (forward closure, dominators, biconnected components, etc.), they extend `depgraph` rather than threading graph state through other packages. Second, it's pure-data, which means it's exhaustively testable: synthetic graphs constructed inline in tests cover every interesting topology (linear, diamond, cycle, self-loop, disconnected) without needing fixtures or subprocess machinery. The 100% coverage gate is genuinely cheap to hit and structurally guards against silent algorithmic regressions.

## Required reading (before implementation)

Per the substrate pillar of [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), load the relevant Go guides before writing code. M3's load list is leaner than M2's — no io, no concurrency, no error chains — so the topic-specific set is smaller.

**Load order:**

1. **Index, always:** [`assets/ai/go/SKILL.md`](../../../assets/ai/go/SKILL.md). Document Selection Guide table + Critical Rules.

2. **Anti-patterns, always:** [`assets/ai/go/09-anti-patterns.md`](../../../assets/ai/go/09-anti-patterns.md). Walk the AP-NN list. For M3 specifically, the relevant clusters are around package APIs (AP-16…AP-20-ish — exported-vs-unexported, API stability) and slice/map idioms (the entries about nil-map writes, slice aliasing, and zero-value usefulness).

3. **Topic-specific for M3:**

   - [`assets/ai/go/04-type-design.md`](../../../assets/ai/go/04-type-design.md) — `Graph` is a value type with internal state; the design choices around exported-vs-unexported fields, zero-value usefulness, and pointer-vs-value receivers live here. Load-bearing IDs: **TD-01…TD-05** (zero values), **TD-37** (validated types if `Build` ever needs to reject inputs).

   - [`assets/ai/go/02-api-design.md`](../../../assets/ai/go/02-api-design.md) — the public API surface (`Build`, `Graph`, methods) is the contract. Load-bearing IDs: **API-41** (functional options if extension knobs surface in M3+), **API-42** (no globals; everything threaded through `Build`'s output).

   - [`assets/ai/go/05-interfaces-methods.md`](../../../assets/ai/go/05-interfaces-methods.md) — `Graph` methods take pointer receivers (consistent with `*Graph` returned from `Build`); accept-interfaces-return-concrete applies. Load-bearing IDs: **IM-01** (small interfaces at the consumer boundary — likely none in M3, but verify), **IM-17** (typed-nil traps — `Build` returns `*Graph`, never typed-nil).

   - [`assets/ai/go/07-testing.md`](../../../assets/ai/go/07-testing.md) — pure-data testing is where M3 earns its 100% gate. Load-bearing IDs: **TE-01…TE-15** (table-driven idiom — every test in M3 should be one), **TE-43** (use `package depgraph_test` for the public-API tests so we exercise the contract, not internals).

   - [`assets/ai/go/11-documentation.md`](../../../assets/ai/go/11-documentation.md) — every exported name documented per **DC-01** / **DC-02**. Ledger row F-19 (godoc renders cleanly) gates on this.

4. **Skim only if needed:**

   - `01-core-idioms.md` — declarations, naming, control flow. `golangci-lint` already enforces most.
   - `08-performance.md` — only if profiling reveals the BFS or sort is hot. Closure on the internal-monorepo-scale corpus (2422 packages from M2's F-18 evidence) ran in milliseconds in informal scratch; production scale shouldn't surface optimisation needs.
   - `03-error-handling.md` and `06-concurrency.md` — not load-bearing for M3 (no errors return from `Build`/`RevDepClosure`; no concurrency).

Closing report names the loaded guides and pattern IDs cited.

## Public API surface

Five exported names: one type (`Graph`), one constructor (`Build`), three methods (`RevDepClosure`, `DirectImports`, `DirectImporters`, `Has`). Counting methods as separate names: that's six total. Minimum for cascade plus a small introspection set so library users (per the OSS public-API decision) can navigate the graph without re-loading from `golist.Package`.

### `Graph`

Opaque value type with unexported state. Constructed only via `Build`; not zero-value-useful (the empty `Graph{}` is a valid empty graph but callers should always go through `Build`).

```go
package depgraph

// Graph is a directed import graph constructed from a slice of
// golist.Package values. Edges run importer → imported, mirroring
// `go list -deps`'s view. The Graph also stores reverse edges
// internally so RevDepClosure runs in O(V + E) without rebuilding.
//
// A Graph is immutable after Build returns. Methods are safe for
// concurrent reads from multiple goroutines.
//
// API stability: pre-v1.0, the Graph type is opaque (no exported
// fields, no exported methods beyond those documented here). Internal
// representation may change without notice.
type Graph struct {
    // unexported internal state — node map, edge sets, sorted index
}
```

### `Build`

```go
// Build constructs a Graph from the given slice of packages.
//
// Edge semantics: for each package P, the union of P.Imports +
// P.TestImports + P.XTestImports forms P's outgoing-edge set. This
// merging reflects affected-set intent: if a package's tests import
// X and X changes, the package needs re-testing.
//
// Build never fails — any input produces a valid Graph. Empty input
// yields an empty Graph (zero nodes, zero edges). Duplicate
// ImportPath entries in pkgs are treated as a single node, with the
// last entry's edge set winning (callers should not pass duplicates;
// `go list -deps -json` doesn't produce them).
//
// Build does not validate that every imported path appears as a
// node in pkgs. Stdlib packages typically appear as nodes (go list
// -deps includes them); external packages may or may not, depending
// on the caller's input. Edges to absent nodes are recorded but
// don't add nodes — calling Has on an absent path returns false.
func Build(pkgs []golist.Package) *Graph
```

### `Graph.RevDepClosure`

The headline operation. BFS over reverse edges from a seed set; returns the union (seeds included) sorted lexicographically.

```go
// RevDepClosure returns the seeds plus every package that
// transitively imports any of them — the reverse-transitive closure
// under the imports relation. The output is sorted lexicographically
// for deterministic CI behaviour.
//
// Seeds not present in the graph are silently skipped. (Callers in
// production won't encounter this — `go list -deps` should include
// every package the change-set touches — but the defensive behaviour
// avoids a useless error path.)
//
// Cycle-safe via a visited set. Go's import graph is acyclic by
// language rule, but defensive coding is cheap and protects against
// pathological inputs.
//
// Complexity: O(V + E) over the reachable subgraph. Output sort is
// O(k log k) on the result size.
func (g *Graph) RevDepClosure(seeds []string) []string
```

### `Graph.DirectImports` / `Graph.DirectImporters`

```go
// DirectImports returns the import paths directly imported by path
// (the union of Imports + TestImports + XTestImports of the
// underlying Package). Sorted lexicographically. Returns nil if
// path is not in the graph.
func (g *Graph) DirectImports(path string) []string

// DirectImporters returns the import paths that directly import
// path. Sorted lexicographically. Returns nil if path is not in the
// graph.
func (g *Graph) DirectImporters(path string) []string
```

### `Graph.Has`

```go
// Has reports whether path is a node in the graph.
func (g *Graph) Has(path string) bool
```

## Out of scope (deferred to later milestones or never)

- **Forward-transitive closure** (what does X transitively import?). Not needed by cascade itself; can be added when a real use surfaces. The internal forward-edge map already exists, so this is a small extension.
- **Stdlib filtering / external-module filtering.** `Build` records every package the input contains. M5's CLI may filter the output of `RevDepClosure` to drop stdlib seeds (since stdlib doesn't change in a PR), but that's a CLI-layer concern, not a graph-layer one.
- **Edge weights / labels.** All edges are unweighted and untyped (we don't distinguish "regular import" from "test import" in the graph — the merging happens in `Build`). If a future need requires distinguishing, that's a v2 design.
- **Topological sort, dominators, shortest path, articulation points.** Not in cascade's problem space.
- **Graph mutation after Build.** No `AddNode`, `AddEdge`, `Remove*`. The graph is built-once-then-read.
- **Graph diff or graph union.** No `Merge(other *Graph)`. If a caller wants to combine packages from multiple sources, they should pass the union to `Build` once.
- **Visualisation / DOT output.** Useful for debugging but not in M3's scope. A `graph.go.tmpl`-rendered DOT export could be a small extension later.
- **Error returns.** `Build` and `RevDepClosure` return only their primary output type — no `error`. There is no input shape that fails (empty is fine, duplicates are silently dedup'd, missing seeds are silently skipped). This is a deliberate API choice that keeps the contract narrow.

## Test strategy

Pure-data tests, all table-driven, all in `package depgraph_test` (per TE-43) so the public API contract is what's exercised. Synthetic graphs constructed inline via a small helper.

### Helper: `buildTestGraph`

```go
// buildTestGraph constructs a Graph from a compact map representation
// for use in tests. Keys are import paths; values are direct imports
// (merged into Imports per the Build contract — TestImports/XTestImports
// can be exercised separately when needed).
func buildTestGraph(t *testing.T, edges map[string][]string) *depgraph.Graph {
    t.Helper()
    pkgs := make([]golist.Package, 0, len(edges))
    for path, imports := range edges {
        pkgs = append(pkgs, golist.Package{
            ImportPath: path,
            Imports:    imports,
        })
    }
    return depgraph.Build(pkgs)
}
```

### Test cases (table-driven over synthetic graphs)

The unit-test surface is small enough to enumerate. Each case names the graph topology, the seed set, and the expected closure.

**`TestRevDepClosure_StandardTopologies` cases:**

- `empty graph, no seeds` → `[]`
- `empty graph, one seed` → `[]` (silently skipped)
- `single node, no edges, seed = that node` → `[that node]`
- `single node, no edges, seed = different node` → `[]`
- `linear chain A→B→C, seed = C` → `[A, B, C]`
- `linear chain A→B→C, seed = B` → `[A, B]`
- `linear chain A→B→C, seed = A` → `[A]`
- `diamond A→B, A→C, B→D, C→D, seed = D` → `[A, B, C, D]`
- `diamond, seed = B` → `[A, B]`
- `cycle A→B→C→A, seed = A` → `[A, B, C]` (cycle terminated by visited set)
- `self-loop A→A, seed = A` → `[A]`
- `disconnected components, seed in one component` → only that component
- `multiple seeds in different components` → union of both closures
- `seeds with duplicates` → deduplicated output
- `unsorted seeds` → output still sorted lexicographically
- `5-package hand-traceable case` → see below; per spec exit criterion

**`TestRevDepClosure_HandTraceable` (exit criterion from high-level plan):**

A synthetic 5-package graph constructed inline with hand-derived expected output. Documents the closure semantics for future maintainers and serves as an integration sanity check.

```text
   pkg/a → pkg/b
   pkg/c → pkg/b
   pkg/b → pkg/d
   pkg/e (no imports)

   RevDepClosure([pkg/a]) = [pkg/a]
   RevDepClosure([pkg/b]) = [pkg/a, pkg/b, pkg/c]
   RevDepClosure([pkg/d]) = [pkg/a, pkg/b, pkg/c, pkg/d]
   RevDepClosure([pkg/e]) = [pkg/e]
   RevDepClosure([pkg/a, pkg/e]) = [pkg/a, pkg/e]
   RevDepClosure([]) = []
```

**`TestBuild_TestAndXTestImportsBecomeEdges`:**

A package P with `TestImports = ["t"]` and `XTestImports = ["xt"]` (and empty `Imports`). After `Build`, `g.RevDepClosure([]string{"t"})` includes P; `g.RevDepClosure([]string{"xt"})` includes P. Confirms the Imports + TestImports + XTestImports merging.

**`TestDirectImports` and `TestDirectImporters`:**

Table-driven over a fixed graph; exercise present-path / absent-path cases plus sort determinism.

**`TestHas`:**

Trivial table; present/absent cases.

**`TestBuild_DuplicatePackagesIdempotent`:**

Two `golist.Package` entries with the same `ImportPath`. Verify `Has` returns true once, `g.RevDepClosure` doesn't double-count, and the last entry's edge set wins (matches the documented contract).

### Coverage target

100% statement coverage on `depgraph/`, gated by `scripts/coverage-check.sh`. Pure-data with no io means there's no inherently-uncoverable line; if the gate reports < 100%, the missing lines are real (and either need a test or need to be removed as dead code).

## Acceptance ledger

Per [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md). Every row reaches a final status before M3 closes. CC fills Status + Evidence at the commit each row lands.

| ID | Criterion | Verify (reproducible) | Significance | Status | Evidence | Notes |
|----|-----------|----------------------|--------------|--------|----------|-------|
| F-1 | `depgraph/doc.go` exists with package comment | `test -f depgraph/doc.go && head -3 depgraph/doc.go \| grep -q '^// Package depgraph'` | serious | open | | preserved from M1 |
| F-2 | `Graph` type exported, opaque (no exported fields) | `go doc github.com/geomyidia/cascade/depgraph.Graph \| grep -E '^type Graph struct'` returns one line; no exported field lines after | serious | open | | TD-design discipline |
| F-3 | `Build` signature matches spec | `go doc github.com/geomyidia/cascade/depgraph.Build \| grep -F 'func Build(pkgs []golist.Package) *Graph'` | serious | open | | |
| F-4 | `RevDepClosure` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.RevDepClosure \| grep -F 'func (g *Graph) RevDepClosure(seeds []string) []string'` | serious | open | | |
| F-5 | `DirectImports` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.DirectImports \| grep -F 'DirectImports(path string) []string'` | serious | open | | |
| F-6 | `DirectImporters` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.DirectImporters \| grep -F 'DirectImporters(path string) []string'` | serious | open | | |
| F-7 | `Has` method signature | `go doc github.com/geomyidia/cascade/depgraph.Graph.Has \| grep -F 'Has(path string) bool'` | serious | open | | |
| F-8 | All 16 `TestRevDepClosure_StandardTopologies` cases pass | `go test -run 'TestRevDepClosure_StandardTopologies' ./depgraph` | serious | open | | enumerated in Test strategy |
| F-9 | Hand-traceable 5-package case passes | `go test -run 'TestRevDepClosure_HandTraceable' ./depgraph` | serious | open | | spec exit criterion |
| F-10 | Test/XTest imports become edges | `go test -run 'TestBuild_TestAndXTestImportsBecomeEdges' ./depgraph` | serious | open | | edge-merging contract |
| F-11 | `DirectImports` / `DirectImporters` / `Has` correct | `go test -run 'TestDirect|TestHas' ./depgraph` | serious | open | | |
| F-12 | Duplicate-package handling idempotent | `go test -run 'TestBuild_DuplicatePackagesIdempotent' ./depgraph` | correctness | open | | |
| F-13 | Output is sorted lexicographically; deterministic across runs | `go test -count=10 -run 'TestRevDepClosure' ./depgraph` (10 runs, all pass) | serious | open | | nondeterminism is the bug class |
| F-14 | Cycle / self-loop terminates | `go test -timeout=10s -run 'TestRevDepClosure_StandardTopologies/cycle\|TestRevDepClosure_StandardTopologies/self-loop' ./depgraph` | serious | open | | termination guard |
| F-15 | Per-package coverage gate at 100% for `depgraph` | `bash scripts/coverage-check.sh` | serious | open | | pure-data; no exclusions |
| F-16 | Package has no non-stdlib imports beyond `golist` (own module) | `go list -f '{{.Imports}}' ./depgraph \| tr -d '[]' \| tr ' ' '\n' \| grep -v '^github.com/geomyidia/cascade/golist$' \| xargs -I {} sh -c 'go doc {} 2>/dev/null \| head -1' \| grep -v '^package '` should return nothing | correctness | open | | minimal-deps discipline |
| F-17 | Concurrent read of Graph is safe | `go test -race -run 'TestGraphConcurrentRead' ./depgraph` | correctness | open | | doc claims safety; verify |
| F-18 | `go doc github.com/geomyidia/cascade/depgraph` renders cleanly | `go doc github.com/geomyidia/cascade/depgraph \| head -30` returns useful overview | polish | open | | DC-01 / DC-02 enforced via revive |
| F-19 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | polish | open | | methodology requirement |

**Closure expectations:**

- F-1 through F-15 are CI-verifiable. CC produces green Verify output for each in the closing report.
- F-16 is a structural check; the verify command above is fragile because of shell quoting — CC may simplify (e.g., `[ "$(go list -m all | wc -l | tr -d ' ')" = "1" ]` as in M2 F-16) if it's adequate.
- F-17 needs a small concurrent-read test to be written; the criterion is real but the test isn't named in the design doc — CC should add `TestGraphConcurrentRead` (N goroutines calling `RevDepClosure` with the same seed against the same `*Graph`, asserting identical output).
- F-18 and F-19 are reviewer/closing-report checks.

**Deferral budget:** zero rows expected to defer.

## Risks & mitigations

**Reverse-edge representation memory cost.** Storing both forward and reverse edges doubles the graph's memory footprint. For cascade's problem size (M2's F-18 measured 2422 packages on a real corpus), this is trivial — graph fits in a few MB. Mitigation: none needed. If profiling later surfaces the cost, lazy-build the reverse index.

**Sort stability across Go versions.** `sort.Strings` is documented as deterministic for equal inputs. Mitigation: the `-count=10` test catches any nondeterminism that does sneak in.

**Cycle detection cost.** The visited-set check on every node is O(1) per visit; the BFS already requires a visited set for correctness. No additional cost. Pathological inputs (deeply cyclic graphs, which Go's import system forbids in practice) terminate via the visited set.

**Concurrent-read safety claim drift.** The Graph is documented as safe for concurrent reads. If a future change adds a lazy field or a mutable cache, that property breaks silently. Mitigation: the F-17 test exists to catch this, and the doc comment makes the contract explicit so reviewers notice if a PR introduces mutation.

**API drift from extension pressure.** Once `Has` / `DirectImports` / `DirectImporters` exist, contributors will want `Imports`, `Importers`, `Closure` (forward), `Reverse()` (return reversed graph), etc. Each is small but adds API surface. Mitigation: the v0.x window is the time to iterate; lock the surface at v1.0. Document the v1 commitment in CONTRIBUTING.md when v1 approaches.

## Open questions

These are the calibrations Duncan should weigh in on (or explicitly delegate). Same disclosed-deferral discipline as M2.

1. **`DirectImports` / `DirectImporters` / `Has` — keep all three, or trim?** I've specced them on the rationale that an opaque `Graph` with only `RevDepClosure` is awkward for library users to introspect. The cost of testing all three is small. But each is exported public API surface that v1.0 will commit to. My weak preference: keep — the introspection set is the minimum useful complement to the closure operation. Confirm or trim.

2. **Edge-set merging (Imports + TestImports + XTestImports) — verify the semantics match cascade's intent.** I've specced "yes, merge them all" on the reasoning that an affected-package set should include packages whose *tests* would re-run if X changes. This matches gta's behaviour. Confirm.

3. **Missing-seed handling — silent skip vs error?** I've specced silent skip. Alternative: return a `(closure []string, missing []string)` tuple, or return an error. Silent skip is the friendlier API for cascade's call site (M5 wraps `golist.Run` → `changeset.Resolve` → `g.RevDepClosure` and returns the result; an error mid-pipeline is surprising for "the user passed a path that no package owns"). Confirm.

4. **`Build` returns `*Graph` (pointer) vs `Graph` (value).** I've specced `*Graph` because the Graph holds maps internally and copying would be a deep-copy hazard. Alternative is `Graph` value with `func (g Graph) RevDepClosure(...)` receivers; copying is then explicit if the user wants their own copy. Pointer is cleaner for cascade's use; confirm or override.

5. **Concurrent-read test (F-17) shape.** Should it be a single goroutine-fan-out test (N callers of `RevDepClosure` against the same `*Graph`) or a more elaborate stress test? My recommendation: simple fan-out + identical-output assertion is enough; race detector catches any actual races. Confirm scope.

6. **Should `depgraph` add a `Stats() Stats` method** returning node/edge counts, max in-degree, etc.? Useful for `cascade --debug` later; not strictly needed for M3. Lean: defer to M3+ unless there's a real need now.

7. **Method receiver convention.** All methods take `*Graph` pointer receivers. Should `Has` (pure read, no modification) take a value receiver `(g Graph)` instead? Convention-wise, mixed receivers on the same type is anti-pattern (IM-04 in `go-guidelines`). Recommend uniform `*Graph`. Confirm.

## Cross-references

- Parent plan: [`0001-cascade-high-level-project-plan.md`](./0001-cascade-high-level-project-plan.md), §"M3 — Dep graph + reverse-dep index + closure".
- Predecessor: M2 design ([`docs/design/05-active/0003-cascade-m2-golist-adapter.md`](../05-active/0003-cascade-m2-golist-adapter.md)) and M2 implementation plan ([`docs/dev/0002-implementation-plan-project-build-info-fallback-m2-golist-adapter.md`](../../dev/0002-implementation-plan-project-build-info-fallback-m2-golist-adapter.md)).
- Methodology: [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`assets/ai/LEDGER_DISCIPLINE.md`](../../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/SUBAGENT-DELEGATION-POLICY.md`](../../../assets/ai/SUBAGENT-DELEGATION-POLICY.md).
- Go canon for pure-data graph work: `go-guidelines` skill, especially `04-type-design.md` (TD-01…TD-05 zero-value usefulness, TD-37 validated types), `02-api-design.md` (API-41 functional options if knobs surface, API-42 DI over globals), `05-interfaces-methods.md` (IM-01 small interfaces, IM-04 receiver consistency, IM-17 typed-nil), `07-testing.md` (TE-01…TE-15 table-driven, TE-43 external test package), `09-anti-patterns.md` (full walk; M3-relevant clusters around slice/map/sort idioms), `11-documentation.md` (DC-01, DC-02).
- M2 closure evidence (informs scale assumptions): F-18 measured 2422 packages on a real corpus in ~3 seconds for `golist.Run` alone; `depgraph.Build` + `RevDepClosure` should be milliseconds-to-tens-of-milliseconds at that scale.
