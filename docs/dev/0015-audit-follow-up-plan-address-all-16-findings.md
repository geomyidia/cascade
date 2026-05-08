# Audit Follow-up Plan — Address All 16 Findings

**Source audit:** [`docs/dev/0014-go-quality-audit.md`](../docs/dev/0014-go-quality-audit.md). Audit + CDC verification both filed; this plan is the action layer.

**Goal:** address 100% of the audit's findings — every Open, every Acknowledged-with-rationale. For the rationale-accepted findings, "address" means either (a) strengthening the inline rationale, or (b) actually substrate-aligning the code if you'd rather close the deviation than carry it forward.

**Methodology:** every finding gets a row below with disposition (`fix-now` / `fix-as-CC-task` / `keep-as-rationale-with-doc-strengthen` / `decline`), estimated effort, and a brief justification. Duncan strikes/edits any row before execution; whatever survives is the work order.

## Finding-by-finding

| ID | Severity | Audit recommendation | Proposed disposition | Effort | Notes |
|----|----------|----------------------|----------------------|--------|-------|
| **F-1** | SHOULD | `AP-02` → `AP-13` cite at `pkg/golist/errors.go:16` | **fix-now** | 1 char | Trivial. Documentation pass. |
| **F-2** | SHOULD | `EH-08` → `CC-08` cite at `pkg/golist/golist.go:204` | **fix-now** | 1 word | Trivial. Doc pass. |
| **F-3** | SHOULD | `EH-08` → `CC-08` cite at `internal/cli/seam.go:62` | **fix-now** | 1 word | Trivial. Doc pass. |
| **F-4** | Acknowledged | ctx-as-last-param in `classifyRunError` + `classifyGitDiffError`. Audit closed as accept-with-rationale; substrate's CC-08 is MUST. | **keep-as-rationale-with-doc-strengthen** | 0 / refactor | Two paths: (a) accept the deviation as audit closed it (the rationale is sound; both classifiers are post-hoc, not propagation; cousin-shape consistent). (b) Substrate-align by moving ctx to first param. **My recommendation: (a).** Rationale is documented, cousin-consistent, and the post-hoc-classifier exception is genuinely outside CC-08's intended scope. Strengthen the inline comment to make the post-hoc-vs-propagation distinction explicit and add a CC-08 cite (so readers find the rule the rationale is deviating from). If you want full substrate alignment instead, this becomes a CC task: refactor both functions + every call site (~6 files). |
| **F-5** | CONSIDER | Add `Dir string` field to `*GitDiffError` so it matches `*ExitError`'s shape; populate from `cfg.root` at call site. | **fix-as-CC-task** | small refactor | Real cousin-shape symmetry fix per the audit. Small (errors.go ~5 LoC; cli.go call site ~1 LoC), but needs a test update in `internal/cli/seam_test.go` to assert the new field. CC writes a small impl plan, lands as its own PR. |
| **F-6** | CONSIDER | Add "Parallel-unsafe: shares package-level seam variables" comment near each `withSeam`-style helper. | **fix-now** | ~6 lines across 5 files | Documentation discipline. Done in the doc-pass PR. |
| **F-7** | POLISH | Test-helper duplication — `internal/cli/cli_test.go:21-26`'s save-restore dance duplicates `internal/project/version_test.go:withMetadata`. | **decline** | 0 | Audit already calls it "not load-bearing." Exporting `project.WithMetadataForTest` adds a public API surface for one consumer; not worth the surface. Keep duplication; add a 2-line comment cross-referencing `version_test.go`'s helper as "the canonical pattern." Effective decline-with-cross-reference. |
| **F-8** | POLISH | `helpers_test.go`'s `writeAll` is duplicated by `writeFile` in `cli_test.go`. | **fix-now** | small | Inline `writeAll` into `writeFile`, delete `helpers_test.go`. ~10 lines net deletion. Done in the doc-pass PR. |
| **F-9** | POLISH | Inline-map subtest name in `pkg/golist/golist_test.go:106`; refactor to struct-named subtests. | **fix-now** | ~5 lines | Cosmetic but cheap. Done in the doc-pass PR. |
| **F-10** | SHOULD | Doc drift in `pkg/changeset/changeset.go:65-68` — claims "moduleRoot stays empty" when `getCwd` errors; post-bug-#12 fix actually absolutizes via `filepath.Abs("")` to cwd. | **fix-now** | ~6 lines | Most substantive of the SHOULD findings. Rewrite the doc paragraph to describe the actual behavior. Done in the doc-pass PR. |
| **F-11** | POLISH | Same doc drift in the internal comment at `pkg/changeset/changeset.go:113-118`. | **fix-now** | ~5 lines | Pairs with F-10. |
| **F-12** | Acknowledged | `pkg/` layout vs PS-06 SHOULD-AVOID. Audit closed as accept-with-documented-rationale (cascade's package-layout refactor decision). | **keep-as-rationale** + **carry-forward to v1.0** | 0 | Audit's closure is right. Add a one-line note in CLAUDE.md's Architecture section: "PS-06 deviation accepted; revisit at v1.0 if a flatter layout becomes feasible." Done in the doc-pass PR. |
| **F-13** | POLISH | `_ = io.EOF` dead code at `pkg/golist/golist.go:241`. | **fix-now** | 4 lines | Delete the placeholder + comment. `io` is already used at line 91. Done in the doc-pass PR. |
| **F-14** | CONSIDER | Synthetic argv at `internal/cli/cli.go:269` can drift from real argv at `internal/cli/seam.go:40`. | **fix-as-CC-task** | small refactor | The structural fix (return argv from the seam) is a small but cross-file change. CC writes a small impl plan, lands alongside or after F-5. Alternative cheap fix is a comment; my recommendation is the structural fix since `*GitDiffError` is part of the public-facing diagnostic chain. |
| **F-15** | CONSIDER | `moduleRootSet bool` in `pkg/changeset.config` exists primarily to support the seam tests. | **decline** + **document** | small | Audit's closure stands: drop-and-restructure (delete `moduleRootSet`, delete `getCwd` seam, rewrite seam tests via `t.Chdir`) is a non-trivial refactor for a small readability win. Keep the flag. Add a comment at the field declaration explaining it exists to distinguish the `WithModuleRoot("")` case from "not supplied" for the os.Getwd-fallback path. Done in the doc-pass PR. |
| **F-16** | Acknowledged | Mutable globals in `internal/project`. Audit closed as accept-with-rationale (link-time constants, not runtime mutables). | **keep-as-rationale-with-doc-strengthen** | small | Audit's closure stands. Strengthen the doc at the var block (`internal/project/version.go:54`) to explicitly name the AP-07 deviation + rationale (link-time injection target via `-X` ldflags, not runtime config). Pair with F-6's "no `t.Parallel()`" discipline note. Done in the doc-pass PR. |

**Plus the meta-finding** I surfaced in CDC verification:

| ID | Source | Recommendation | Disposition | Effort |
|----|--------|----------------|-------------|--------|
| **PROMPT-1** | CDC pass on the audit | Audit prompt's `TD-09 / IM-04` pairing should read `IM-12 / TD-09` (receiver-consistency lives at IM-12; IM-04 is `-er` interface naming). | **fix-now** | 1-line fix in `workbench/cc-audit-prompt-go-quality.md` | Closes the third instance of the systemic ID-drift surfaced by S-1. Without this fix, the next audit run inherits the same drift. |

## Grouping into PRs

**PR-A — Documentation pass (single small PR; ~25 lines net change across 8 files).** Closes F-1, F-2, F-3, F-6, F-7 (decline-with-cross-reference), F-8, F-9, F-10, F-11, F-12 (CLAUDE.md note), F-13, F-15 (doc-only address), F-16 (doc-strengthen), and PROMPT-1.

This is the bulk of the audit's findings (13 of 16 + the meta-finding) collapsed into one cleanly-scoped doc-pass PR. No behavioral changes; no test changes; no public-API changes. CI matrix runs the existing tests against the modified comments + doc.go content + a couple of test-helper renames.

**PR-B — Cousin-shape symmetry (single small refactor PR; ~30 lines).** Closes F-5 (add `*GitDiffError.Dir` field) and F-14 (return argv from the git-diff seam, drop the synthetic argv at the call site). These two pair structurally — both are about the diagnostic chain matching the actual exec invocation.

**PR-C (optional) — F-4 substrate-alignment (medium refactor PR; ~6 files).** Only if you want to flip F-4 from accept-with-rationale to substrate-align. Moves ctx to the first parameter on `classifyRunError` + `classifyGitDiffError` + every call site. **My recommendation is to skip PR-C** — the rationale in PR-A's strengthened doc-comment captures the post-hoc-classifier exception cleanly, and substrate-aligning would force ctx into the first slot on functions that don't propagate it (against the spirit of CC-08, even if it satisfies the letter).

## Closure expectations

- **PR-A:** I can execute and produce a single commit + diff for review. ~13 cleanly-scoped edits, each anchored at the audit's evidence file:line. Estimated wall-clock: 20-30 minutes.
- **PR-B:** CC implementation plan + execution + your CDC review. Same workflow as M3/M4/M5 milestones. ~1 hour total including the round-trip.
- **PR-C:** decline by default; we strike the row from this plan and PR-A's doc-strengthen handles F-4.

After all PRs land, the audit's findings table gets updated with `Status: closed` per row. The audit doc itself becomes the closed record of the pass.

## Open questions for Duncan

1. **PR-A scope confirm** — happy to execute now, or want to route through CC for consistency with the established workflow? Either works; PR-A is small enough that doing it directly saves a roundtrip.

2. **PR-C decline confirm** — agree we skip the F-4 substrate-alignment refactor and let the strengthened doc-comment in PR-A handle it? My read is yes (see F-4 row above), but it's a real call.

3. **PR-B routing** — straight to CC with this plan as the impl-plan source, or do you want a fuller `docs/dev/0015-…md` impl plan written first? F-5 + F-14 are small enough that the rows above might be sufficient; happy to expand if not.

4. **Decline rows** (F-7, F-15, F-12 carry-forward) — agree these are correctly closed without code change? F-7 in particular could go either way (export the test helper vs accept duplication-with-cross-reference).

Once you've struck/approved the rows, this plan becomes the work order. Whoever executes (me for PR-A, CC for PR-B) walks each row and fills in the audit doc's per-row `Status` + `Evidence` after each fix.
