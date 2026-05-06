# M1 Implementation Retrospective: Repo Scaffold + CI Baseline

**Status:** closed.
**Closing commit:** `3bef597` (head of `main` at close), plus follow-ups `7f97a77` and `231a59a` for the golangci-lint pin and a separate commit for the versioning-package work after the F-12 re-run.
**CDC verification:** Claude (Opus 4.7, 2026-05-06 session).
**Source spec:** [`docs/design/06-final/0002-m1-repo-scaffold-ci-baseline.md`](../design/06-final/0002-m1-repo-scaffold-ci-baseline.md).
**Source impl plan:** [`docs/dev/0001-m1-implementation-plan-repo-scaffold-ci-baseline.md`](./0001-m1-implementation-plan-repo-scaffold-ci-baseline.md).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md).

## Closure summary

The impl plan's "Verification (M1 exit criteria)" section listed twelve acceptance criteria. All twelve reach a final status. Six items deferred at spec-amendment time carry forward as named deferrals, each with a re-entry condition.

| Status | Count |
|--------|-------|
| Done | 12 |
| Deferred | 6 |
| No-op | 0 |
| Open at close | 0 |

The exit signal from the spec — *"a clean push to `main` runs CI green on the Go-version matrix; the coverage gate fires correctly when violated; `go install github.com/geomyidia/cascade/cmd/cascade@<HEAD>` produces a runnable binary that prints version metadata"* — is satisfied.

## Ledger

| ID | Criterion | Verify (reproducible) | Status | Evidence |
|----|-----------|----------------------|--------|----------|
| F-1 | `go build ./...` on Go 1.25.3 and Go 1.26.x | CI matrix run | done | PR #1 green on commit `c53ce20`; `main` green on `3bef597`. |
| F-2 | `go test ./...` passes | CI matrix run | done | PR #1. |
| F-3 | `go vet ./...` clean | CI matrix run | done | PR #1. |
| F-4 | `golangci-lint run` clean | CI lint job | done | PR #1, after the action+version pin work in `7f97a77` and `231a59a`. **Spec deviation:** action `@v6` → `@v8`, version `latest` → `v2.11.4`. Rationale documented inline in `.github/workflows/ci.yml` (v8 is required for the linter v2 config schema; v2.11.4 is the earliest release built with go≥1.25, satisfying the module floor). Net: tighter than the spec asked for, with the *why* captured at the load-bearing artifact. |
| F-5 | `gofmt -l .` empty | CI gofmt step | done | PR #1. |
| F-6 | `go mod tidy` no-op | CI tidy gate | done | PR #1. The CI step is robust to the no-`go.sum` case (`if [ -f go.sum ]; then …; fi`) — relevant in M1 since cascade declares no external deps. |
| F-7 | `make check-all` passes locally | `make check-all` | done | CC's local environment, pre-PR. |
| F-8 | CI green on a fresh PR | GitHub PR status | done | PR #1. |
| F-9 | Coverage gate fires when violated | deliberate violation against the gate | done | PR #2 (closed) produced `::error::github.com/geomyidia/cascade/golist coverage 0% < required 100%` at the per-package step in **both** matrix jobs. Strong evidence — observed fire, not asserted. |
| F-10 | Branch protection rejects direct `main` pushes | GitHub branch-protection settings | done-with-bypass | Rule active. Admin (sole maintainer) has `bypass_mode: always`, so the rejection isn't observable from the maintainer's account — that is the intended sole-maintainer operational reality. |
| F-11 | Repo-health files present | `for f in CONTRIBUTING.md CODE_OF_CONDUCT.md SECURITY.md .github/PULL_REQUEST_TEMPLATE.md .github/ISSUE_TEMPLATE/bug_report.md .github/ISSUE_TEMPLATE/feature_request.md; do test -f "$f" \|\| exit 1; done` | done | CDC verified by direct read. `CODE_OF_CONDUCT.md` is genuinely Contributor Covenant **3.0** (verified by the v3-distinct pledge wording, not just the version label). |
| F-12 | `go install github.com/geomyidia/cascade/cmd/cascade@<sha>` succeeds and `cascade --version` prints injected metadata | clean-GOPATH install + `--version` exec | done | Originally softpedalled with "Makefile-driven local build verified; proxy will index when available" — that's not the criterion. CDC flagged. CC re-ran the actual `go install` invocation against the proxy and threaded in the maintainer's project-versioning methodology in the process. Closed cleanly. |

## Deferrals

Six items called out as deferred at spec-amendment time. Each has a reason and a re-entry condition; per `LEDGER_DISCIPLINE.md`, "later" is not an acceptable closure for a deferral.

| ID | Item | Reason | Re-entry condition |
|----|------|--------|--------------------|
| D-1 | "Require pull request before merging" branch-protection rule | Single-maintainer makes "≥1 approving review" awkward without admin-bypass overhead. Maintainer-authorised deferral, not CC drift. | Re-enable when a second contributor lands. Tracked as a v0.2 prerequisite. |
| D-2 | "Require conversation resolution before merging" branch-protection rule | Same single-maintainer rationale as D-1. | Same as D-1. |
| D-3 | `CODEOWNERS` file | Single-maintainer; no review-routing question to answer yet. | When a second contributor lands; pairs with D-1. |
| D-4 | Required signed commits | Contributor signing setup needs documenting in CONTRIBUTING.md before becoming a hard gate; raising friction without docs is hostile. | Defer to v0.2 (per spec). |
| D-5 | Release tooling beyond the Makefile (GoReleaser, release-please) | Manual `make release VERSION=v0.1.0` is sufficient for M6. Out-of-scope per spec. | Revisit if release cadence or contributor count warrants automation. |
| D-6 | `CHANGELOG.md` | No releases yet; first entry lands at M6. | M6 release prep. |

## What Worked

Patterns and decisions that made M1 close cleanly. Per `LEDGER_DISCIPLINE.md` CDC protocol step 8, this is the Safety-II complement to the defect ledger — the record that prevents the discipline from becoming purely reactive.

**`golangci-lint` pin discovery, with the *why* captured inline.** The spec said `version: latest` and `@v6`. CC discovered during execution that v6 of the action doesn't support the v2 linter config schema, and that v2.x linter releases below v2.11.4 are built with go1.24 and refuse to lint a module declaring `go 1.25+`. Both constraints are now load-bearing inline comments in `.github/workflows/ci.yml`, so the next person who touches the pin doesn't have to relearn them from first principles. This is the substrate pattern in miniature: capture the discovered constraint at the artifact, not in tribal knowledge.

**Coverage-driven test addition.** `cmd/cascade/main_test.go` includes a fifth case beyond the four specified — `"unexpected positional argument"` — which exercises the `fs.NArg() > 0` branch in `run()`. Without it that branch would have been at 0% statement coverage. Adding-test-cases-from-coverage-gaps is the discipline that keeps the per-package gate from drifting into noise.

**Defensive sanity check in `scripts/coverage-check.sh`.** The `if [[ "${#PACKAGES[@]}" -ne "${#THRESHOLDS[@]}" ]]` guard catches a maintenance footgun the spec didn't call out: adding a package threshold but forgetting the matching path entry, or vice versa. The script will refuse to run with mismatched arrays — fail loud, fail early.

**`VERSION` file vs. `./version/` directory dance.** macOS APFS's default case-insensitive collation collides `./version/` (a directory CC initially attempted) with `VERSION` (the maintainer-convention file). The resolution preserved the existing extension-free `VERSION` filename — the convention that other release tooling keys off — without forcing a case-sensitive volume on contributors. Worth remembering for any future package-naming decision; the trap is invisible until someone clones to a case-sensitive volume and gets contradictory behaviour.

**ODM lifecycle moved cleanly.** The M1 design doc transitioned 01-draft → 05-active → 06-final via `odm` as the work progressed and closed. Document state and milestone state stayed coherent throughout — no drift between "what the spec says" and "what the project state is."

**Independent verification surfaced a softpedalled `done`.** F-12 was initially closed with evidence that didn't match the criterion (Makefile build verified, not `go install`). CDC's per-row walk caught it and returned the row for re-execution. CC re-ran the actual `go install` and used the opportunity to thread in the maintainer's project-versioning methodology, which makes M5's CLI work easier when it lands. The protocol worked exactly as `LEDGER_DISCIPLINE.md` describes — the recorder-separate-from-closer split caught what self-assessment missed.

## CDC review notes

Three observations from this milestone's CDC pass that affect how future milestones should be reviewed.

**Misattribution to watch for.** The CDC pass initially flagged D-1 and D-2 as unauthorized deferrals on CC's part — "spec-softening". The maintainer corrected: the deferrals were maintainer-authorised at amendment time, not silent CC drift. The lesson for CDC is to ask "who decided this?" before treating an undocumented deferral as drift. For future milestones, deferrals authorised at spec-amendment should land in the closing ledger with explicit attribution (`maintainer-authorised` vs `CC-discovered`) so the audit trail doesn't require a back-and-forth to disambiguate.

**Sandbox limitation acknowledged.** CDC could not re-run the go-toolchain criteria (F-1 through F-7) directly — the verification environment had no `go` binary. Those rows are CC-attested via CI green, with structural verification (read the workflow, read the source, confirm the tests would behave as claimed) standing in for re-execution. This is the verification/assertion calibration the methodology asks for; the floor is "CC's CI claim plus CDC structural read" rather than "CDC re-ran end-to-end". Future milestones with reachable Go environments should close that gap with re-execution.

**The protocol caught a softpedalled row.** F-12's initial closure was evidence-of-a-different-criterion, not evidence-of-the-criterion. Per `LEDGER_DISCIPLINE.md`, this is the most-easily-corrupted discipline ("an audit that only reads the claim of completion is theater"). Catching it required reading the criterion text against the evidence text and noticing the mismatch. The countermeasure stays the same: per-row walk, evidence text compared against criterion text, no skip on rows that look obviously closed.

## Carry-forward into later milestones

Items that emerged during M1 and should affect M2+:

- **M2's design doc embeds a formal ledger from the start** (per [`docs/design/01-draft/0003-m2-golist-adapter.md`](../design/01-draft/0003-m2-golist-adapter.md)'s ledger section, currently in `workbench` for `odm` cycling). M1's retrospective is the *first* artifact to formalise the ledger structure; M2 is where the ledger lives with the design from day one. This is the methodology becoming load-bearing rather than retroactive.

- **Golangci-lint pin will need bumping again** when the Go floor advances (each new Go major requires a linter built with that toolchain or newer). Comment in `ci.yml` flags this. Add to M-series milestone-prep checklist if/when Go 1.27 lands.

- **macOS case-insensitive filesystem caveat** is worth a one-liner in `CONTRIBUTING.md` — "If you create a directory whose name shadows an existing file under case-insensitive collation, the conflict won't surface until someone clones to a case-sensitive volume." Pairs with the bash-3.2 portability note that's already there.

- **Branch-protection re-entry conditions (D-1, D-2, D-3)** all hinge on a second contributor joining. Worth tagging "second-contributor-readiness" as a meta-milestone so these don't get lost when that contingency arrives.

- **CDC bandwidth.** M1 was small (12 rows, mostly checkbox-shaped). M2 has 18 rows with grep-verifiable Verify commands and Layer-3 manual evidence. Reviewing M2 will be substantively heavier; budget accordingly.

## Closure

Closed at commit `3bef597` (and successor commits for the F-12 versioning-methodology landing). All twelve criteria reach a final status. Six deferrals carry forward with named re-entry conditions. CDC verification: this document.

Total rows: 12. Done: 12. Deferred-at-spec: 6. No-op: 0.
