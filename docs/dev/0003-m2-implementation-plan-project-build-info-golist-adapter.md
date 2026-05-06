# Implementation Plan: project/ Build-Info Fallback + M2 (`golist` adapter)

**Source specs:**
- `docs/design/05-active/0003-cascade-m2-golist-adapter.md` (M2 design)
- Conversation: user-requested `runtime/debug.ReadBuildInfo()` fallback for the existing `project` package, with constraints

**Parent plan:** `docs/design/01-draft/0001-cascade-high-level-project-plan.md`
**Predecessor:** M1 (merged to main; PR #1 + PR #3)

## Context

Two adjacent pieces of work, sequenced as separate PRs:

1. **Build-info fallback (`project` package upgrade).** During the M1/post-M1 verification, bare `go install github.com/geomyidia/cascade/cmd/cascade@<sha>` produced `cascade N/A (build N/A)` — the package vars are empty without `-ldflags` injection. The user wants this gap closed *now*, with three explicit constraints:
   - **VERSION file is the ultimate source of truth for `Version`.** Never bypassed.
   - **Makefile `LDFLAGS` injection is the ultimate source for production builds.** Wins when present.
   - **Bypass `LDFLAGS` only locally** (when not doing a full Makefile build). Concretely: only the `go install` / `go run` path, where ldflags weren't passed.

2. **M2 — `golist` adapter.** Build the io shell that converts `go list -deps -json -tags=<union> <patterns...>` output into a typed `[]Package`. The only place in cascade that talks to the `go` toolchain. Per the design doc, M2's structural goal is to make gta's silent-failure mode impossible: explicit error returns, typed error chains, fixture-driven tests at 100% coverage on the algorithmic surface.

The two pieces are kept as separate PRs because they're orthogonal — the build-info work touches only `project/`, never `golist/` — and PR-1's small scope makes it easy to verify in isolation before M2 development starts on top of it.

## Decisions resolved

### Build-info fallback decisions

| # | Question | Decision |
|---|---|---|
| BI-1 | How does `//go:embed` reach the VERSION file from `project/`? | **Move VERSION to `project/VERSION` + create a `./VERSION` symlink at repo root pointing to `project/VERSION`.** Real file inside the package (so `//go:embed VERSION` works); symlink at root preserves the convention that "the version is at `./VERSION`" for tooling and casual readers. |
| BI-2 | Source-of-truth precedence for `Version` | **VERSION (always embedded) → `-ldflags` override (Makefile) → ReadBuildInfo (NEVER overrides Version).** ReadBuildInfo only fills git metadata. |
| BI-3 | What ReadBuildInfo populates | `GitCommit` (from `vcs.revision`, truncated to 7 chars, `+ "-dirty"` if `vcs.modified == "true"`), `BuildDate` (from `vcs.time` — best-effort; technically commit time, not build time, but it's the only signal available). `GitBranch` and `GitSummary` remain empty (Go embeds neither). |
| BI-4 | Fallback ordering inside `project.init()` | `Version` initialised from embedded VERSION if and only if `Version == ""` (so ldflags wins by virtue of being applied at link time, before init runs against vars that are now set). Same pattern for git metadata: ReadBuildInfo only writes a field if that field is empty after ldflags. |

### M2 decisions (resolved spec open questions)

| # | Spec § | Question | Decision |
|---|---|---|---|
| M2-1 | "Run parameter ordering" Q1 | `tags` before `patterns` or after? | **`tags` first** per spec recommendation. Tags scope; patterns name. |
| M2-2 | "WithEnv" Q2 | Drop or keep? | **Keep** (user's call, overriding spec lean). Useful for hermetic test environments. |
| M2-3 | "Empty tags" Q3 | `-tags=""` or omit flag? | **Omit flag entirely** when `tags == nil` or empty. Matches spec recommendation. |
| M2-4 | "patterns == nil" Q4 | Error or default to `./...`? | **Default to `[]string{"./..."}`.** Friendlier API; matches common case. |
| M2-5 | "Test fixture count" Q5 | 8 enough or trim? | **8 is the floor.** CC adds more if coverage gaps surface during implementation. |
| M2-6 | "Sample module" Q6 | Add build-tag exercise? | **Yes — add `pkgd/pkgd_linux.go` + `pkgd/pkgd_darwin.go`** with `//go:build` directives. Smoke test passes `tags` slice and asserts cross-platform file selection. |
| M2-7 | "Layer 3 manual sanity check" Q7 | Generic framing in closing report? | **Yes.** Closing-report evidence describes module size, package count, test invocation, output shape — no project name. |
| M2-8 | "Ledger location" Q8 | Embed in spec or separate file? | **Embed in spec for M2.** Revisit if it becomes unwieldy in M3+. |

## Stage 1 — Build-info fallback (separate PR)

Branch: `feature/buildinfo-fallback` off `main`. Single PR. Estimated ~80 lines added across 4 files.

### 1.1 Move VERSION

```bash
mv VERSION project/VERSION
ln -s project/VERSION VERSION
```

The symlink is committed (git tracks symlinks natively as mode 120000 on Linux/macOS). Document in `CONTRIBUTING.md` that the canonical version path is `project/VERSION` and `./VERSION` is a convenience symlink.

**Note on Windows**: git on Windows defaults to converting symlinks to text files unless `core.symlinks = true`. cascade isn't targeting Windows for development; CI runs on Linux. Add a one-line note in CONTRIBUTING.md for future-Windows-contributors.

### 1.2 `project/version.go` — embed + ReadBuildInfo fallback

Add to `project/version.go`:

```go
import (
    _ "embed"
    "runtime/debug"
    "strings"
)

//go:embed VERSION
var versionFile string

func init() {
    // Source-of-truth precedence:
    //   1. -ldflags (set before init runs; if present, vars are non-empty)
    //   2. Embedded VERSION file (this init block) — for go-install / go-run
    //   3. ReadBuildInfo (this init block) — for git metadata only
    //
    // The constraint enforced by this ordering: VERSION is *always* the
    // source for Version (whether via ldflags injection of $(cat VERSION)
    // or via the embed below). ReadBuildInfo is *never* allowed to set
    // Version — it only fills git metadata that ldflags would have provided.

    if Version == "" {
        Version = strings.TrimSpace(versionFile)
    }

    if GitCommit == "" || BuildDate == "" {
        readBuildInfoFallback()
    }
}

// readBuildInfoFallback populates GitCommit and BuildDate from
// runtime/debug.ReadBuildInfo when ldflags didn't inject them.
// It NEVER touches Version (per the source-of-truth contract).
// GitBranch and GitSummary stay empty here — Go's build-info doesn't
// embed branch names, and synthesising a git-describe equivalent
// would risk diverging from the Makefile-built representation.
func readBuildInfoFallback() {
    info, ok := debug.ReadBuildInfo()
    if !ok {
        return
    }
    var revision, modified, vcsTime string
    for _, s := range info.Settings {
        switch s.Key {
        case "vcs.revision":
            revision = s.Value
        case "vcs.modified":
            modified = s.Value
        case "vcs.time":
            vcsTime = s.Value
        }
    }
    if GitCommit == "" && revision != "" {
        commit := revision
        if len(commit) > 7 {
            commit = commit[:7]
        }
        if modified == "true" {
            commit += "-dirty"
        }
        GitCommit = commit
    }
    if BuildDate == "" && vcsTime != "" {
        BuildDate = vcsTime
    }
}
```

### 1.3 `project/version_test.go` — extend coverage

Refactor first, then add tests against the testable seams.

**Refactor target:**
- Rename the body of `init()` to `loadDefaults()` and call `loadDefaults()` from `init()`. Tests invoke `loadDefaults()` directly after `withMetadata` clears vars.
- Factor `readBuildInfoFallback` so the scanning logic takes an injectable `*debug.BuildInfo` argument; the package-level wrapper just calls `debug.ReadBuildInfo()` and delegates.

```go
func readBuildInfoFallback() {
    info, ok := debug.ReadBuildInfo()
    if !ok { return }
    applyBuildInfo(info)
}

func applyBuildInfo(info *debug.BuildInfo) {
    // ... the scanning + assignment logic
}
```

**`TestLoadDefaults` cases** (new test, table-driven):
- `embed-fills-empty-Version` — Version="", loadDefaults() → Version=="0.1.0" (or whatever the embedded VERSION file contains)
- `ldflags-Version-takes-precedence` — Version="9.9.9" (simulating ldflags), loadDefaults() → Version unchanged
- `empty-GitCommit-triggers-fallback` — GitCommit="", loadDefaults() → readBuildInfoFallback runs (verified by side-effect or by seam injection in a sibling test)
- `set-GitCommit-skips-fallback` — GitCommit="abcdef0", loadDefaults() → GitCommit unchanged

**`TestApplyBuildInfo` cases** (new test, table-driven over synthetic `*debug.BuildInfo`):
- `vcs.revision` long → truncated to 7 chars
- `vcs.modified == "true"` → `-dirty` suffix
- `vcs.time` populated → `BuildDate` set
- all settings empty → no-op
- `GitCommit` already set → ReadBuildInfo skipped (precedence preserved)

100% coverage on the `project` package is achievable via `loadDefaults` + `applyBuildInfo` cases plus the existing `VersionString`/`BuildString`/`PrintVersions` cases.

#### 1.3.1 Update `cmd/cascade/main_test.go` for the new defaults

**This is a Stage-1 prerequisite, not optional.** The existing `TestRun/"version flag"` case asserts `"cascade N/A (build N/A)"` in stdout — that holds today because `project.Version` is empty in the test process. Once the `//go:embed VERSION` lands in `project/version.go`, `init()` populates `project.Version` to the embedded `0.1.0` before any test runs, and the assertion breaks.

Two acceptable fixes:

1. **Reset-in-test (preserves the test's intent — "no metadata in scope yields N/A output"):**
   ```go
   // At the top of TestRun, before the table:
   saveVersion, saveCommit, saveBranch, saveDate :=
       project.Version, project.GitCommit, project.GitBranch, project.BuildDate
   t.Cleanup(func() {
       project.Version, project.GitCommit, project.GitBranch, project.BuildDate =
           saveVersion, saveCommit, saveBranch, saveDate
   })
   project.Version, project.GitCommit, project.GitBranch, project.BuildDate = "", "", "", ""
   ```
2. **Update assertion (acknowledges the new defaults):** change `wantStdout` for the `"version flag"` case to `"cascade 0.1.0"` (substring), accepting that GitCommit may be populated by ReadBuildInfo when tests run from a `.git`-rooted working tree.

Pick (1) — it's behaviour-preserving and the assertion remains a strong signal. (2) makes the test's intent muddier ("test that --version prints something containing the version string" is weaker than "test the no-metadata-in-scope path returns N/A").

Add `cmd/cascade/main_test.go` to the Stage-1 critical-files table.

### 1.4 Makefile + `scripts/coverage-check.sh`

- `Makefile`: change `VERSION := $(shell cat VERSION 2>/dev/null || echo "unknown")` to `VERSION := $(shell cat project/VERSION 2>/dev/null || echo "unknown")`. The repo-root `./VERSION` symlink would still work for `cat VERSION`, but reading the canonical path makes the dependency explicit.
- `scripts/coverage-check.sh`: no change — `project` is already at 100%, the new code is in the same package.

### 1.5 README / CONTRIBUTING text

- README.md "Status" section: add a one-liner that `go install` users get embedded version + auto-detected commit/dirty/timestamp from VCS info.
- CONTRIBUTING.md: note that the canonical version source is `project/VERSION`; bumping is a one-file edit at that path.

### 1.6 Stage 1 verification

```bash
make check-all                           # full local gauntlet
make build && ./bin/cascade --version    # cascade 0.1.0 (build <branch>@<sha>, <date>)

# Bare go install — must now show real Version + commit (no ldflags)
TMPDIR=$(mktemp -d); GOPATH=$TMPDIR GOBIN=$TMPDIR/bin GOMODCACHE=$TMPDIR/pkg/mod \
  go install github.com/geomyidia/cascade/cmd/cascade@<post-merge-sha>
$TMPDIR/bin/cascade --version
# Expected: cascade 0.1.0 (build @<sha-short>[-dirty], <commit-time>)
# (Branch is empty under the no-ldflags path; that's by design.)
```

Critical: **embedded VERSION must show `0.1.0`** in the no-ldflags case, not `N/A`. That's the closing signal that the source-of-truth contract holds.

## Stage 2 — M2 (`golist` adapter)

Branch: `m2/golist-adapter` off post-Stage-1 `main`. Larger PR; could be split into the parser (Layer 1) and the io shell (Layer 2) if the diff gets unwieldy, but a single PR is fine if it stays under ~600 lines.

### 2.1 Pre-flight reading (CC's responsibility)

Per design doc §"Required reading", before writing any code:

- [ ] `assets/ai/go/SKILL.md` — index + Critical Rules
- [ ] `assets/ai/go/09-anti-patterns.md` — AP-01…AP-15 walked
- [ ] `assets/ai/go/03-error-handling.md` — EH-01, EH-04, EH-07, EH-36 noted
- [ ] `assets/ai/go/06-concurrency.md` — CC-08…CC-12, CC-23, CC-42 noted
- [ ] `assets/ai/go/02-api-design.md` — API-41, API-42 noted
- [ ] `assets/ai/go/05-interfaces-methods.md` — IM-17 noted
- [ ] `assets/ai/go/07-testing.md` — TE-01…TE-15, TE-42, TE-43 noted
- [ ] `assets/ai/go/11-documentation.md` — DC-01, DC-02 noted
- [ ] `assets/ai/go/10-project-structure.md` — package boundary discipline skim

Closing report names the loaded guides and the IDs cited during implementation.

### 2.2 Public API surface

Per spec §"Public API surface", with M2-1…M2-4 applied:

- **`Package` struct** — 11 exported fields exactly as specced.
- **`Module` struct** — `Path string`, `Main bool`.
- **`Run` signature** —
  ```go
  func Run(ctx context.Context, tags []string, patterns []string, opts ...Option) ([]Package, error)
  ```
  Per M2-4: if `len(patterns) == 0`, `Run` substitutes `[]string{"./..."}` and continues (documented in godoc; tested).
  Per M2-3: if `len(tags) == 0`, the `-tags=` flag is **omitted** entirely from the argv (not passed as `-tags=""`).

- **Options:** `WithDir`, `WithEnv`, `WithGoBin` — keep all three per M2-2.
- **Error types:** `*ExitError`, `*ParseError` with `Is`/`Unwrap` methods. Sentinels: `ErrGoNotFound`, `ErrGoListFailed`, `ErrParseFailed`. `ParseErrorMaxPayload` const at 4096.

### 2.3 Implementation file layout

```
golist/
├── doc.go              (already present from M1)
├── golist.go           (new — public API: Run, Option, types)
├── parse.go            (new — streaming JSON decoder; in-process testable)
├── errors.go           (new — typed error definitions + Is/Unwrap impls)
├── golist_test.go      (new — public-API tests in `package golist_test` per TE-43)
├── parse_test.go       (new — fixture-driven decoder tests)
├── errors_test.go      (new — Is/Unwrap chain assertions)
└── testdata/
    ├── single-package.json
    ├── multi-package.json
    ├── with-tests.json
    ├── build-tag.json
    ├── stdlib-mixed.json
    ├── empty.json
    ├── truncated.json
    ├── malformed.json
    └── sample-module/
        ├── go.mod
        ├── pkga/a.go
        ├── pkgb/b.go
        ├── pkgc/
        │   ├── c.go
        │   └── c_test.go
        └── pkgd/
            ├── pkgd_linux.go     (//go:build linux)
            └── pkgd_darwin.go    (//go:build darwin)
```

### 2.4 Coverage discipline

Per spec F-13: per-package gate at 100% on `golist/`. The single os/exec line (`cmd.Run()` in the io shell) is the documented exception — covered by Layer 2's subprocess test, not the per-package gate.

Strategy: factor the io shell so the testable seam is the JSON decoder + a small adapter. The function calling `exec.CommandContext(...).Output()` is ~5 lines; everything else (argv construction, decoding, error wrapping, option application) is in-process testable.

**To make the os/exec line not count against the 100% gate:** put it in a thin wrapper that's in a separate `// +build !cover` file? No — that's brittle. Better approach: the thin wrapper has 1 statement and 0 branches; covering it via Layer 2 is sufficient and the gate's "100% statements" target either hits it via the `TestRun_SampleModule` test or accepts a 99.x% reading. The spec acknowledges this in F-13's notes column.

Recommended concrete shape: a private `runGoList(cmd *exec.Cmd) (io.ReadCloser, *ExitError, error)` whose body is `output, err := cmd.Output(); ...`. Layer 2's subprocess test covers it. The decoder, argv builder, option application, and error wrappers are all reached by Layer 1 fixtures, getting them to 100%.

### 2.5 Cancellation & concurrency

Per spec §"Risks & mitigations":
- Use `exec.CommandContext(ctx, ...)`. ctx cancellation kills the subprocess.
- After `Run`'s decoder loop, drain stdout/stderr pipes (don't leak file descriptors).
- Goroutine running the decoder must terminate cleanly when ctx is done — guard the loop with `select { case <-ctx.Done(): return ctx.Err() default: }` or rely on the read returning an error when the pipe closes.
- **`TestRun_ContextCancellation` (spec F-14):** exercises mid-stream cancellation via `t.Context()` + a deliberate cancel; runs under `-race` to catch goroutine leaks.
- **`TestRun_Concurrent` (spec F-15):** spawns N concurrent `Run` calls against the sample module from independent goroutines; asserts each returns the same `[]Package` and no race-detector violations fire. Verify command in the spec ledger: `go test -race -run 'TestRun_Concurrent' ./golist`.

### 2.6 M2 acceptance ledger (carry from spec)

The 18 ledger rows F-1…F-18 are the spec's; CC fills Status + Evidence at the commit each lands. CDC verifies. No deferrals expected.

### 2.7 Stage 2 verification

Per spec exit criteria + the decisions above:

- [ ] `make check-all` passes locally (Makefile gate at 90%; per-package gate at 100% on golist via fixture coverage).
- [ ] CI green on the matrix (1.25.3 + 1.26.x + lint).
- [ ] Per-package coverage gate fires correctly when violated (Layer-1 fixture run through `bash scripts/coverage-check.sh`).
- [ ] `go doc github.com/geomyidia/cascade/golist` renders cleanly (spec F-17).
- [ ] **F-15 explicitly:** `go test -race -run 'TestRun_Concurrent' ./golist` passes.
- [ ] **F-16 explicitly:** `[ "$(go list -m all | wc -l | tr -d ' ')" = "1" ]` — confirms `golist` (and the cascade module overall) has no non-stdlib imports.
- [ ] All other ledger rows F-1…F-14, F-17 verify-commands green.
- [ ] F-18 manual sanity check on a real Go module documented in closing report (generic framing).
- [ ] Closing report lists guides loaded + pattern IDs cited.

## Critical files to be created/modified

### Stage 1 (build-info fallback)

| Path | Action | Notes |
|---|---|---|
| `VERSION` | Move to `project/VERSION` | repo-root path becomes a symlink |
| `./VERSION` (symlink) | Create | `ln -s project/VERSION VERSION` |
| `project/version.go` | Modify | Add `_ "embed"`, `//go:embed VERSION`, `versionFile var`, refactored `init()` → `loadDefaults()`, `readBuildInfoFallback()`, `applyBuildInfo(*debug.BuildInfo)` |
| `project/version_test.go` | Extend | Tests for `loadDefaults` and `applyBuildInfo` precedence/fallback paths |
| `cmd/cascade/main_test.go` | Modify | `TestRun/"version flag"` resets `project.Version`/`GitCommit`/`GitBranch`/`BuildDate` to `""` with `t.Cleanup` restore, preserving the "no-metadata yields N/A" assertion under the new embed-default behaviour |
| `Makefile` | Modify | `VERSION := $(shell cat project/VERSION ...)` (canonical path; `cat VERSION` still works via symlink) |
| `CONTRIBUTING.md` | Modify | One-paragraph note: canonical version source is `project/VERSION`; root symlink is for convention |
| `README.md` | Modify (optional) | One-line addition under "Status" describing the embedded-version + VCS-fallback behavior for `go install` |

### Stage 2 (M2 golist adapter)

| Path | Action | Reuses |
|---|---|---|
| `golist/doc.go` | Already exists from M1 | — |
| `golist/golist.go` | Create | exec, json/Decoder; consumes `*exec.Cmd` from io shell |
| `golist/parse.go` | Create | json/Decoder over io.Reader (testable without subprocess) |
| `golist/errors.go` | Create | errors.New + Is/Unwrap method receivers |
| `golist/golist_test.go` | Create | `package golist_test` (external; per TE-43) |
| `golist/parse_test.go` | Create | Fixture-driven |
| `golist/errors_test.go` | Create | `errors.Is`/`errors.As` chain checks |
| `golist/testdata/*.json` | Create (8 files) | Verbatim shapes from spec §"Layer 1" |
| `golist/testdata/sample-module/` | Create | `go.mod`, `pkga/a.go`, `pkgb/b.go`, `pkgc/c.go` + `c_test.go`, `pkgd/pkgd_{linux,darwin}.go` |

## Verification (consolidated)

### Stage 1 exit criteria

- [ ] `project/VERSION` exists; `./VERSION` is a symlink to it; `cat VERSION` from repo root yields `0.1.0`
- [ ] `make build && ./bin/cascade --version` shows `cascade 0.1.0 (build <branch>@<sha>, <date>)` with all four fields populated
- [ ] Bare `go install ...@<sha>` (clean GOPATH, no ldflags) shows `cascade 0.1.0 (build @<sha>[-dirty], <vcs.time>)` — Version from embed, GitCommit from ReadBuildInfo, BuildDate from vcs.time, GitBranch empty
- [ ] `project` package coverage at 100% (the per-package gate's existing requirement; verified by `bash scripts/coverage-check.sh`)
- [ ] CI green on the matrix; lint clean
- [ ] CONTRIBUTING.md updated with the canonical-path note

### Stage 2 exit criteria (per spec ledger F-1…F-18)

Verify-commands for F-1 through F-17 all green; F-18 manual sanity check documented in closing report. Closing report names guides loaded + pattern IDs.

## Risks & mitigations

### Stage 1

- **Symlink + go:embed interaction**: `//go:embed VERSION` inside `project/` resolves to the *real file* `project/VERSION`, not the root-level symlink. The embed directive enforces "no symlinks" — but it's reading the real file directly, so this is fine. The root-level symlink is only used by humans / shell tooling.
- **Windows symlink behavior**: git on Windows treats symlinks as text files unless `core.symlinks = true`. Mitigation: document in CONTRIBUTING.md; CI is Linux so unaffected. If a Windows contributor reports issues, the trivial fallback is to delete the symlink and read `project/VERSION` directly.
- **`vcs.modified` semantics**: `runtime/debug.ReadBuildInfo` only returns VCS info when built from a clean Go module (i.e. `go build`/`go install` from a directory with a `.git` parent). Buildflags like `-buildvcs=false` suppress it. If a build uses `-buildvcs=false`, the fallback yields nothing; `GitCommit` stays empty. Document the dependency on default `-buildvcs=auto` in the godoc.
- **`vcs.time` is commit time, not build time**: Naming `BuildDate` to hold a commit time is a slight semantic stretch, but it's the only signal available without ldflags. Acknowledged in the godoc on `BuildDate`.

### Stage 2 (carry from spec, all addressed in design)

- `go list -deps -json` schema drift across Go versions
- Streaming decoder partial reads (covered by `truncated.json`)
- Cancellation race (covered by `TestRun_ContextCancellation`)
- `go list` slowness on large modules (out of scope; M5 may add knobs)
- Module-proxy / network dependency (sample module is hermetic — no external deps)
- Fixture drift across Go releases (existing fixtures still parse; renames fail loudly)

## Out of scope (explicit)

### Stage 1

- `Main.Version` → `Version` substitution. ReadBuildInfo's `Main.Version` provides the module version (e.g., `v0.0.0-<pseudo>` or a real semver tag). The user's constraint is unambiguous: **VERSION file is the only source for `Version`**. The pseudo-version that `go install ...@<sha>` reports via `Main.Version` is *more accurate* than the embedded `0.1.0` for that particular build, but the user has chosen single-source-of-truth over per-build accuracy. ReadBuildInfo's `Main.Version` is intentionally ignored.
- `GitBranch` populated from any source other than ldflags. Go's build-info doesn't carry branch names; the field stays empty under the no-ldflags path.
- `GitSummary` populated from any source other than ldflags. Synthesising a `git describe`-equivalent from `Main.Version` would risk mismatch with the Makefile's representation.

### Stage 2 (carry from spec)

- Building the import graph (M3)
- File-to-package mapping (M4)
- CLI integration (M5)
- Filtering stdlib / external deps (M3+ may filter using fields M2 exposes)
- Build-tag inference (caller passes union explicitly)
- On-disk caching of `go list` output
- Concurrent invocation of multiple `go list` calls within one `Run`
