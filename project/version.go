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
// tag) is intentionally ignored; bumping cascade's version is a one-file
// edit at project/VERSION.
//
// Modeled on the zylog version-package pattern, adapted to cascade.
package project

import (
	_ "embed"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
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
