// Package project exposes build metadata that the Makefile injects into
// cascade binaries via -ldflags. Binaries built without ldflags (plain
// `go install` / `go run`) get a fallback population strategy that keeps
// the VERSION file as the canonical source for `Version` and uses
// `runtime/debug.ReadBuildInfo` for git metadata.
//
// Source-of-truth precedence (highest to lowest):
//
//  1. -ldflags injection (Makefile builds). Set at link time, before
//     init() runs — so the package vars are non-empty when init() executes,
//     and the fallback below is a no-op.
//  2. Embedded VERSION file (this package's `init()`, via go:embed).
//     Always populated; ldflags wins only because the var is already set.
//  3. runtime/debug.ReadBuildInfo (this package's `init()`). Fills git
//     metadata only — never Version.
//
// VERSION file is the *only* source for the `Version` field. ReadBuildInfo's
// `Main.Version` (a pseudo-version like v0.0.0-...-<sha> or a real semver
// tag) is never used for `Version` — bumping cascade's version is a one-file
// edit at project/VERSION. However, when `Main.Version` *is* a pseudo-version,
// its trailing 12-char commit prefix and 14-digit timestamp are extracted as
// fallbacks for `GitCommit` and `BuildDate` respectively. This closes the
// proxy-install gap: a `go install ...@<sha>` built from the Go module proxy
// (where vcs.* settings are absent) still reports the actual commit.
//
// Modeled on the zylog version-package pattern, adapted to cascade.
package project

import (
	_ "embed"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// versionFile is the VERSION file's contents, embedded at compile time.
// The file lives at project/VERSION; a convenience symlink at the repo
// root (./VERSION) points to it.
//
//go:embed VERSION
var versionFile string

// Build-time metadata. All vars are populated at link time via -ldflags
// (see Makefile's LDFLAGS_VERSION) for Makefile-built binaries. For
// plain `go install` / `go run` builds (no ldflags), init() falls back
// to the embedded VERSION file (Version) and runtime/debug.ReadBuildInfo
// (GitCommit, BuildDate). GitBranch and GitSummary stay empty under the
// fallback path — Go's build-info doesn't carry branch names, and a
// synthesised git-describe could diverge from the Makefile representation.
//
// AP-07 deviation (acknowledged): these are exported package-level
// **mutable** vars. The substrate's AP-07 ("Mutable Package-Level
// Globals") is SHOULD-AVOID. Rationale for keeping them: cascade uses
// them as **link-time injection targets** for `-ldflags -X`, populated
// once before init() runs and never mutated thereafter in production.
// They are the standard Go pattern for build-info embedding (also seen
// in `kubectl`, `hugo`, every cobra-based CLI). AP-07's hazards
// (order-dependent tests, multi-tenancy collisions) don't apply because
// these aren't config or registries — they're constants whose value is
// set at link time. Tests in this package and `internal/cli/cli_test.go`
// do mutate them (via the withMetadata helper in version_test.go); the
// "do not call t.Parallel() in any test that mutates these" discipline
// is documented in S-2 of docs/dev/0014-go-quality-audit.md and at the
// top of every seam-using *_test.go file.
var (
	// Version is the cascade module version, sourced from project/VERSION.
	// Always populated (either via ldflags or the go:embed fallback).
	Version string

	// GitCommit is the short SHA of the commit the binary was built from.
	// Under the fallback path, this comes from runtime/debug build settings'
	// vcs.revision, truncated to 7 characters and suffixed with "-dirty"
	// when vcs.modified == "true".
	GitCommit string

	// GitBranch is the branch the binary was built from. Populated by
	// ldflags only; Go's build-info doesn't carry branch names, so this
	// stays empty under the fallback path.
	GitBranch string

	// GitSummary is `git describe --tags --dirty --always` output —
	// gives a concise human-readable identifier (e.g. v0.1.0-3-gabc1234).
	// Populated by ldflags only; the fallback path leaves it empty rather
	// than synthesising a substitute that could diverge.
	GitSummary string

	// BuildDate is the RFC3339-formatted UTC build timestamp under the
	// ldflags path. Under the fallback path, it holds the commit time
	// (vcs.time from runtime/debug) — a slight semantic stretch, but the
	// only timestamp signal available without ldflags.
	BuildDate string
)

func init() {
	loadDefaults()
}

// loadDefaults applies the source-of-truth fallback chain: embedded
// VERSION for Version, ReadBuildInfo for git metadata. Each field is
// populated only if it's empty, so ldflags-injected values win.
//
// Factored out of init() for testability — tests can clear the package
// vars and re-invoke loadDefaults() to exercise the fallback paths.
func loadDefaults() {
	if Version == "" {
		Version = strings.TrimSpace(versionFile)
	}
	if GitCommit == "" || BuildDate == "" {
		readBuildInfoFallback()
	}
}

// readBuildInfo is a seam over runtime/debug.ReadBuildInfo so tests can
// drive both the ok=true and ok=false branches of the fallback. Production
// builds use debug.ReadBuildInfo directly.
var readBuildInfo = debug.ReadBuildInfo

// readBuildInfoFallback populates GitCommit and BuildDate from
// runtime/debug.ReadBuildInfo when ldflags didn't inject them. It NEVER
// touches Version (per the source-of-truth contract).
//
// `go build`/`go install` populate the VCS settings automatically when
// run from a git working tree. Builds invoked with -buildvcs=false
// suppress the settings, in which case GitCommit and BuildDate stay
// empty (and BuildString() returns "N/A").
func readBuildInfoFallback() {
	info, ok := readBuildInfo()
	if !ok {
		return
	}
	applyBuildInfo(info)
}

// applyBuildInfo is the testable seam for readBuildInfoFallback's
// scanning logic. Takes an injectable BuildInfo so tests can drive
// every branch with synthetic settings.
//
// Fallback order for git metadata (each field independently):
//
//  1. info.Settings vcs.* keys (present when built from a git working tree).
//  2. Pseudo-version suffix on info.Main.Version (present when built via
//     the Go module proxy from a commit SHA).
//
// Either path can populate GitCommit and BuildDate; ldflags-injected values
// always win because the field is only set when empty.
func applyBuildInfo(info *debug.BuildInfo) {
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

	// If either field is still empty and Main.Version looks like a
	// pseudo-version (proxy-install path), extract from there.
	if GitCommit != "" && BuildDate != "" {
		return
	}
	pseudoCommit, pseudoDate, ok := parsePseudoVersion(info.Main.Version)
	if !ok {
		return
	}
	if GitCommit == "" {
		GitCommit = pseudoCommit
	}
	if BuildDate == "" {
		BuildDate = pseudoDate
	}
}

// pseudoVersionSuffix matches the trailing "<14-digit-timestamp>-<12-hex-commit>"
// pattern in a Go module pseudo-version. Two forms are accepted:
//
//   - "v0.0.0-20260506200756-4fd94246d2e2"        (no prior tag — leading "-")
//   - "v0.1.1-0.20260506200756-4fd94246d2e2"      (after a tag — leading ".")
//
// Real semver tags ("v0.1.0", "v1.2.3-rc1") don't match.
var pseudoVersionSuffix = regexp.MustCompile(`[-.](\d{14})-([a-f0-9]{12})$`)

// parsePseudoVersion extracts the commit prefix and ISO-formatted commit
// time from a Go module pseudo-version string. Returns ok=false if the
// input isn't a pseudo-version (real semver tag, empty, or malformed
// timestamp). Commit is truncated to 7 chars to match the project's
// short-SHA convention.
func parsePseudoVersion(v string) (commit, date string, ok bool) {
	m := pseudoVersionSuffix.FindStringSubmatch(v)
	if m == nil {
		return "", "", false
	}
	t, err := time.Parse("20060102150405", m[1])
	if err != nil {
		return "", "", false
	}
	return m[2][:7], t.UTC().Format(time.RFC3339), true
}

// VersionString returns Version if set, else "N/A". With the embedded
// VERSION file fallback, "N/A" is reachable only when tests deliberately
// zero the package vars; production binaries always have Version populated.
func VersionString() string {
	if Version == "" {
		return "N/A"
	}
	return Version
}

// BuildString returns "<branch>@<commit>, <date>" if GitCommit is set,
// else "N/A". GitCommit is the canary because a binary built without
// any ldflags or VCS info has all build-metadata fields empty; if
// GitCommit is set, the others were either injected by ldflags or
// populated from ReadBuildInfo.
func BuildString() string {
	if GitCommit == "" {
		return "N/A"
	}
	return fmt.Sprintf("%s@%s, %s", GitBranch, GitCommit, BuildDate)
}

// PrintVersions writes a multi-line block of version info to w, suitable
// for verbose --version output. Single-line callers should compose
// VersionString and BuildString directly.
func PrintVersions(w io.Writer) {
	fmt.Fprintf(w, "cascade version: %s\n", VersionString())
	fmt.Fprintf(w, "Build: %s\n", BuildString())
	fmt.Fprintf(w, "Go version: %s\n", runtime.Version())
}
