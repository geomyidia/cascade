// Package project exposes build metadata that the Makefile injects into
// cascade binaries via -ldflags. The package vars are empty by default,
// so a binary built without ldflags (e.g. a plain `go install`) reports
// "N/A" through the helpers rather than misleading hard-coded defaults.
//
// Modeled on the zylog version-package pattern, adapted to cascade.
// Lives at the top level (not under util/) and named `project` so call
// sites read naturally as project.VersionString() rather than the
// package-stutter version.VersionString().
package project

import (
	"fmt"
	"io"
	"runtime"
)

// Build-time metadata. All vars are populated at link time via -ldflags
// (see Makefile's LDFLAGS_VERSION). They remain empty when the binary is
// built without ldflags; the helpers below treat empty as "N/A."
var (
	// Version is the cascade module version, sourced from the VERSION
	// file at the repo root by the Makefile.
	Version string

	// GitCommit is the short SHA of the commit the binary was built from.
	GitCommit string

	// GitBranch is the branch the binary was built from.
	GitBranch string

	// GitSummary is `git describe --tags --dirty --always` output —
	// gives a concise human-readable identifier (e.g. v0.1.0-3-gabc1234).
	GitSummary string

	// BuildDate is the RFC3339-formatted UTC build timestamp.
	BuildDate string
)

// VersionString returns Version if set, else "N/A".
func VersionString() string {
	if Version == "" {
		return "N/A"
	}
	return Version
}

// BuildString returns "<branch>@<commit>, <date>" if GitCommit is set,
// else "N/A". GitCommit is the canary because a binary built without
// any ldflags has all fields empty; if GitCommit is set, the others
// were injected too.
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
