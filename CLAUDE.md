# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What cascade is

A Go CLI + library that computes the reverse-transitive closure of a Go change-set under the imports relation. Given a base ref and a head ref, it prints the affected-package set so CI can run only the tests for packages the PR could plausibly have broken. Replaces DigitalOcean's `gta`, which silently failed against Go 1.25's stricter `packages.Load`.

The high-level project plan lives in `docs/design/01-draft/0001-cascade-high-level-project-plan.md`. Per-milestone designs land in `docs/design/05-active/` once active and `docs/dev/` for execution-ready implementation plans.

## Methodology and collaboration practices

Cascade is built per the engineering practices in [`assets/ai/`](assets/ai/) (gitignored; locally-available substrate, not part of the OSS surface). The four files that are load-bearing for day-to-day work:

- `assets/ai/AI-CONSTITUTION-SUPPLEMENT.md` — peer-frame collaboration; rights on both sides; the flag-dissonance discipline (interrupt the work to surface a concern rather than power through).
- `assets/ai/AI-ENGINEERING-METHODOLOGY.md` — three pillars (substrate, posture, process); the 9-point SDLC; CAP-style independent audits; spec-keeping; silent-drop detection; "write to the floor, not the ceiling."
- `assets/ai/LEDGER_DISCIPLINE.md` — per-milestone ledger format. Every acceptance criterion is a row with a grep-verifiable Verify command, a final status (`done` / `deferred` / `no-op`, never `open` at close), and reproducible evidence. Five-iteration cap per milestone.
- `assets/ai/SUBAGENT-DELEGATION-POLICY.md` — embedded below per its own install instructions.

### Roles in this collaboration

- **CC** (Claude Code, this assistant) — implementer and first-line self-assessor. Works against the per-milestone ledger; updates Status/Evidence as work lands; produces a closing report that walks each row individually (per-row, not summary) with the Verify command's actual output.
- **CDC** (a separate Claude instance Duncan brings in) — independent verifier. Re-runs every Verify command against the actual commit state; watches for spec-softening, partial adoption, silent drops. Returns softpedalled `done`s for completion rather than rubber-stamping.

The mature-field discipline this approximates has the recorder of a defect structurally separate from the closer (aviation 14 CFR 121.563; NRC inspector pattern). In our setup CC is both implementer and self-assessor, which is structurally weaker — CDC's independent reproduction is the protection. Trust the protocol over the instinct to report "deviations: none." Per the surgery / aviation literature on ledgers (Pickering 2013, Levy 2012), paper compliance regularly overstates observed compliance by large margins; the per-row walk with evidence is the countermeasure.

### Subagent Delegation Policy

**Work mode — subagent delegation.**

- **Do not delegate thinking work to subagents.** This includes: code edits, design decisions, architecture choices, reasoning about tradeoffs, choosing between options, writing prose for the codebase, judging whether a finding is real, planning a task's structure, evaluating whether something is correct.
- **Subagent delegation is fine for lookup work.** This includes: searching for files or symbols, grepping across a codebase, fetching documentation, listing call sites of a function, reading a file you need but haven't loaded — anything that returns information without requiring judgment about that information.
- **Serial on thinking, parallel on lookup.** Thinking tasks in a multi-task job run one-at-a-time in the main context. Lookup subagents may run in parallel within a task.
- **Quality over elapsed time on the thinking path.** Do not trade thinking quality for wall-clock speed. On the lookup path, parallelism is welcome.
- **Phrasing to follow when planning a task:** *"I will do X thinking/edit work in this context; I may delegate Y lookup if useful."* Both sides explicit. Do not forbid all subagent use (hurts lookup parallelism). Do not leave the line implicit (it won't hold).

### Git Safety Protocol

**Work mode — destructive git operations.**

- **Never run destructive git operations on `main` or any tracking branch without explicit per-occurrence confirmation in chat.** The destructive set: `git reset --hard <ref>`, `git push --force` and `--force-with-lease`, `git clean -fd`, `git branch -D`, `git checkout -- .`, `git restore .`, `git stash drop`, `git rebase -i` with drop/squash on already-pushed commits, `git rm -rf` against tracked files. None of these have a built-in "are you sure?" prompt; all of them can silently destroy work.
- **CC works on feature branches only.** Branch off `main` at the start of a milestone; commits land on the feature branch; the only way to update `main` is via a merged PR. CC never `git checkout main && commit`. CC never `git checkout main && reset`. If CC finds itself on `main` with uncommitted work, the protocol is: `stash` → branch off → `stash pop`.
- **Preservation is the default when state is unexpected.** If `git status` shows a state CC doesn't fully understand — local ahead of origin, unexpected files in the working tree, an unrecognised stash, a detached HEAD, anything that wasn't an explicit consequence of the last few CC operations — *stop and ask*. The non-destructive resolutions almost always exist (push to a backup branch, create a recovery branch from HEAD, ask the user what those commits represent). Reaching for a destructive resolution because "the divergence looks wrong" is the failure mode this protocol exists to prevent.
- **The reflog is a backstop, not a license.** The repo's local `.git/config` extends reflog longevity (90 days unreachable / 365 days reachable; see `scripts/setup-git.sh`). That's a recovery window for when something goes wrong despite the protocol — *not* permission to skip it. There is also a pre-push hook at `scripts/hooks/pre-push` that refuses non-fast-forward and deletion pushes to `main`; that's another backstop, not permission.
- **Phrasing to follow when planning a destructive operation:** if any operation in the destructive set above is in the plan, name it and wait — *"Next step: I'm about to run `git reset --hard origin/main` to undo my last three local commits. OK?"* — and pause until confirmed. Do not bury the destructive command inside a chain of operations; do not assume an earlier "go ahead" extends to a new destructive step.

## Common commands

The `Makefile` is the canonical menu. `make help` prints the full list with descriptions. Frequently used:

```bash
make check          # build + lint + test (the unattended bundle)
make check-all      # check + per-package coverage gate
make test           # go test -race -count=1 ./...
make lint           # gofmt-check + go vet + golangci-lint
make format         # gofmt -w + goimports -w
make build          # produces ./bin/cascade with version-injected ldflags
make build-release  # adds -trimpath -s -w
make coverage-html  # browseable HTML coverage report
make tidy           # go mod tidy
make check-tools    # verify which optional tools are installed
make clean          # rm -rf ./bin + coverage artifacts
```

Run a single test:

```bash
go test -run TestRun ./cmd/cascade/...           # one test by name
go test -run TestRun/version_flag ./cmd/cascade/... # one table-driven subtest
go test -short ./...                              # skip end-to-end binary tests
```

Coverage gates (two layers):

```bash
make coverage-check          # Makefile-level: 90% overall floor
bash scripts/coverage-check.sh   # CI-level: per-package 100% on pure pkgs
```

## Architecture

Three layers, with deliberate boundaries:

### Pure core — public API (`pkg/golist/`, `pkg/depgraph/`, `pkg/changeset/`)

Algorithmic packages. **No io, no syscalls, no `os/exec`** between them. Inputs are typed values; outputs are typed values; tests are table-driven over synthetic graphs. These are exported public APIs — downstreams can build their own affected-package tooling on top of cascade's primitives without re-implementing the graph algorithms.

- `pkg/golist` — typed `Package` values parsed from `go list -deps -json` output, plus a `Run()` function that owns the `os/exec` boundary. **The only place cascade shells out to `go`.** The parsed types are public so callers fighting `golang.org/x/tools/go/packages.Load` can use them directly.
- `pkg/depgraph` — directed import graph + reverse-transitive closure traversal.
- `pkg/changeset` — maps changed file paths to import paths.

These three packages are **gated at 100% statement coverage in CI** by `scripts/coverage-check.sh`. The gate skips packages with no coverage data ("N/A"), so empty pre-implementation stubs don't fire it; once a package gets code, the gate fires immediately.

**Layout deviation note (PS-06).** The substrate's `assets/ai/go/guides/10-project-structure.md` flags `pkg/` as `SHOULD-AVOID` — a community convention rather than a Uber/Google rule, but one the Go standard library and most idiomatic projects don't follow. Cascade adopts `pkg/` deliberately: it makes the public OSS surface explicit, pairs cleanly with `internal/`'s language-enforced privacy boundary, and keeps the substrate of "what's importable by downstreams" obvious at a glance. The deviation is acknowledged in the M2-M5 audit (`docs/dev/0014-go-quality-audit.md` finding F-12) as **Acknowledged-with-rationale**. **Carry-forward:** revisit at v1.0 if a flatter layout becomes feasible without breaking downstream consumers' import paths.

### Private support — module-internal (`internal/project/`)

Build-metadata package; carries cascade-specific Version / GitCommit / GitBranch / BuildDate values populated via `-ldflags` at link time, with a `runtime/debug.ReadBuildInfo` fallback for `go install`-built binaries. The pattern is generic but the values are cascade-specific; Go's `internal/` rule structurally enforces that downstream consumers cannot import this package. Also gated at 100% coverage by `scripts/coverage-check.sh`.

### I/O shell (`cmd/cascade/`)

Wires the pure core to the outside world: argument parsing, `git diff` invocation, `go list` invocation, output formatting. Coverage is **not** gated per-package (would force untestable contortions); behavior is verified by an end-to-end test that builds the binary and runs it.

`main()` is a one-liner that delegates to `func run(args []string, stdout, stderr io.Writer) int`. The pattern matters — it's what makes the CLI testable in-process without an `os.Exit` shim. New CLI work should preserve this split.

**Coverage capture trap (read this once):** subprocess-spawning tests do *not* accumulate coverage for the spawned binary's code. `go test`'s instrumentation only records coverage for code that runs in the test process itself. If you write a test that does `exec.Command("go", "build", …)` and then runs the resulting binary, the binary's `main()` shows 0% covered — even if the test passes. The `run()` indirection above exists specifically so that pure logic *is* exercised in-process and counts toward coverage; the subprocess smoke test is incidental, owned by the integration layer, and not gated.

## Before implementing — required reading

The Go knowledge base at `assets/ai/go/` (symlinked into the repo from upstream infrastructure; gitignored) is the substrate cascade is built on. Per the methodology's substrate pillar, implementing without loading the relevant chapters means rederiving conventions from training-data instinct rather than from the project's source of truth — which is exactly the failure mode the substrate pillar exists to prevent.

Standard load order at the start of any non-trivial milestone:

1. **Index, always:** `assets/ai/go/SKILL.md`. Read the "Document Selection Guide" table and the "Critical Rules" section. The chapters are loaded on demand.
2. **Anti-patterns, always:** `assets/ai/go/09-anti-patterns.md`. Walk the AP-NN list relevant to what you're touching. Cite IDs in commit messages and in the closing report (e.g., *"Replaces string-checked error with `errors.Is` (AP-04)"*).
3. **Topic-specific:** load on demand based on what the milestone touches. The full mapping is in SKILL.md; rough map for cascade:
   - **I/O shells** (`pkg/golist/`, `cmd/cascade/`) — `03-error-handling.md`, `06-concurrency.md`, `02-api-design.md`, `05-interfaces-methods.md`, `07-testing.md`, `11-documentation.md`.
   - **Pure-data packages** (`pkg/depgraph/`, `pkg/changeset/`) — `01-core-idioms.md`, `04-type-design.md`, `07-testing.md`, `11-documentation.md`.

Per-milestone design docs (in `docs/design/`) name the load-bearing chapter list and pattern IDs explicitly. Use those as the authoritative load list for the milestone you're working on.

The closing report names which guides were loaded at session start and which pattern IDs were cited during implementation. This isn't compliance theater — it's the record that lets CDC review against specific patterns and lets future contributors see which conventions were load-bearing for which decisions. The discipline is cumulative when the citations are concrete; it rots when they're hand-wavy.

## Build flags & version injection

The `Makefile` injects build metadata via `-ldflags`:

```
-X main.Version=$(GIT_TAG) -X main.GitCommit=$(GIT_COMMIT)
-X main.GitBranch=$(GIT_BRANCH) -X main.BuildTime=$(BUILD_TIME)
```

The vars are declared in `cmd/cascade/main.go` with sensible defaults (`"dev"` / `"unknown"`) so `go run` and `go install` (without ldflags) still produce sensible output. The `-X` values **must not contain spaces** — the recipe relies on shell double-quoting around the whole `-ldflags` value, with no inner single-quotes. If a future build var could contain spaces (it shouldn't — these are commit hashes, ISO-8601 timestamps, ref names), the quoting strategy needs revisiting.

`make build-release` adds `-trimpath -s -w`. Toolchain directive is intentionally omitted from `go.mod` to keep `go install` friction low for OSS consumers.

## Testing conventions

- `github.com/stretchr/testify/require` for assertions.
- Table-driven tests for anything with multiple input/output variants.
- File ordering: test functions first, then helpers and fixtures (departure from common Go practice; check existing `_test.go` files for the pattern).
- Group related package-level vars into a single `var ( ... )` block.
- End-to-end binary tests use `testing.Short()` to skip in fast paths.

`assets/ai/CLAUDE-CODE-COVERAGE.md` documents the coverage discipline (what to test, how to read profiles, how to fix root causes rather than mask symptoms).

## Linting

`.golangci.yml` enables errcheck, govet, ineffassign, staticcheck, unused, gosec, gocritic, revive (with `exported`, `package-comments`, `var-naming`, `error-*` rules).

Two narrowly-scoped suppressions:

- **errcheck** excludes `fmt.Fprintf` / `fmt.Fprintln` / `fmt.Fprint` globally — write errors to stdout/stderr aren't actionable (if stdout is broken, you can't even report the failure).
- **gosec** is path-scoped off `_test\.go` — G204 (subprocess-from-variable) is a false positive in tests where inputs are test-controlled.

If a new lint complaint appears that fits one of these patterns, prefer extending the config exclusion over adding inline `//nolint:` directives.

## CI

`.github/workflows/ci.yml` runs two jobs in parallel:

- **`test`** — matrix on the Go floor (currently `1.25.3`) and the latest supported major (`1.26.x`). Steps: tidy-check, gofmt-check, vet, `go test -race -count=1 -coverprofile=coverage.out`, then `scripts/coverage-check.sh`. Coverage profile is uploaded as an artifact only on the floor version.
- **`lint`** — single Go version, `golangci/golangci-lint-action@v6`.

CI's matrix policy: track Go's two-newest-major support window. When a new Go major ships, the floor advances and the matrix updates in lockstep.

## Repo gotchas

- **`/cascade` and `/bin` in `.gitignore` are root-anchored.** The unanchored form (`cascade`) silently shadows `cmd/cascade/` — any new file under `cmd/cascade/` would be ignored without warning. Keep the leading slash.
- **`assets/ai/` is gitignored** but contains reference material (Go best-practice guides, coverage discipline, AI-engineering methodology) that's loaded into the symlink target outside the repo. Useful for context; not part of the OSS surface.
- **`docs/design/`** has numbered subdirectories representing lifecycle states: `01-draft/`, `05-active/`. Implementation-ready plans live in `docs/dev/`.
- **Adding a new public package?** Add a matching pair of entries to `PACKAGES` and `THRESHOLDS` in `scripts/coverage-check.sh` at the same array index — that's how the per-package coverage policy is committed to.
- **Scripts in `scripts/` must remain bash 3.2 compatible.** No `declare -A` (associative arrays — bash 4+), no `mapfile`, no `[[ ... ]]` GNU extensions where `[ ... ]` works. macOS's default `/bin/bash` is still 3.2.x, and we don't force contributors to install Homebrew bash. If a script grows beyond ~80 lines and the portability constraint becomes painful, rewrite as a Go script (`go run`-able) — that drops the bash question entirely.

## Commit and PR conventions

- Imperative mood, summary ≤72 chars, body explains *why* (the *what* should be in the diff).
- No required ticket prefix (this is OSS).
- Branches off `main` use prefixes (`feature/`, `fix/`, `m2/`, etc.).
- Linear history on `main` — rebase or use GitHub's "rebase and merge" / "squash and merge".

See `CONTRIBUTING.md` for the full version. `SECURITY.md` for vulnerability reporting. `CODE_OF_CONDUCT.md` (Contributor Covenant 3.0) for community norms.
