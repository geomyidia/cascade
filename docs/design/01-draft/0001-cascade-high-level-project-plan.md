---
number: 1
title: "cascade — High-Level Project Plan"
author: "geomyidia project"
component: All
tags: [ci, go, dependency-graph, affected-tests, build-tooling]
created: 2026-05-06
updated: 2026-05-06
state: Draft
supersedes: null
superseded-by: null
version: 1.0
---

# cascade — High-Level Project Plan

**Status:** draft. Canonical home: [`docs/design/01-draft/0001-cascade-high-level-project-plan.md`](https://github.com/geomyidia/cascade/blob/main/docs/design/01-draft/0001-cascade-high-level-project-plan.md).

## Problem Statement

Many Go project CI workflow needs affected-package test selection so that PRs only run tests for the packages they touch (plus reverse-dependency closure), with the full suite reserved for merge-queue events. The end-state benefit is a 3-10× CI speedup on typical PRs. The larger the project, the more modules it has, the more critical this need.

Prior to Go 1.25, the preferred solution for this was DigitalOcean's [`gta`](https://github.com/digitalocean/gta). Recent investigation after a Go 1.25 upgrade surfaced a hard blocker: gta v0.2.0 + v0.1.2 fail silently against Go 1.25.x. The failure surface is `golang.org/x/tools/go/packages.Load` — that loader's stricter module-resolution emits "go: updates to go.mod needed" against a module that the regular `go list` family considers tidy. gta swallows the error and exits 0 with an empty package list. The named fallbacks in the strategic plan (`jharlap/affected`, hand-rolled bash) carry the same risk profile (`packages.Load` reuse) or have unacceptable engineering trade-offs (bash for critical infra has no type system, no test framework, awkward error handling — exactly the failure-mode invitation that just bit us with gta).

Go projects need a Go-based, hand-rolled, minimal-scope affected-package selector. It must avoid the `packages.Load` library that bit gta and instead must shell out to `go list -deps -json` directly (which has been verified to work with newer Go releases). It must be fully tested at the non-main / non-io boundary so the silent-failure-mode that bit us with gta is structurally impossible. It must live in a dedicatd repo and be consumed as a binary dependency (`go install <module>@<version>`) by Go projects with this need.

## Solution Overview

A small Go CLI tool that, given a base ref and a head ref (or a list of changed files via stdin), prints the set of Go packages affected by the change. "Affected" means: any changed package's reverse-dependency closure under the build-tag union the caller specifies.

Architecture is two layers:

- **Pure core:** typed dep-graph traversal, change-set classification, build-tag handling. No io. 100% test coverage via table-driven tests over synthetic graphs. **Exported as public Go packages** so other Go projects can build their own affected-package tooling on top of cascade's primitives without re-implementing the graph algorithms.
- **Thin io shell:** `os/exec` to `go list -deps -json -tags=<union> ./...` plus `os/exec` to `git diff --name-only <base>..<head>` for change-set resolution. Streaming JSON parser over `go list` stdout. Real error handling — every io error is returned and surfaced; never swallowed (the gta failure mode). The `golist` package is also public — its parsed `[]Package` representation is useful to anyone who's currently fighting `go/packages.Load`.

**API stability:** v0.x carries no API guarantees — pure-core package surfaces will iterate as we learn how downstream consumers use them. v1.0 will commit to standard Go module compatibility (no breaking changes within `v1.x.x`); breaking changes thereafter follow Go's `vN/` directory convention.

The tool is consumed by a Github Actions CI workflow at install time:

```yaml
# .github/workflows/ci.yml in org/consuming-project
- name: Cache cascade binary
  uses: actions/cache@v4
  with:
    path: ~/go/bin/cascade
    key: cascade-${{ runner.os }}-v0.1.0
- id: affected
  if: steps.paths.outputs.wide != 'true'
  run: |
    if [[ ! -f ~/go/bin/cascade ]]; then
      go install github.com/geomyidia/cascade/cmd/cascade@v0.1.0
    fi
    PKGS=$(~/go/bin/cascade \
      --tags=<your,build,tag,union> \
      --base=origin/${{ github.base_ref }} \
      --head=HEAD)
    # ... emit outputs same shape as the gta step ...
```

The wiring shape matches what gta provided — only the binary name and module path change. This provides a pain-free migration path for users of gta who have hit the same limitation we have.

## Variables

Substitute these placeholder names with concrete values before the plan goes canonical. Unset placeholders should fail any "is this plan ready to execute?" check.

| Variable | Description | Value | Owner |
|---|---|---|---|
| `${PROJECT_NAME}` | Human-readable project name; used in README, release notes, and informal references. | `cascade` | Duncan |
| `${BINARY_NAME}` | The CLI binary name installed by `go install`. Typically matches `${PROJECT_NAME}` lowercased. | `cascade` | Duncan |
| `${REPO_URL}` | The git URL of the repo. | `github.com/geomyidia/cascade` | Duncan |
| `${MODULE_PATH}` | The Go module path declared in `go.mod`. Identical to `${REPO_URL}` minus the protocol prefix. | `github.com/geomyidia/cascade` | Duncan |
| `${SPEC_HOME}` | Where the detailed implementation spec lives. Per `odm.toml`, design docs live under `./docs/design/`. | `github.com/geomyidia/cascade/tree/main/docs/design/` | Duncan |
| `${LICENSE_CHOICE}` | Open-source license. Affects `LICENSE` file at root. | `Apache-2.0` (already chosen; `LICENSE` committed) | Duncan |
| `${INITIAL_VERSION}` | First tagged release for production use. | `v0.1.0` (conventional) | Duncan |
| `${GO_TOOLCHAIN_PIN}` | Go floor version pinned in `go.mod`'s `go` directive. Tool must build and run on this version and all forward-compatible later versions. | `1.25.3` (floor; CI also tests on 1.26.x) | Duncan |
| `${COVERAGE_THRESHOLD}` | Coverage gate enforced in CI for the non-main / non-io packages. | `100` | Duncan |
| `${CANONICAL_HOME}` | Where this project plan lives. | `github.com/geomyidia/cascade/blob/main/docs/design/01-draft/0001-cascade-high-level-project-plan.md` | Duncan |

## Out of scope (the minimal-scope discipline)

Things this tool deliberately does NOT do, so the scope stays bounded:

- **No file-system watch mode.** CI invokes once per run; no daemon.
- **No on-disk caching of the dep graph.** CI starts fresh each run; saved-state correctness is harder than recomputing.
- **No configuration files.** Flags only.
- **No plugins or extensibility hooks.**
- **No detection of non-Go inputs** (config files, schemas, generators). That's `paths-filter`'s job in the consumer workflow.
- **No automatic build-tag inference.** Caller passes the tag union explicitly via `--tags`.
- **No JSON or alternate output formats.** Newline-separated package paths to stdout, ready for `go test` to consume.

If the tool's non-test LOC exceeds ~500, it has scope-crept. The pure-core parts of this problem fit in 200-300 LOC; the io shell + main is another 100-150.

## Milestones

Five internal milestones plus one external integration. Each milestone is a logical shipping unit with its own PR (or set of PRs) and review cycle. Total estimated time: 8-10 engineering days.

### M1 — Repo scaffold + CI baseline

**Complexity:** S (1 day).
**Goal:** Establish the repo, license, module structure, and CI gates so subsequent milestones can land code without rebuilding plumbing each time.

**Deliverables:**
- `git init` of `github.com/geomyidia/cascade` with Apache-2.0 `LICENSE` and a stub `README.md` describing the tool's purpose. *(Done: repo exists, `LICENSE` committed, `README.md` carries the tagline.)*
- `go.mod` with `github.com/geomyidia/cascade` + `go 1.25.3`.
- Initial directory layout: `cmd/cascade/main.go` (placeholder that prints the version string and exits 0); top-level public packages `golist/` (empty for M2), `depgraph/` (empty for M3), `changeset/` (empty for M4). `internal/` is reserved for CLI-only glue that emerges in M5 — not used in M2-M4.
- `.github/workflows/ci.yml` with:
  - Go-version matrix: `[1.25.3, 1.26.x]` (floor + latest currently-supported major). 1.24 and 1.23 are out of Go's two-newest-major support window; cascade isn't needed on them anyway since gta works pre-1.25.
  - Steps: `go fmt -d`, `go vet ./...`, `go test ./...`, and a coverage check that fails if non-main/non-io packages drop below `100`.
  - Runs on PR and push-to-main.
- Branch protection on `main`: require CI green + ≥1 approval before merge. (Specifics deferred to M1's design doc.)
- `.gitignore` excluding common Go artifacts (binaries, coverage profiles, IDE files). *(Done.)*
- Convention notes in `CONTRIBUTING.md`: lean on the Go best-practices set in [`assets/ai/go/`](../../../assets/ai/go/) (symlinked into the repo). On top of that base: `testify/require` for assertions, table-driven test layout, file ordering with test functions before helpers, var-block grouping, and the coverage discipline documented in [`assets/ai/CLAUDE-CODE-COVERAGE.md`](../../../assets/ai/CLAUDE-CODE-COVERAGE.md).

**Exit criteria:**
- Repo is cloneable, `go build ./...` succeeds, `go test ./...` passes (with no tests yet, that's a no-op pass), CI workflow runs green on the first push.
- Coverage check fires correctly (verify by intentionally adding an untested function in a follow-up commit, watching CI fail, then reverting).

**Dependencies:** none.

### M2 — `go list` adapter (shell-out + JSON parser)

**Complexity:** M (2-3 days).
**Goal:** Build the io shell that converts `go list -deps -json -tags=<union> ./...` output into a typed `[]Package` slice. This is the only place in the tool that talks to `go`.

**Deliverables:**
- `golist/` package with:
  - A `Package` struct mirroring the relevant subset of `go list`'s JSON output (`ImportPath`, `Imports`, `TestImports`, `XTestImports`, `Dir`, `GoFiles`, `TestGoFiles`, `XTestGoFiles`, build-tag-related fields).
  - A `Run(ctx context.Context, tags []string, patterns []string) ([]Package, error)` function that shells out to `go list` and returns the parsed result.
  - A streaming JSON decoder (since `go list -json` emits a stream of objects, not an array — `json.NewDecoder` with `Decode()` in a loop).
  - Real error handling: shell-exit-non-zero returns a typed error with stderr captured; JSON-parse failures return a typed error with the offending payload; never silently empty.
- 100% coverage on the parser using fixture JSON files in `testdata/`. At minimum: a single-package fixture, a multi-package fixture with imports, a fixture with build-tag-affected GoFiles, an empty fixture.
- One smoke integration test that shells out to `go list` against a tiny embedded test module (under `golist/testdata/sample-module/`) and confirms the parsing pipeline works end-to-end against real `go list` output.

**Exit criteria:**
- `Run()` returns sane data for the embedded sample module.
- Coverage gate passes (100% on `golist/` excluding the `Run()`-shells-out-to-`go list` line, which is the io boundary).
- Manual sanity-check: invoke `Run()` against the a Go project's repo with the four-tag union; confirm non-empty `[]Package` returned. (This is the verification gate that distinguishes this tool from gta — proving on the actual codebase that bit gta.)

**Dependencies:** M1.

### M3 — Dep graph + reverse-dep index + closure

**Complexity:** S-M (1-2 days).
**Goal:** Pure-data graph algorithms over `[]Package`. Build a forward dep graph, reverse it, expose closure traversal.

**Deliverables:**
- `depgraph/` package with:
  - A `Graph` type: `map[ImportPath]*Node` where `Node` carries direct-imports and direct-importers.
  - A `Build(packages []golist.Package) *Graph` constructor.
  - A `Graph.RevDepClosure(seeds []ImportPath) []ImportPath` method: BFS over reverse edges from the seed set, returning the union (seeds included).
  - Sorted output for determinism (sort by import path lexicographically).
  - Cycle-safety (use a visited set; Go's import graph is acyclic but defensive coding is cheap).
- 100% coverage with table-driven tests over synthetic graphs. Cases: single package no deps; linear chain; diamond (A imports B and C; both import D); cycle (defensively handled even though shouldn't occur); empty graph; seeds missing from graph (return empty or error?).

**Exit criteria:**
- Coverage gate passes 100% on `depgraph/`.
- Hand-traceable test case: synthetic 5-package graph; given seed `pkg/a`, returns the correct closure.

**Dependencies:** M1, M2 (uses `golist.Package`).

### M4 — Changed-files-to-packages mapping

**Complexity:** S (1 day).
**Goal:** Convert a list of changed file paths into the set of packages those files belong to.

**Deliverables:**
- `changeset/` package with:
  - A `Resolve(changedFiles []string, packages []golist.Package) []ImportPath` function.
  - File-to-package mapping: each Go file maps to its containing package via `Dir` lookup; non-Go files map to nothing (returned set excludes them).
  - Test files (`_test.go`) map to the same package as non-test files for affected-set purposes.
  - External-test files (`_test.go` with `package foo_test`) ALSO map to the regular package — affecting either the package or its external test reflects the same "tests should re-run" intent.
  - Sorted output for determinism.
- 100% coverage with table-driven tests. Cases: pure-Go change in one package; change spanning two packages; non-Go file changes only (returns empty); _test.go files; mixed Go and non-Go.

**Exit criteria:**
- Coverage gate passes 100% on `changeset/`.
- A change-set that includes a file outside any package returns the right thing (empty or skipped, not an error).

**Dependencies:** M1, M2.

### M5 — CLI + main wiring

**Complexity:** S (1 day).
**Goal:** Wire M2 + M3 + M4 into a usable CLI binary. Real flag parsing, real error handling, real exit codes.

**Deliverables:**
- `cmd/cascade/main.go` with:
  - Flag parsing using stdlib `flag`
  - Required flags:
    - `--tags string` (comma-separated build-tag union; e.g., `parallel_safe,one_at_a_time,integration_test,common_testing`)
    - `--base string` (base git ref, e.g., `origin/main`)
    - `--head string` (head git ref, e.g., `HEAD`)
    - `--help`
    - `--version`
  - Optional flags:
    - `--changed-files -` (read changed files from stdin instead of running `git diff`; useful for testing and for callers who already have the list)
    - `--root .` (override the working directory for `go list`; defaults to `.`)
  - Wire pipeline: resolve changed files (via `git diff --name-only ${base}..${head}` or stdin) → invoke `golist.Run` → `depgraph.Build` → `changeset.Resolve` → `Graph.RevDepClosure` → print package list, one per line, sorted.
  - Exit codes:
    - `0` — success (output may be empty if no Go files changed; that's still success).
    - `1` — flag-parse error.
    - `2` — `git diff` failed.
    - `3` — `go list` failed.
    - `4` — internal logic error (should not occur; surface as a real bug).
  - **Critical invariant: NEVER swallow errors.** Every io call's error is wrapped + returned. The whole point of this tool is structural rejection of gta's silent-failure mode.
- README updated with usage examples + flag reference + exit code table.

**Exit criteria:**
- `cascade --help` prints the flag reference cleanly.
- Manual end-to-end test: invoke against a consuming repo with that repo's build-tag union and a real PR's range; confirm non-empty, sensible package list. This is the same sanity-check from M2 + the integration of all the pure-core pieces.
- `go install github.com/geomyidia/cascade/cmd/cascade@<HEAD>` succeeds and produces a working binary.

**Dependencies:** M1, M2, M3, M4.

### M6 — `v0.1.0` release

**Complexity:** S (half day).
**Goal:** Tag, release, document. Make the tool installable via `go install github.com/geomyidia/cascade/cmd/cascade@v0.1.0`.

**Deliverables:**
- Git tag `v0.1.0` on the main branch.
- Release notes describing: scope, dependency on Go ≥ `1.25.3`, usage example, flag reference, exit code table, "this is v0.x — no API stability guarantees yet."
- README pointing at the release.
- Verify `go install github.com/geomyidia/cascade/cmd/cascade@v0.1.0` succeeds from a clean GOPATH.

**Exit criteria:**
- The tag exists, points at a commit that builds cleanly, and `go install` works against the public module URL.
- Release notes are linked from the README.

### External Testing / Verification

**Complexity:** S (half day).
**Goal:** Re-point an existing Go repo's `ci.yml` from gta to `cascade`. **This work happens in THAT project, not in this project's repo** — it's the consumer-side integration.

**Deliverables (in consuming project):**
- Replace gta's `actions/cache` step + install step + invoke step in `ci.yml`'s `classify` job (or equivalent) with the `cascade` equivalents.
- Cache key uses `v0.1.0`; install uses `go install github.com/geomyidia/cascade/cmd/cascade@v0.1.0`.
- Verify on a real Go-changes PR that the affected list is non-empty and the workflow passes.

**Exit criteria:**
- Consuming project demonstrates a working affected-package step.
- The blocked-on-gta state in a consuming project is resolved.

## Summary table

| Milestone | Scope | Complexity | Estimated days |
|---|---|---:|---:|
| M1 | Repo scaffold + CI baseline | S | 1 |
| M2 | `go list` shell-out + parser | M | 2-3 |
| M3 | Dep graph + reverse-dep closure | S-M | 1-2 |
| M4 | Changed-files-to-packages | S | 1 |
| M5 | CLI + main wiring | S | 1 |
| M6 | `v0.1.0` release | S | 0.5 |
| Ext | CI re-wiring (in consuming project) | S | 0.5 |
| **Total** | | | **7-9 days** |
