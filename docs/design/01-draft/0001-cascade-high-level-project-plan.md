---
number: 1
title: "cascade — High-Level Project Plan"
author: "Go projects"
component: All
tags: [change-me]
created: 2026-05-06
updated: 2026-05-06
state: Draft
supersedes: null
superseded-by: null
version: 1.0
---

# cascade — High-Level Project Plan

**Status:** draft (CDC-authored). Canonical home TBD per `${SPEC_HOME}`.

## Problem Statement

Many Go project CI workflow needs affected-package test selection so that PRs only run tests for the packages they touch (plus reverse-dependency closure), with the full suite reserved for merge-queue events. The end-state benefit is a 3-10× CI speedup on typical PRs. The larger the project, the more modules it has, the more critical this need.

Prior to Go 1.25, the preferred solution for this was DigitalOcean's [`gta`](https://github.com/digitalocean/gta). Recent investigation after a Go 1.25 upgrade surfaced a hard blocker: gta v0.2.0 + v0.1.2 fail silently against Go 1.25.x. The failure surface is `golang.org/x/tools/go/packages.Load` — that loader's stricter module-resolution emits "go: updates to go.mod needed" against a module that the regular `go list` family considers tidy. gta swallows the error and exits 0 with an empty package list. The named fallbacks in the strategic plan (`jharlap/affected`, hand-rolled bash) carry the same risk profile (`packages.Load` reuse) or have unacceptable engineering trade-offs (bash for critical infra has no type system, no test framework, awkward error handling — exactly the failure-mode invitation that just bit us with gta).

Go projects need a Go-based, hand-rolled, minimal-scope affected-package selector. It must avoid the `packages.Load` library that bit gta and instead must shell out to `go list -deps -json` directly (which has been verified to work with newer Go releases). It must be fully tested at the non-main / non-io boundary so the silent-failure-mode that bit us with gta is structurally impossible. It must live in a dedicatd repo and be consumed as a binary dependency (`go install <module>@<version>`) by Go projects with this need.

## Solution Overview

A small Go CLI tool that, given a base ref and a head ref (or a list of changed files via stdin), prints the set of Go packages affected by the change. "Affected" means: any changed package's reverse-dependency closure under the build-tag union the caller specifies.

Architecture is two layers:

- **Pure core:** typed dep-graph traversal, change-set classification, build-tag handling. No io. 100% test coverage via table-driven tests over synthetic graphs.
- **Thin io shell:** `os/exec` to `go list -deps -json -tags=<union> ./...` plus `os/exec` to `git diff --name-only <base>..<head>` for change-set resolution. Streaming JSON parser over `go list` stdout. Real error handling — every io error is returned and surfaced; never swallowed (the gta failure mode).

The tool is consumed by a Github Actions CI workflow at install time:

```yaml
# .github/workflows/ci.yml in org/consuming-project
- name: Cache ${BINARY_NAME} binary
  uses: actions/cache@v4
  with:
    path: ~/go/bin/${BINARY_NAME}
    key: ${BINARY_NAME}-${{ runner.os }}-${INITIAL_VERSION}
- id: affected
  if: steps.paths.outputs.wide != 'true'
  run: |
    if [[ ! -f ~/go/bin/${BINARY_NAME} ]]; then
      go install ${MODULE_PATH}/cmd/${BINARY_NAME}@${INITIAL_VERSION}
    fi
    PKGS=$(~/go/bin/${BINARY_NAME} \
      --tags=parallel_safe,one_at_a_time,integration_test,common_testing \
      --base=origin/${{ github.base_ref }} \
      --head=HEAD)
    # ... emit outputs same shape as the gta step ...
```

The wiring shape matches what gta provided — only the binary name and module path change. This provides a pain-free migration path for users of gta who have hit the same limitation we have.

## Variables

Substitute these placeholder names with concrete values before the plan goes canonical. Unset placeholders should fail any "is this plan ready to execute?" check.

| Variable | Description | Default | Owner |
|---|---|---|---|
| `${PROJECT_NAME}` | Human-readable project name; used in README, release notes, and informal references. | TBD | Duncan |
| `${BINARY_NAME}` | The CLI binary name installed by `go install`. Typically matches `${PROJECT_NAME}` lowercased. | TBD | Duncan |
| `${REPO_URL}` | The git URL of the new repo, e.g. `github.com/<org>/<name>`. | TBD | Duncan |
| `${MODULE_PATH}` | The Go module path declared in `go.mod`. Almost always identical to `${REPO_URL}` minus the protocol prefix. | TBD | Duncan |
| `${SPEC_HOME}` | Where the detailed implementation spec lives (in the new repo's `docs/` dir). | TBD | Duncan |
| `${LICENSE_CHOICE}` | Open-source license, or `UNLICENSED` if private. Affects `LICENSE` file at root. | TBD (recommend MIT to match `gta` precedent unless private) | Duncan |
| `${INITIAL_VERSION}` | First tagged release for production use. | `v0.1.0` (conventional) | Duncan |
| `${GO_TOOLCHAIN_PIN}` | Go version pinned in `go.mod`. Must be ≤ 1.25.x. | `1.25.0` (or whatever api repo currently pins) | Duncan |
| `${COVERAGE_THRESHOLD}` | Coverage gate enforced in CI for the non-main / non-io packages. | `100` (per Duncan's directive in the proposal) | Duncan |
| `${CANONICAL_HOME}` | Where this very project plan lives once moved out of `workbench/`. Likely `${REPO_URL}/blob/main/docs/project-plan.md` once the repo exists. | TBD | Duncan |

Defaults marked "(conventional)" or "(recommend X)" are CDC suggestions; Duncan can override.

## Out of scope (the minimal-scope discipline)

Things this tool deliberately does NOT do, so the scope stays bounded:

- **No file-system watch mode.** CI invokes once per run; no daemon.
- **No on-disk caching of the dep graph.** CI starts fresh each run; saved-state correctness is harder than recomputing.
- **No public library API.** CLI only. The pure-core packages are `internal/`-scoped.
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
- `git init` of `${REPO_URL}` with `${LICENSE_CHOICE}` and a stub `README.md` describing the tool's purpose.
- `go.mod` with `${MODULE_PATH}` + `go ${GO_TOOLCHAIN_PIN}`.
- Initial directory layout: `cmd/${BINARY_NAME}/main.go` (placeholder that prints the version string and exits 0); `internal/golist/` (empty for M2); `internal/depgraph/` (empty for M3); `internal/changeset/` (empty for M4).
- `.github/workflows/ci.yml` with steps: `go fmt -d`, `go vet ./...`, `go test ./...`, and a coverage check that fails if non-main/non-io packages drop below `${COVERAGE_THRESHOLD}`.
- `.gitignore` excluding common Go artifacts (binaries, coverage profiles, IDE files).
- Convention notes in `CONTRIBUTING.md` or in-`README.md`: testify/require for tests, table-driven test layout, file ordering, test functions before helpers, var blocks, etc.

**Exit criteria:**
- Repo is cloneable, `go build ./...` succeeds, `go test ./...` passes (with no tests yet, that's a no-op pass), CI workflow runs green on the first push.
- Coverage check fires correctly (verify by intentionally adding an untested function in a follow-up commit, watching CI fail, then reverting).

**Dependencies:** none.

### M2 — `go list` adapter (shell-out + JSON parser)

**Complexity:** M (2-3 days).
**Goal:** Build the io shell that converts `go list -deps -json -tags=<union> ./...` output into a typed `[]Package` slice. This is the only place in the tool that talks to `go`.

**Deliverables:**
- `internal/golist/` package with:
  - A `Package` struct mirroring the relevant subset of `go list`'s JSON output (`ImportPath`, `Imports`, `TestImports`, `XTestImports`, `Dir`, `GoFiles`, `TestGoFiles`, `XTestGoFiles`, build-tag-related fields).
  - A `Run(ctx context.Context, tags []string, patterns []string) ([]Package, error)` function that shells out to `go list` and returns the parsed result.
  - A streaming JSON decoder (since `go list -json` emits a stream of objects, not an array — `json.NewDecoder` with `Decode()` in a loop).
  - Real error handling: shell-exit-non-zero returns a typed error with stderr captured; JSON-parse failures return a typed error with the offending payload; never silently empty.
- 100% coverage on the parser using fixture JSON files in `testdata/`. At minimum: a single-package fixture, a multi-package fixture with imports, a fixture with build-tag-affected GoFiles, an empty fixture.
- One smoke integration test that shells out to `go list` against a tiny embedded test module (under `internal/golist/testdata/sample-module/`) and confirms the parsing pipeline works end-to-end against real `go list` output.

**Exit criteria:**
- `Run()` returns sane data for the embedded sample module.
- Coverage gate passes (100% on `internal/golist/` excluding the `Run()`-shells-out-to-`go list` line, which is the io boundary).
- Manual sanity-check: invoke `Run()` against the a Go project's repo with the four-tag union; confirm non-empty `[]Package` returned. (This is the verification gate that distinguishes this tool from gta — proving on the actual codebase that bit gta.)

**Dependencies:** M1.

### M3 — Dep graph + reverse-dep index + closure

**Complexity:** S-M (1-2 days).
**Goal:** Pure-data graph algorithms over `[]Package`. Build a forward dep graph, reverse it, expose closure traversal.

**Deliverables:**
- `internal/depgraph/` package with:
  - A `Graph` type: `map[ImportPath]*Node` where `Node` carries direct-imports and direct-importers.
  - A `Build(packages []golist.Package) *Graph` constructor.
  - A `Graph.RevDepClosure(seeds []ImportPath) []ImportPath` method: BFS over reverse edges from the seed set, returning the union (seeds included).
  - Sorted output for determinism (sort by import path lexicographically).
  - Cycle-safety (use a visited set; Go's import graph is acyclic but defensive coding is cheap).
- 100% coverage with table-driven tests over synthetic graphs. Cases: single package no deps; linear chain; diamond (A imports B and C; both import D); cycle (defensively handled even though shouldn't occur); empty graph; seeds missing from graph (return empty or error?).

**Exit criteria:**
- Coverage gate passes 100% on `internal/depgraph/`.
- Hand-traceable test case: synthetic 5-package graph; given seed `pkg/a`, returns the correct closure.

**Dependencies:** M1, M2 (uses `golist.Package`).

### M4 — Changed-files-to-packages mapping

**Complexity:** S (1 day).
**Goal:** Convert a list of changed file paths into the set of packages those files belong to.

**Deliverables:**
- `internal/changeset/` package with:
  - A `Resolve(changedFiles []string, packages []golist.Package) []ImportPath` function.
  - File-to-package mapping: each Go file maps to its containing package via `Dir` lookup; non-Go files map to nothing (returned set excludes them).
  - Test files (`_test.go`) map to the same package as non-test files for affected-set purposes.
  - External-test files (`_test.go` with `package foo_test`) ALSO map to the regular package — affecting either the package or its external test reflects the same "tests should re-run" intent.
  - Sorted output for determinism.
- 100% coverage with table-driven tests. Cases: pure-Go change in one package; change spanning two packages; non-Go file changes only (returns empty); _test.go files; mixed Go and non-Go.

**Exit criteria:**
- Coverage gate passes 100% on `internal/changeset/`.
- A change-set that includes a file outside any package returns the right thing (empty or skipped, not an error).

**Dependencies:** M1, M2.

### M5 — CLI + main wiring

**Complexity:** S (1 day).
**Goal:** Wire M2 + M3 + M4 into a usable CLI binary. Real flag parsing, real error handling, real exit codes.

**Deliverables:**
- `cmd/${BINARY_NAME}/main.go` with:
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
- `${BINARY_NAME} --help` prints the flag reference cleanly.
- Manual end-to-end test: invoke against a consuming repo with the four-tag union and a real PR's range; confirm non-empty, sensible package list. This is the same sanity-check from M2 + the integration of all the pure-core pieces.
- `go install ${MODULE_PATH}/cmd/${BINARY_NAME}@<HEAD>` succeeds and produces a working binary.

**Dependencies:** M1, M2, M3, M4.

### M6 — `${INITIAL_VERSION}` release

**Complexity:** S (half day).
**Goal:** Tag, release, document. Make the tool installable via `go install ${MODULE_PATH}/cmd/${BINARY_NAME}@${INITIAL_VERSION}`.

**Deliverables:**
- Git tag `${INITIAL_VERSION}` on the main branch.
- Release notes describing: scope, dependency on Go ≥ `${GO_TOOLCHAIN_PIN}`, usage example, flag reference, exit code table, "this is v0.x — no API stability guarantees yet."
- README pointing at the release.
- Verify `go install ${MODULE_PATH}/cmd/${BINARY_NAME}@${INITIAL_VERSION}` succeeds from a clean GOPATH.

**Exit criteria:**
- The tag exists, points at a commit that builds cleanly, and `go install` works against the public module URL.
- Release notes are linked from the README.

### External Testing / Verification

**Complexity:** S (half day).
**Goal:** Re-point an existing Go repo's `ci.yml` from gta to `${BINARY_NAME}`. **This work happens in THAT project, not in this project's repo** — it's the consumer-side integration

**Deliverables (in consuming project):**
- Replace gta's `actions/cache` step + install step + invoke step in `ci.yml`'s `classify` job with the `${BINARY_NAME}` equivalents.
- Cache key uses `${INITIAL_VERSION}`; install uses `go install ${MODULE_PATH}/cmd/${BINARY_NAME}@${INITIAL_VERSION}`.
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
| M6 | `${INITIAL_VERSION}` release | S | 0.5 |
| Ext | CI re-wiring (in api repo) | S | 0.5 |
| **Total** | | | **7-9 days** |
