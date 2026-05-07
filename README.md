# cascade

[![][build-badge]][build]
[![][tag-badge]][tag]
[![][license badge]][license]

[![][logo]][logo-large]

*A tool for performing the reverse-transitive closure of a Go change-set under the imports relation.*

> *Reverse-BFS from your diff, over `go list -deps`'s import DAG.*

cascade computes the affected-package set for a Go CI test-selection workflow: given a base ref and a head ref, it prints the set of packages that need re-testing — the changed packages plus everything that imports them, transitively. The intended use is dropping the full-suite test bill on a typical PR by 3-10× while keeping merge-queue runs honest.

## Why

cascade exists because [DigitalOcean's `gta`](https://github.com/digitalocean/gta) — an established Go affected-package tool — silently fails on Go 1.25.x. The failure surfaces in `golang.org/x/tools/go/packages.Load`, which gta uses; that loader's stricter module resolution emits "go: updates to go.mod needed" against modules the regular `go list` family considers tidy, and gta swallows the error and exits 0 with an empty package list. An empty list means CI runs zero tests; zero tests means a green build that proved nothing.

Go's helper libraries — especially `golang.org/x/tools/go/packages` — evolve faster than the `go` command itself; depending on the CLI tool is the more stable bet. cascade takes a deliberately narrower path than gta: shell out to `go list -deps -json` directly (verified on Go 1.25 and 1.26), parse the stream into typed values, build the import DAG, reverse the edges, and compute the closure of the change-set. Every I/O error is returned and surfaced — silent-failure mode is structurally impossible.

## Install

```bash
go install github.com/geomyidia/cascade/cmd/cascade@latest
```

Requires Go ≥ 1.25.3. CI tests against the floor and the latest currently-supported Go major; the matrix advances with each Go release.

## CLI usage

```bash
cascade --help
cascade --version

# In a Go repo, against a PR's range:
cascade \
  --tags=integration_test,parallel_safe \
  --base=origin/main \
  --head=HEAD
```

Output is one import path per line, sorted lexicographically — ready to feed into `go test`:

```bash
go test $(cascade --base=origin/main --head=HEAD)
```

For callers who already have a list of changed files (e.g., a CI workflow that's already run `paths-filter`), pass them on stdin:

```bash
git diff --name-only origin/main..HEAD | cascade --changed-files=-
```

### Flag reference

<!-- keep in sync with internal/cli/cli.go's helpText constant -->

| Flag | Default | Purpose |
|------|---------|---------|
| `--tags` | (none) | Comma-separated build tags passed to `go list -tags=`. |
| `--base` | (required unless `--changed-files`) | Base git ref (e.g. `origin/main`); cascade runs `git diff --name-only <base>..<head>` to derive the change-set. |
| `--head` | `HEAD` | Head git ref. |
| `--changed-files` | (none) | Path to a file with one change-set entry per line. `-` reads from stdin. When set, `--base` is not required and `git diff` is not invoked. |
| `--root` | `.` | Working directory for `go list` and module-root for `changeset.Resolve`. Absolute or relative; cascade absolutizes via `filepath.Abs` before use, so the default `.` resolves to the process cwd. |
| `--version` | false | Print `cascade <Version> (build <Branch>@<Commit>, <BuildDate>)` and exit. (Branch is empty for `go install` builds — the module proxy doesn't carry branch metadata.) |
| `--help` | false | Print usage and exit. Routes to stdout per GNU convention; flag-parse errors route help to stderr per stdlib `flag` default. |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success. Output may be empty if no Go files changed. |
| 1 | Flag-parse error, missing required flags, or stdin/file read failure. |
| 2 | `git diff` failed. The `*GitDiffError` carries the captured `git` stderr; cascade prints it on its own stderr. |
| 3 | `go list` failed. The wrapped `*golist.ExitError` carries the captured `go list` stderr. |
| 4 | Internal logic error. Should never occur — surface as a real bug. |
| 5 | Cancelled or interrupted (SIGINT, SIGTERM, or context cancellation). |

`cascade --help` prints the same flag reference and exit code table to stdout. CI workflows should branch on the specific exit code (e.g. retry the run on exit 5; fail-fast on exit 4) rather than treating all non-zero codes uniformly.

## Library

cascade's pure-core packages are exported public APIs. If you want to build your own affected-package tooling — or just want to read `go list -deps -json` output without fighting `golang.org/x/tools/go/packages.Load` — the primitives are available:

```go
import (
    "context"

    "github.com/geomyidia/cascade/pkg/changeset"
    "github.com/geomyidia/cascade/pkg/depgraph"
    "github.com/geomyidia/cascade/pkg/golist"
)

ctx := context.Background()

// Run `go list -deps -json -tags=...` and parse the streaming output.
pkgs, err := golist.Run(ctx, []string{"integration_test"}, []string{"./..."})
if err != nil { /* handle, never swallow */ }

// Build a forward + reverse import graph.
g := depgraph.Build(pkgs)

// Map changed file paths onto their containing packages' import paths.
// repoRoot is typically `git rev-parse --show-toplevel` from the caller;
// when omitted, changeset.Resolve falls back to os.Getwd.
seeds := changeset.Resolve(changedFiles, pkgs, changeset.WithModuleRoot(repoRoot))

// Compute the reverse-transitive closure (the "cascade").
affected := g.RevDepClosure(seeds)
```

The pure packages (`pkg/golist`, `pkg/depgraph`, `pkg/changeset`) compose without adapter glue and are 100% test-covered. Errors from the I/O edges (`golist.Run`'s `go list` invocation; `changeset.Resolve`'s optional `os.Getwd` fallback) wrap their causes with `%w`, so callers can use `errors.Is`/`errors.As` to triage.

## How it works

1. **Resolve the change-set** — `git diff --name-only <base>..<head>`, or read paths from stdin for callers who already have a list.
2. **Run `go list -deps -json -tags=<union> ./...`** and stream-parse the result into typed `golist.Package` values. (This is the only place cascade talks to `go`.)
3. **Build the import DAG**, then reverse its edges.
4. **Map the changed file paths to the packages that contain them.** These become the seed set.
5. **BFS from the seeds over the reversed graph**; emit the union (seeds included), sorted lexicographically for determinism.

Every step is small, typed, and tested. The only I/O is the two `os/exec` calls in steps 1 and 2 — both are isolated in the I/O shell, so the algorithmic core has no error-swallowing surface area.

## Development

The project ships a `Makefile` that's the canonical menu of common commands:

```bash
make help          # show all available targets
make check-tools   # verify your local toolchain (Go, gofmt, goimports, golangci-lint, …)
make check         # build + lint + test
make check-all     # also: coverage gate + godoc
make coverage-html # open a browseable HTML coverage report
```

CI runs the same gates on every PR and on every push to `main`, against a Go-version matrix that tracks Go's two-newest-major support window.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for code conventions, the test discipline, and the engineering practices the project uses (peer-frame collaboration, milestone-scoped ledger discipline, evidence-backed verification). The reference materials in [`assets/ai/`](assets/ai/) — covering Go best practices, coverage discipline, subagent delegation policy, and the broader collaborative methodology — are the substrate the project's design and review work is built on.

## License

[Apache-2.0](LICENSE).

## Acknowledgments

cascade owes its problem framing to [DigitalOcean's `gta`](https://github.com/digitalocean/gta), the prior art that worked beautifully on Go 1.24 and earlier. The algorithm shape — `go list -deps -json` plus reverse-graph traversal — is documented in various community gists and shell sketches; cascade refines it into a small, hand-rolled Go binary with first-class error handling and a public package API.

The project name nods both to the operation (a change *cascading* through its reverse-dependents) and to the [Cascade Range](https://en.wikipedia.org/wiki/Cascade_Range) of the Pacific Northwest. See the project image for the gophers' opinion on the matter.

---

[//]: ---Named-Links---

[logo]: assets/images/logo-v1.png
[logo-large]: assets/images/logo-v1-large.png
[build]: https://github.com/geomyidia/cascade/actions/workflows/ci.yml
[build-badge]: https://github.com/geomyidia/cascade/actions/workflows/ci.yml/badge.svg
[tag-badge]: https://img.shields.io/github/tag/geomyidia/cascade.svg
[tag]: https://github.com/geomyidia/cascade/tags
[license]: LICENSE
[license badge]: https://img.shields.io/badge/License-Apache%202.0-blue.svg
