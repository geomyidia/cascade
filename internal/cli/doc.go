// Package cli is the testable entry point for the cascade binary. It owns
// flag parsing, the four-line pipeline assembly (golist.Run → depgraph.Build
// + changeset.Resolve → g.RevDepClosure), the git-diff io edge, error-to-
// exit-code mapping, and signal-driven cancellation.
//
// The package is internal/ so external consumers cannot import it; cmd/cascade
// is the only intended caller. Run is exposed (not main) so the orchestration
// is testable in-process without subprocess overhead, mirroring the M1
// pattern. The git-diff io edge is hooked through a function-variable seam
// (runGitDiff), mirroring pkg/golist's runGoList from M2.
//
// Exit-code contract:
//
//	0 — success (output may be empty if no Go files changed)
//	1 — flag-parse error or missing required flags or stdin read failure
//	2 — `git diff` failed (*GitDiffError returned somewhere in the pipeline)
//	3 — `go list` failed (*golist.ExitError or wrapper)
//	4 — internal logic error (should never occur — surface as a real bug)
//	5 — cancelled / interrupted (context cancellation reached the io layer)
//
// The exit-code table is mirrored in three places: this doc, cascade --help's
// output, and the README's CLI-usage section. Keep in sync.
package cli
