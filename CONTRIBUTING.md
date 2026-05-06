# Contributing to Cascade

Cascade is a Go CLI + library that computes the reverse-transitive closure of a Go change-set under the imports relation, used for affected-package test selection in CI. See the [project plan](./docs/design/01-draft/0001-cascade-high-level-project-plan.md) for the full motivation and architectural framing.

Contributions are welcome. This document describes the conventions you'll need to follow for a PR to land.

## Development setup

```bash
# Confirm your toolchain
make check-tools

# Build, lint, test
make check
```

`make check-tools` will tell you which tools are installed and which need installing (`gofmt`, `goimports`, `golangci-lint`, `pkgsite`). All are optional except Go itself; missing tools degrade specific Make targets gracefully.

The Go floor for the module is declared in `go.mod`. Local development can use any later Go release; CI tests against the floor and the latest currently-supported major version.

## Branch and PR conventions

- Branch off `main`. Use a short, descriptive prefix (`feature/`, `fix/`, `m2/`, etc.).
- Open a PR against `main`. CI must pass and at least one approving review is required before merging.
- Linear history is enforced — rebase or use the GitHub "rebase and merge" / "squash and merge" path. No merge commits on `main`.
- Conversation resolution is required before merging.

## Commit messages

- Imperative mood ("Add coverage gate", not "Added coverage gate" or "Adds coverage gate").
- Summary line ≤72 chars.
- Optional body, separated from the summary by a blank line, explaining *why* (the *what* should be evident from the diff).
- No required prefix (this is OSS — no internal ticket-tag conventions).

## Code conventions

The general baseline is Go community best practices. The repo also keeps a curated set of Go best-practice notes at `assets/ai/go/` (a symlink, gitignored — these are reference material, not part of the OSS surface). Cascade-specific overrides on top of that baseline:

- **Testing:** `github.com/stretchr/testify/require` for assertions. Table-driven tests for anything with multiple input/output variants.
- **Test file ordering:** test functions first, then helpers and fixtures.
- **Variable declarations:** group related vars in a single `var ( ... )` block at file scope.
- **Coverage:** the pure packages (`golist/`, `depgraph/`, `changeset/`) must hit **100%** statement coverage, enforced per-package by `scripts/coverage-check.sh` in CI. The Makefile's `coverage-check` target enforces a softer 90% overall floor as a quick local sanity check. The CLI shell (`cmd/cascade/`) is not coverage-gated — its behavior is verified by an end-to-end test.

For coverage discipline specifically — what to test, how to read profiles, how to fix root causes rather than masking symptoms — see [`assets/ai/CLAUDE-CODE-COVERAGE.md`](./assets/ai/CLAUDE-CODE-COVERAGE.md).

## Adding a new public package

If a future change adds a new public package (sibling to `golist/`, `depgraph/`, `changeset/`) with implementation, also add a matching pair of entries to `PACKAGES` and `THRESHOLDS` in `scripts/coverage-check.sh` at the same array index. This is the structural way to commit to a coverage policy explicitly per package.

## Go-version policy

CI's matrix tracks Go's currently-supported major versions (the two newest releases). Expect the floor in `go.mod` to advance with each new Go release.

## Reporting issues

- **Bugs:** use the bug-report template.
- **Feature requests:** use the feature-request template.
- **Security vulnerabilities:** see [`SECURITY.md`](./SECURITY.md) — do not file public issues.
- **Code of Conduct concerns:** see [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md).
