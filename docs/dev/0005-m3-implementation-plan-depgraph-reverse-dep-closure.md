# Implementation Plan: M3 — `depgraph` (Reverse-Dep Closure)

**Source spec:** `docs/design/05-active/0004-cascade-m3-depgraph-reverse-dep-closure.md`
**Parent plan:** `docs/design/01-draft/0001-cascade-high-level-project-plan.md`
**Predecessor:** M2 — `golist` adapter (merged at origin/main `54e7227`).

## Context

M3 is the algorithmic heart of cascade: pure-data graph construction and reverse-transitive closure traversal over `golist.Package` values. M2 produces the input (typed `[]Package`); M4 will produce the seed set (changed packages); `depgraph` computes the affected set by walking importers backwards.

Properties that distinguish this milestone:
- **Pure data, zero io.** No subprocess invocations, no file reads, no concurrency. Synthetic graphs constructed inline in tests cover every interesting topology.
- **100% coverage gate is genuinely cheap to hit.** No "uncoverable os/exec line" exception like M2's; if the gate reports `<100%`, the missing lines are real.
- **18 (+1) ledger rows; zero deferrals.** Spec sets the budget; no rows expected to slip.
- **No seam needed.** The function-variable seam pattern from M2 doesn't apply here — there's nothing to mock.

The exit signal: `depgraph.Build(pkgs)` returns a `*Graph`; `g.RevDepClosure(seeds)` returns the seeds plus every package that transitively imports any of them, sorted lexicographically; per-package coverage gate at 100% holds.

## Decisions resolved

| # | Spec § | Question | Decision |
|---|---|---|---|
| M3-1 | Q1 (helper API) | Which introspection methods ship in v0.x? | **All four:** `Has`, `DirectImports`, `DirectImporters`, **and `Stats()`**. User overrode the spec's deferral lean on Stats. |
| M3-2 | Q2 (edge merging) | Form edges from `Imports + TestImports + XTestImports`? | **Union all three** (matches spec recommendation). If a package's tests import X and X changes, the package needs re-testing — so test imports are edges. |
| M3-3 | Q3 (missing seeds) | Silent skip vs error vs tuple? | **Silent skip** (matches spec recommendation). Cleanest API for M5's pipeline; missing-seed-as-error mid-pipeline is surprising for the typical cause (stdlib-path-not-in-input). |
| M3-4 | Q4 (`*Graph` vs `Graph`) | Pointer or value receiver type? | **`*Graph`** (matches spec). Internal maps make copy-by-value a deep-copy hazard. |
| M3-5 | Q5 (concurrent-read test shape) | Simple fan-out or stress? | **Simple fan-out + identical-output assertion** under `-race`. Race detector catches actual races; no need for elaborate stress. |
| M3-6 | Q7 (receiver convention) | All `*Graph` receivers, or mixed? | **Uniform `*Graph` pointer receivers** per IM-04. No mixed receivers on a single type. |
| OUT-1 | beyond spec | Lint-cache drift fix carry-forward from M1/M2 retros | **Fold into M3 PR** as a small Makefile change. |

## `Stats` type design (M3-1 follow-on)

```go
// Stats summarises the shape of a Graph. Returned by Graph.Stats.
// Fields are computed once at Build time and cached on the Graph.
type Stats struct {
    Nodes        int // distinct ImportPath nodes in the graph
    Edges        int // forward edges (importer → imported); reverse edges are not double-counted
    MaxInDegree  int // largest reverse-edge count (the most-depended-on package's importer count)
    MaxOutDegree int // largest forward-edge count (the package with the most imports)
}

// Stats returns a summary of the graph's shape. The returned value is
// a copy; callers may freely store or modify it.
func (g *Graph) Stats() Stats
```

Computed in `Build` (single pass over the constructed maps) and stored on the Graph; `Stats()` is a cheap accessor. This keeps the cost of always-computing-stats negligible (the alternative — lazy compute on first call — adds locking complexity for the documented "concurrent-read safe" property).

## Lint-cache fix design (OUT-1)

**Pattern observed across M1 and M2:** local `make lint` passes with a stale `golangci-lint` cache; the same code fails in CI with a cold cache. M1's CDC review flagged it; M1's carry-forward proposed a fix; M1's recommendation wasn't adopted; M2 was bitten by it twice (revive stutter, gosec G204 / revive unused-parameter).

**Resolution shipped with M3:** add `GOLANGCI_LINT_CACHE := $(shell mktemp -d)` to the Makefile, exported into the `lint` recipe's environment so each invocation uses a fresh cache directory. Cost: ~5–10 seconds added to `make lint` per run. Benefit: contributors observe the same lint behaviour CI does.

```make
# Use a fresh cache directory on every `make lint` invocation so local lint
# behaviour matches CI's cold-cache behaviour. Cost: ~5–10s extra per run.
# Justification: the M1 and M2 milestones both shipped lint failures that
# passed locally with stale cache (see retrospectives 0001 and 0002).
LINT_CACHE_DIR := $(shell mktemp -d -t cascade-lint-cache.XXXXXX 2>/dev/null || echo /tmp/cascade-lint-cache)
```

Then in the `lint` recipe: `GOLANGCI_LINT_CACHE=$(LINT_CACHE_DIR) golangci-lint run ./...`.

Validation step in §"Verification" below: deliberately introduce a stale-cache-only-passing lint issue locally, run `make lint`, verify it now fails (confirming the cold-cache effect). Revert before commit.

## Implementation outline

Branch: `m3/depgraph-closure` off `main` (currently at `54e7227`).

### File layout

```
depgraph/
├── doc.go              (M1 stub, untouched — spec F-1)
├── depgraph.go         Graph struct + Build + private helpers
├── methods.go          Has, DirectImports, DirectImporters, RevDepClosure, Stats
├── stats.go            Stats type definition (or fold into methods.go)
├── depgraph_test.go    (package depgraph_test, per TE-43) — public API tests
├── helpers_test.go     (package depgraph_test) — buildTestGraph helper, shared
└── seam_test.go        — not needed; no io seam in this package
```

Whether `methods.go` and `stats.go` are split depends on file length once written. If `methods.go` exceeds ~200 lines, split out `stats.go`; otherwise fold. Decide at write time.

### Public API summary (spec §"Public API surface" with M3-1..M3-6 applied)

```go
package depgraph

type Graph struct { /* opaque; unexported state */ }
type Stats struct { Nodes, Edges, MaxInDegree, MaxOutDegree int }

func Build(pkgs []golist.Package) *Graph

func (g *Graph) RevDepClosure(seeds []string) []string
func (g *Graph) DirectImports(path string) []string
func (g *Graph) DirectImporters(path string) []string
func (g *Graph) Has(path string) bool
func (g *Graph) Stats() Stats
```

### Internal representation

`Graph` holds three pieces of state:
1. `nodes map[string]struct{}` — set of all ImportPath nodes the graph knows about. Used for `Has`, defines what edges target known nodes.
2. `forward map[string][]string` — sorted slice of direct imports per node. Used for `DirectImports` (returns a copy or a pre-sorted slice).
3. `reverse map[string][]string` — sorted slice of direct importers per node. Used for `DirectImporters` and `RevDepClosure`'s BFS.
4. `stats Stats` — pre-computed at `Build` time.

Sorting at construction (rather than at query time) makes the methods O(1) post-Build for slice access, and gives deterministic output without per-call sorts. The trade-off is more work in `Build`; for cascade's scale (M2 F-18: 2422 packages) this is sub-millisecond.

### `RevDepClosure` algorithm

Standard BFS over the reverse-edge map with a visited set:

```
visited := map[string]bool{}
queue := slices.Clone(seeds-that-exist-in-graph)
result := []string{}

for len(queue) > 0:
    n := queue[0]; queue = queue[1:]
    if visited[n]: continue
    visited[n] = true
    result = append(result, n)
    queue = append(queue, reverse[n]...)

sort.Strings(result)
return result
```

Cycle-safe via `visited`. Deterministic via the sort at the end (BFS order would be non-deterministic if multiple seeds are passed, since map iteration order randomises queue prefix).

## Test strategy (per spec §"Test strategy")

All tests in `package depgraph_test`. Helper `buildTestGraph(t, edges)` constructs synthetic graphs from a compact `map[string][]string` representation.

Required test functions and what each covers:

| Test function | Cases | Spec ledger row |
|---|---|---|
| `TestRevDepClosure_StandardTopologies` | 16 cases (empty, single node, linear chain, diamond, cycle, self-loop, disconnected, multi-seed, duplicate seeds, unsorted seeds, etc.) | F-8 |
| `TestRevDepClosure_HandTraceable` | 5-package hand-derived case from the spec | F-9 |
| `TestBuild_TestAndXTestImportsBecomeEdges` | TestImports + XTestImports → reverse edges | F-10 |
| `TestDirectImports` / `TestDirectImporters` | present/absent paths; sort determinism | F-11 |
| `TestHas` | present/absent paths | F-11 |
| `TestBuild_DuplicatePackagesIdempotent` | last-entry-wins per spec contract | F-12 |
| `TestRevDepClosure_Determinism` (or `-count=10`) | 10 runs, all return identical output | F-13 |
| `TestRevDepClosure_TerminatesOnCycles` | cycle + self-loop with `-timeout=10s` | F-14 |
| `TestGraphConcurrentRead` | N goroutines × `RevDepClosure` against same `*Graph` under `-race` | F-17 |
| `TestStats` | computed values for a known graph; pre-computation correctness | (no spec row; user added M3-1) |

Coverage target: **100% statement coverage on `depgraph/`** (F-15). Pure data with no io means there's no inherently-uncoverable line; if the gate reports `<100%`, the missing lines need a test or are dead code.

## Implementation order

Single-PR shape (M3 is small enough to land as one PR; ~400-600 lines impl + tests).

1. **Pre-flight reading** (CC's responsibility per spec §"Required reading"):
   - `assets/ai/go/SKILL.md` index + Critical Rules
   - `assets/ai/go/09-anti-patterns.md` walked
   - `assets/ai/go/04-type-design.md` (TD-01..TD-05, TD-37) noted
   - `assets/ai/go/02-api-design.md` (API-41, API-42) noted
   - `assets/ai/go/05-interfaces-methods.md` (IM-01, IM-04, IM-17) noted
   - `assets/ai/go/07-testing.md` (TE-01..TE-15, TE-43) noted
   - `assets/ai/go/11-documentation.md` (DC-01, DC-02) noted
2. Branch `m3/depgraph-closure` off `main`.
3. Implement `depgraph/depgraph.go` (Graph + Build + internal Stats computation).
4. Implement `depgraph/methods.go` (Has, DirectImports, DirectImporters, RevDepClosure, Stats — plus Stats type def, possibly split into `stats.go`).
5. Tests in `depgraph/depgraph_test.go` (public API per TE-43); helpers in `depgraph/helpers_test.go`.
6. Local verification:
   - `go test -race -count=10 ./depgraph/...` (F-13 + F-14 + F-17)
   - `bash scripts/coverage-check.sh` (F-15)
   - `make check-all` (overall + lint + Makefile threshold)
   - `go doc github.com/geomyidia/cascade/depgraph` (F-18)
7. Add the lint-cache fix to `Makefile` (OUT-1). Validate: introduce a deliberately-stale-passing issue, confirm `make lint` now catches it, revert.
8. Update README milestone table (M2 → completed; M3 → in progress).
9. Commit, push, open PR, monitor CI matrix + lint job.
10. Pre-merge CC self-check on each ledger row's planned-evidence vs criterion text (carry-forward from M2 retro: pre-empt softpedalled rows).

## Critical files

| Path | Action | Notes |
|---|---|---|
| `depgraph/doc.go` | Untouched | M1 stub satisfies F-1 |
| `depgraph/depgraph.go` | Create | Graph type, Build, internal helpers |
| `depgraph/methods.go` | Create | Has, DirectImports, DirectImporters, RevDepClosure, Stats |
| `depgraph/stats.go` | Optional | Split out if methods.go > 200 lines |
| `depgraph/depgraph_test.go` | Create | Public API tests (`package depgraph_test`) |
| `depgraph/helpers_test.go` | Create | `buildTestGraph` helper, shared |
| `Makefile` | Modify | Add `LINT_CACHE_DIR` and use it in the `lint` recipe (OUT-1) |
| `.github/PULL_REQUEST_TEMPLATE.md` | Modify | Add a checklist line: `[ ] Each ledger row's planned evidence text matches the criterion text (per M2 retro carry-forward)`. Promoted from §Risks mitigation to deliverable so the protocol gets installed in the artifact contributors actually see. |
| `README.md` | Modify | M2 → completed; M3 → in progress |

`scripts/coverage-check.sh` already lists `depgraph` in PACKAGES at threshold 100 (M1 carry-over) — no change needed.

## Verification (mapping to spec ledger F-1..F-19)

Each row's Verify command from the spec, with planned evidence shape:

| ID | Verify | Planned evidence |
|---|---|---|
| F-1 | `head -3 depgraph/doc.go` | M1 stub — content untouched; criterion satisfied as a carry-over |
| F-2 | `go doc … Graph` returns one struct line, no exported fields | `type Graph struct{ /* unexported */ }` |
| F-3 | `go doc … Build` shows the spec signature | `func Build(pkgs []golist.Package) *Graph` |
| F-4 | `go doc … Graph.RevDepClosure` | `func (g *Graph) RevDepClosure(seeds []string) []string` |
| F-5 | `go doc … Graph.DirectImports` | `func (g *Graph) DirectImports(path string) []string` |
| F-6 | `go doc … Graph.DirectImporters` | `func (g *Graph) DirectImporters(path string) []string` |
| F-7 | `go doc … Graph.Has` | `func (g *Graph) Has(path string) bool` |
| F-8 | `go test -run TestRevDepClosure_StandardTopologies` | All 16 cases pass |
| F-9 | `go test -run TestRevDepClosure_HandTraceable` | 5-package case passes |
| F-10 | `go test -run TestBuild_TestAndXTestImportsBecomeEdges` | passes |
| F-11 | `go test -run 'TestDirect\|TestHas'` | passes |
| F-12 | `go test -run TestBuild_DuplicatePackagesIdempotent` | passes |
| F-13 | `go test -count=10 -run TestRevDepClosure ./depgraph` | 10 runs, all identical output |
| F-14 | `go test -timeout=10s -run '…cycle\|…self-loop'` | terminates well under timeout |
| F-15 | `bash scripts/coverage-check.sh` | `ok: github.com/geomyidia/cascade/depgraph coverage 100% >= 100%` |
| F-16 | minimal-deps check | `go list -m all \| wc -l` returns `1` (carry M2 F-16's simpler form per spec note) |
| F-17 | `go test -race -run TestGraphConcurrentRead` | passes; race detector clean |
| F-18 | `go doc github.com/geomyidia/cascade/depgraph \| head -30` | useful overview, all exports documented |
| F-19 | closing report names guides + IDs | this plan is the load-bearing artifact for that |
| F-20 | `go doc … Graph.Stats` + `go doc … Stats` | `func (g *Graph) Stats() Stats` returned signature; `type Stats struct{ Nodes int; Edges int; MaxInDegree int; MaxOutDegree int }` exported with documented fields. Plus `go test -run TestStats ./depgraph` passes. Added at impl-plan time per M3-1 (Duncan-authorised spec amendment); the active spec's ledger should pick this row up at next ODM promotion so the spec stays the source-of-truth. |

CC self-check protocol (carry-forward from M2 retro): before opening the PR, walk each row above, read criterion text against planned evidence text, flag any "pending"/"deferred"/"next round" framings as drift candidates. M1 and M2 both shipped a softpedalled row; M3's protection is a pre-PR audit.

## Risks & mitigations

Carry from spec §"Risks & mitigations":

- **Reverse-edge memory cost** — O(V+E) doubled. Trivial at cascade's scale.
- **Sort stability** — `sort.Strings` is deterministic; `-count=10` test catches drift.
- **Cycle detection** — visited set is O(1) per visit; baseline correctness requirement.
- **Concurrent-read drift** — F-17 test exists to catch any accidental mutation introduced post-M3.
- **API extension pressure** — v0.x window allows iteration; lock at v1.0 (CONTRIBUTING.md note pre-v1).

Plan-level additions:

- **Stats pre-computation in `Build`** — adds work to the constructor. Mitigated by single-pass computation; no observable cost at cascade's scale (sub-millisecond on 2422-package graph).
- **Lint cache fix breaks no-mktemp environments** — the `mktemp` shell-out has a fallback to `/tmp/cascade-lint-cache` for portability. Doesn't strictly fail-on-error.
- **CC self-check protocol failure mode** — if the protocol is forgotten again, F-N softpedal recurs. Mitigation: add a checklist line to the PR template's body — "Each ledger row's planned evidence text matches the criterion text (per M2 retro carry-forward)."

## Out of scope (per spec §"Out of scope")

- Forward-transitive closure (small extension; no current need)
- Stdlib / external-module filtering (M5 CLI concern)
- Edge weights / labels (no current need)
- Topological sort / dominators / shortest path / articulation points (not in cascade's problem space)
- Graph mutation after Build (no `AddNode`, `AddEdge`, `Remove*`)
- Graph diff / union (callers should pass union to Build once)
- Visualisation / DOT export (could land later as small extension)
- Error returns from `Build` or `RevDepClosure` (deliberate API choice — narrow contract)

## Open items deferred from this plan to closing-report

- Final pre-PR CC self-check on F-1..F-19 evidence-vs-criterion text.
- Lint-cache fix validation: deliberate-stale-issue test, confirmed `make lint` now catches, reverted.
- Closing-report's "guides loaded + pattern IDs cited" entry (F-19) populated post-implementation.

## Carry-forward expected to land in M3 retrospective

- **CC self-check protocol track record.** M3 will be the first milestone where the pre-PR row-audit is applied. If it catches a softpedalled row, document. If it fails to catch one that CDC then catches, document the failure mode and improve.
- **Lint-cache fix outcome.** Did the cold-cache change actually catch issues that CI would have caught? (Will know if a contributor pushes work that local lint catches — invisible-positive evidence.) Worth a brief retro entry.
- **Stats() acceptance.** User picked it over the spec's deferral lean. Worth noting in retro whether it earned its keep (e.g., did `cascade --debug` plans surface a real consumer in M5 design?).
