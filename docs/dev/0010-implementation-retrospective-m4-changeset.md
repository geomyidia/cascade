# M4 Implementation Retrospective: `pkg/changeset` (Changed-Files-to-Packages Mapping)

**Status:** closing report; awaiting CDC verification.
**Closing commit:** `75617a2` (head of `m4/changeset-mapping` at close; M4 PR head). Merge OID lands here when rebase-merge to `main` completes.
**Adjacent commit:** `ed64e14` (the implementation plan + design-doc-state transitions, committed to main by the user before the M4 branch was cut).
**CDC verification:** pending.
**Source spec:** [`docs/design/05-active/0005-cascade-m4-changeset-changed-files-to-packages-mapping.md`](../design/05-active/0005-cascade-m4-changeset-changed-files-to-packages-mapping.md). Anticipated to transition 05-active → 06-final post-merge.
**Source impl plan:** [`docs/dev/0009-implementation-plan-changed-files-to-packages-mapping.md`](./0009-implementation-plan-changed-files-to-packages-mapping.md).
**Methodology:** [`assets/ai/LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md), [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md).

## Closure summary

The spec's acceptance ledger declared **fourteen** rows (F-1..F-14) with a stated **deferral budget of zero**. All fourteen reach a final status of `done`. The exit signal — *"`changeset.Resolve(changedFiles, pkgs, opts...)` returns the import paths of packages whose Go files appear in `changedFiles`, sorted lexicographically; per-package coverage gate at 100% holds; a change-set with a file outside any package returns gracefully (empty or skipped, not an error)"* — is satisfied across all three clauses.

| Status | Count |
|--------|-------|
| Done | 14 |
| Deferred | 0 |
| No-op | 0 |
| Open at close | 0 |

The package's exit signal — *"the trio of pure packages (`pkg/golist`, `pkg/depgraph`, `pkg/changeset`) compose without adapter code"* — is observed structurally: `Resolve`'s return type (`[]string` of import paths) is exactly the input shape `depgraph.Graph.RevDepClosure` accepts as seeds. M5 wires the CLI on top with no pending design questions.

## Disclosed amendments to the spec

Three deviations from the spec text, all caught at plan-time and recorded for the spec's next ODM promotion:

1. **F-2 signature drift (positional → functional option).** The spec specified `func Resolve(changedFiles []string, pkgs []golist.Package, moduleRoot string) []string`. User overrode to functional option during plan-mode review (decision Q1): `func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string` with `WithModuleRoot(string) Option`. Same disclosed-amendment pattern as M3's F-20 (Stats type) addition.
2. **`os.Getwd` fallback (Q2 decision).** The spec leaned "skip relative paths when moduleRoot is empty" (b). User chose: fall back to `os.Getwd()` when `WithModuleRoot` is not supplied; tests pass `WithModuleRoot` explicitly to bypass the io. This introduces a single io edge into the package, hooked through an internal function-variable seam (`getCwd`) following M2's `runGoList` pattern.
3. **`doc.go` updated.** The M1 stub said "All operations are pure — no io." That is no longer accurate after Q2. Updated inline to "Operations are deterministic and table-test-friendly. The single io edge — a default os.Getwd fallback when WithModuleRoot is not supplied — is hooked through an internal function-variable seam so tests can be made fully pure by passing WithModuleRoot explicitly." F-1's verify still passes (the `// Package changeset` comment line is preserved).

These amendments are all forward-looking surface decisions; no behavioural drift from the spec's contractual rules (mapping rules, dedup, sort, no-error contract are all preserved).

## Substrate loaded at session start

Per [`assets/ai/AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md) §"Substrate" and the spec's §"Required reading":

- [`AI-CONSTITUTION-SUPPLEMENT.md`](../../assets/ai/AI-CONSTITUTION-SUPPLEMENT.md), [`AI-ENGINEERING-METHODOLOGY.md`](../../assets/ai/AI-ENGINEERING-METHODOLOGY.md), [`LEDGER_DISCIPLINE.md`](../../assets/ai/LEDGER_DISCIPLINE.md) — already loaded from the M3/refactor session continuing into this one.
- [`assets/ai/go/SKILL.md`](../../assets/ai/go/SKILL.md) — Critical Rules section and Document Selection Guide refreshed.
- [`assets/ai/go/guides/09-anti-patterns.md`](../../assets/ai/go/guides/09-anti-patterns.md) — walked. M4-relevant pattern IDs cited: **AP-29** (no interface returns — Resolve returns `[]string`, Option is a function-type alias not an interface), **AP-30** (no producer-side interfaces — none exposed), **AP-36** (early-return over deep nesting — `if !ok { return }` style throughout), **AP-40** (slice prealloc — `make([]string, 0, len(seen))` for the result, `make(map[string]string, len(pkgs))` for the dirMap), **AP-44** (no log+return — Resolve has no error return at all), **AP-52** (filepath confinement — informational; M4 doesn't open files but the prefix-check pattern is documented for any future file-content variant).
- [`assets/ai/go/guides/02-api-design.md`](../../assets/ai/go/guides/02-api-design.md) — **API-42** honoured (no globals; everything threaded through `Resolve`'s args + opts; the `getCwd` seam variable is unexported and tests-only).
- [`assets/ai/go/guides/04-type-design.md`](../../assets/ai/go/guides/04-type-design.md) — already loaded; **TD-09** (uniform receivers; not applicable here as `Resolve` is a free function and `Option` carries no receiver), **TD-17** (nil slices treated as empty; `Resolve` returns nil from the no-match path, callers may `range` over the result either way).
- [`assets/ai/go/guides/05-interfaces-methods.md`](../../assets/ai/go/guides/05-interfaces-methods.md) — already loaded; **IM-01** (return concrete; `Resolve` returns `[]string`; `Option` is a concrete function-type alias).
- [`assets/ai/go/guides/07-testing.md`](../../assets/ai/go/guides/07-testing.md) — already loaded; **TE-01..TE-08** (table-driven idiom — every test in M4 is a table-driven loop; `name` field; `t.Run` per case; field-named struct literals; no `t.Fatal` from secondary goroutines), **TE-15** (no assertion library — plain stdlib `testing`, matches existing cascade convention), **TE-43** (`package changeset_test` for public-API tests; `package changeset` only inside `seam_test.go` to access the unexported `getCwd` variable).
- [`assets/ai/go/guides/11-documentation.md`](../../assets/ai/go/guides/11-documentation.md) — already loaded; **DC-01** (every exported name documented: `Resolve`, `Option`, `WithModuleRoot`); **DC-02** (every doc comment starts with the identifier name).

## Ledger

| ID | Criterion | Verify (reproducible) | Status | Evidence |
|----|-----------|----------------------|--------|----------|
| F-1 | `pkg/changeset/doc.go` exists with package comment | `test -f pkg/changeset/doc.go && head -3 pkg/changeset/doc.go \| grep -q '^// Package changeset'` | done | M1 stub preserved at the structural level (the `// Package changeset` comment line is intact); content body updated per disclosed amendment #3 above to reflect the `os.Getwd` io edge introduced by Q2. Verify command still exits 0. |
| F-2 | `Resolve` signature matches spec | `go doc github.com/geomyidia/cascade/pkg/changeset.Resolve \| grep -F '...'` | done | Output: `func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string`. Diverges from the spec's positional `moduleRoot string` form — the disclosed amendment #1 above documents the user override (Q1) to functional option. The active spec's ledger F-2 verify text needs updating at next ODM promotion to match. |
| F-3 | All `TestResolve_StandardCases` rows pass | `go test -run 'TestResolve_StandardCases' ./pkg/changeset` | done | `ok github.com/geomyidia/cascade/pkg/changeset 0.263s`. 16 named subtests pass: `empty_changedFiles`, `nil_pkgs`, `single_Go_file_in_pkga`, `two_Go_files_in_pkga_deduped`, `files_in_pkga_and_pkgb_sorted`, `_test.go_in_pkga`, `xtest_dot_go_in_pkga`, `mixed_go_and_non_go`, `Go_file_outside_any_package`, `Go_file_in_subdirectory_of_pkg_dir`, `removed_Go_file_in_pkga`, `relative_path_resolved_against_moduleRoot`, `absolute_path_used_directly`, `path_with_dot_dot_components_cleaned`, `duplicate_entries_in_changedFiles`, `unsorted_entries_yield_sorted_output`. Each subtest also asserts `sort.StringsAreSorted(got)` inline — the determinism criterion enforced per-subtest, not just under -count. |
| F-4 | Hand-traceable 4-package case passes | `go test -run 'TestResolve_HandTraceable' ./pkg/changeset` | done | `ok github.com/geomyidia/cascade/pkg/changeset 0.159s`. Seven sub-cases assert the exact mappings from the spec (pkga→ex/pkga, _test.go→ex/pkga, xtest→ex/pkgb, multi-pkg→sorted, subdir→nil, non-go→nil, empty→nil). |
| F-5 | Path-normalisation cases pass | `go test -run 'TestResolve_PathNormalisation' ./pkg/changeset` | done | `ok github.com/geomyidia/cascade/pkg/changeset 0.149s`. Five OS-portable cases pass: redundant separators, `./` prefix, interior `./`, `..` traversal, absolute-with-extra-separators. `filepath.Clean` normalises all of them. |
| F-6 | `_test.go` files map to the package, not to `_test` ImportPath | `go test -run 'TestResolve_StandardCases/_test\.go_in_pkga' ./pkg/changeset` | done | `--- PASS: TestResolve_StandardCases/_test.go_in_pkga (0.00s)`. Confirms `_test.go` files yield `ex/pkga`, not `ex/pkga_test`. |
| F-7 | Non-Go files silently skipped | `go test -run 'TestResolve_StandardCases/mixed' ./pkg/changeset` | done | `--- PASS: TestResolve_StandardCases/mixed_go_and_non_go (0.00s)`. The case includes `pkga/a.go`, `README.md`, `pkgb/data.json`; only the Go file maps. |
| F-8 | Files outside any package skipped, no error | `go test -run 'TestResolve_StandardCases/Go_file_outside' ./pkg/changeset` | done | `--- PASS: TestResolve_StandardCases/Go_file_outside_any_package (0.00s)`. `cmd/cascade/main.go` (parent dir not in dirMap) yields `nil`. |
| F-9 | Removed files mapped via parent-dir match | `go test -run 'TestResolve_StandardCases/removed_Go_file' ./pkg/changeset` | done | `--- PASS: TestResolve_StandardCases/removed_Go_file_in_pkga (0.00s)`. `pkga/deleted.go` (no `os.Stat`, no existence check) yields `[ex/pkga]` because the parent-dir lookup is purely lexical. |
| F-10 | Output sorted lexicographically and deduplicated | `go test -count=10 -run 'TestResolve' ./pkg/changeset` | done | 10 runs, all pass; `ok github.com/geomyidia/cascade/pkg/changeset 0.154s`. Each subtest also asserts `sort.StringsAreSorted(got)` inline so non-determinism would fail in the failing subtest, not just as a flake under -count=10. |
| F-11 | Per-package coverage gate at 100% on `pkg/changeset` | `bash scripts/coverage-check.sh` | done | `ok: github.com/geomyidia/cascade/pkg/changeset coverage 100% >= 100%`. The `getCwd` seam closes the os.Getwd success/error branch coverage gap; the `TestResolve_PackagesWithEmptyFieldsSkipped` and `TestResolve_EmptyFilePathSkipped` tests close the defensive-skip branches in the dirMap-build and changedFiles loops. All four cascade pure/internal packages now at 100%. |
| F-12 | `pkg/changeset` has no non-stdlib imports beyond `pkg/golist` | `[ "$(go list -m all \| wc -l \| tr -d ' ')" = "1" ]` | done | Output: `1` (the cascade module itself; zero external deps). Imports: `os`, `path/filepath`, `sort`, `strings` from stdlib, plus `github.com/geomyidia/cascade/pkg/golist` from own module. |
| F-13 | `go doc github.com/geomyidia/cascade/pkg/changeset` renders cleanly | `go doc github.com/geomyidia/cascade/pkg/changeset \| head -20` | done | Output (capped at 20 lines per Q6 decision): package overview (3 paragraphs covering the mapping role, the io-edge note, and the API-stability note), then `func Resolve(...)`, `type Option func(*config)`, `func WithModuleRoot(dir string) Option`. Every exported name documented per DC-01; every doc comment starts with the identifier per DC-02. |
| F-14 | Closing report names guides loaded + pattern IDs cited | reviewer reads closing report | done | This document, §"Substrate loaded at session start" enumerates every guide loaded with the specific pattern IDs cited (AP-29/30/36/40/44/52, API-42, TD-09/17, IM-01, TE-01..08/15/43, DC-01/02). |

## Deferrals

**Zero deferrals.** The spec's stated deferral budget was zero, and the milestone closes with that property held.

## What Worked

Patterns that made M4 close cleanly. Per `LEDGER_DISCIPLINE.md` CDC protocol step 8.

**The function-variable seam pattern generalised cleanly from M2 to M4.** M2 introduced `runGoList = defaultRunGoList` for the `os/exec` boundary; M4 reuses the exact shape for `getCwd = os.Getwd`. The `withCwdSeam` helper in `pkg/changeset/seam_test.go` is structurally identical to `pkg/golist/seam_test.go`'s `withSeam` — just substitute the type. **This confirms the pattern's reusability:** when a single io edge needs in-process testing without subprocess overhead, the function-variable seam is the textbook structural answer. Future packages with similar "mostly pure plus one io call" shape should reach for the same idiom by default.

**Plan-mode resolution of open questions surfaced design-affecting answers BEFORE implementation.** The spec author had left six open questions. Three (Q1 API shape, Q2 empty-moduleRoot behaviour, Q5 IgnoredGoFiles) were materially impactful; the other three were minor. Asking the user via `AskUserQuestion` with three bundled multiple-choice questions surfaced two override decisions (Q1 and Q2) that would have been wrong if I'd applied the spec's leans blindly. The user's Q1+Q2 combined answer ("functional option, with `os.Getwd` as default but use option pattern so tests can opt out") was clearly worked-out — the kind of design decision that benefits from the explicit ask, not from CC second-guessing. Pattern: **always ask about open questions in the spec, even when leans are present.** The plan-mode workflow's Phase 3 (`AskUserQuestion`) is the right place for this; the cost is one round-trip and the benefit is a plan that matches user intent.

**The test-side `_` package convention (TE-43) plus the internal `seam_test.go` carve-out works.** The public-API tests live in `package changeset_test` (external) so they exercise the contract; the seam-driven tests live in `package changeset` (internal, in `seam_test.go`) so they can replace the unexported `getCwd` variable. Both files coexist in the same directory; `go test` runs them together. **The boundary is clean and well-precedented**: M2's `pkg/golist/seam_test.go` follows the same shape. Carry-forward: any future package with a function-variable seam should follow this two-file layout (external `*_test` for contract, internal `seam_test.go` for seam-driven coverage).

**100% coverage on first try, with no test contortions.** The `pkg/changeset` package hit 100% statement coverage on the first `go test -coverprofile` run. The seam tests close the os.Getwd success/error branches; two small dedicated tests close the defensive-skip branches (`TestResolve_PackagesWithEmptyFieldsSkipped`, `TestResolve_EmptyFilePathSkipped`); the StandardCases table covers everything else. **No "increase coverage by adding speculative branches" anti-pattern was triggered**; every branch in the production code is reachable from natural test inputs. This is what the M3 retro called out as the right shape for pure-data packages with the 100% gate: the gate is honest, not gamed.

**OUT-1 (M3's lint-cache fix) caught the gofmt issue immediately.** First `make check-all` after writing the tests failed at the gofmt step on `pkg/changeset/changeset_test.go` (struct-literal trailing-comment alignment). `make format` fixed it. **OUT-1 again paid for itself**: the lint surfaced the issue locally rather than after a CI round-trip. Pattern is now battle-tested across three milestones (M3, refactor, M4).

**Spec-confirmed: `golist.Package.Dir` is absolute.** The M2 inventory (per the explore agent's report) confirmed `pkg/golist` documents `Dir` as absolute, and live `go list -json` output on the sample-module fixture produces `Dir: "/Users/.../sample-module/pkga"` (fully-qualified). M4's algorithm relies on this — comparing `filepath.Dir(absPath)` against `pkg.Dir` works only if both are absolute. **The dependency on M2's contract is explicit, documented, and verified at the substrate level**, not assumed.

## Pre-PR self-check (CC)

Per the M2/M3/refactor retro carry-forward: read each row's criterion text against the planned evidence, flag any "pending" / "deferred" / "next round" framings as drift candidates *before* PR open.

Walked F-1..F-14:
- All fourteen rows have `done` status with reproducible Verify command output captured in the Evidence column above.
- No rows framed as "pending," "deferred," or "next round."
- F-1 has a noted spec-vs-evidence shape difference (the `doc.go` body changed to reflect the io edge), explicitly documented as disclosed amendment #3 above. The Verify command's letter (the `// Package changeset` comment line) is satisfied; the spirit (the M1 stub's content) is honestly amended, not silently drifted.
- F-2 has a noted signature drift (positional → functional option), explicitly documented as disclosed amendment #1. The active spec's ledger F-2 will need updating at next ODM promotion; CC will mention this in the PR description so CDC notices.
- All other rows: criterion text matches evidence text exactly.

No softpedals identified at self-check.

## CDC review notes

_Pending. To be filled in by CDC after independent verification per `LEDGER_DISCIPLINE.md` CDC protocol._

## Carry-forward into M5

- **Three pure packages compose without adapter code.** The trio (`pkg/golist` + `pkg/depgraph` + `pkg/changeset`) is now functionally complete in-library. M5's CLI wires:
  ```go
  pkgs, err := golist.Run(ctx, tags, patterns)             // M2
  g := depgraph.Build(pkgs)                                 // M3
  seeds := changeset.Resolve(changedFiles, pkgs, opts...)   // M4
  affected := g.RevDepClosure(seeds)                        // M3
  ```
  Each call's output type matches the next call's input shape; no adapter functions are needed. M5 should explicitly verify this at the integration-test layer.

- **`os.Getwd` fallback semantics in M5 CLI.** When M5 invokes `Resolve`, it should pass `WithModuleRoot(rootDir)` explicitly with the value of `git rev-parse --show-toplevel` — not rely on the `os.Getwd` fallback, since the CLI may be invoked from any cwd. The fallback exists for library consumers who *want* the "use the current directory" convenience; M5 specifically does not.

- **The seam pattern is now established convention.** Two milestones (M2, M4) have used the function-variable seam for an io edge. The pattern is mature; M5+ packages with io edges should reach for it by default. No further documentation work needed — the existing two-package precedent is sufficient.

- **Spec amendments need to flow back into 05-active.** Three disclosed amendments in M4 (F-2 signature, Q2 io edge, doc.go content). The active M4 spec at `docs/design/05-active/0005-...md` should be updated post-merge to match — the CC retrospective is the carry-forward record, but the spec is the source-of-truth. ODM promotion (05-active → 06-final) should happen after that update.

- **CC self-check protocol track record: 4-for-4.** M3, refactor, and now M4 have all applied the pre-PR row audit; zero softpedals caught at audit time across the three. The discipline is settled. M5 will be the first milestone where the audit exists pre-implementation (CLI scope is larger; the chance of a softpedal is non-trivial).

- **Test-fixture-vs-real-path convention.** M4 tests use POSIX-style absolute paths (`/m/pkga`) explicitly. CI is Linux-only. If/when Windows-portability becomes relevant, M4's tests would need updating. Documented inline in `pkg/changeset/helpers_test.go`.

## Closure

Closing report submitted with the M4 PR; CDC verification pending. All fourteen criteria reach a final status of `done`. Zero deferrals. Zero no-ops. Zero open at close.

Total rows: 14. Done: 14. Deferred: 0. No-op: 0.
