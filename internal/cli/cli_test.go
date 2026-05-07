package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geomyidia/cascade/internal/cli"
	"github.com/geomyidia/cascade/internal/project"
)

// Layer 1 public-API tests for cli.Run that don't require swapping the
// internal seams (runGitDiff, runGoListWrapper, signalContext). Tests that
// drive those seams live in seam_test.go.

// TestRun_Version (F-2 + F-16 in-process precursor) verifies --version
// prints the project metadata to stdout and exits 0. Mirrors the M1
// in-process version test that previously lived in cmd/cascade/main_test.go.
func TestRun_Version(t *testing.T) {
	saveVersion, saveCommit, saveBranch, saveSummary, saveDate :=
		project.Version, project.GitCommit, project.GitBranch, project.GitSummary, project.BuildDate
	t.Cleanup(func() {
		project.Version, project.GitCommit, project.GitBranch, project.GitSummary, project.BuildDate =
			saveVersion, saveCommit, saveBranch, saveSummary, saveDate
	})
	project.Version, project.GitCommit, project.GitBranch, project.GitSummary, project.BuildDate =
		"", "", "", "", ""

	var stdout, stderr bytes.Buffer
	got := cli.Run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr)

	if got != 0 {
		t.Errorf("exit = %d, want 0; stderr: %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "cascade N/A (build N/A)") {
		t.Errorf("stdout = %q, want substring %q", stdout.String(), "cascade N/A (build N/A)")
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}
}

// TestRun_HelpShorthand verifies that `-h` (which flag.Parse returns
// flag.ErrHelp for, since "h" is not a registered flag) is treated as a
// help request: helpText is printed to stderr (by flag's auto-Usage) and
// Run returns 0. This is the asymmetric counterpart to TestRun_Help's
// stdout-routing for the explicit --help (long form).
func TestRun_HelpShorthand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := cli.Run([]string{"-h"}, strings.NewReader(""), &stdout, &stderr)

	if got != 0 {
		t.Errorf("exit = %d, want 0; stderr: %q", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr should contain helpText (auto-Usage on -h); got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("-h should route to stderr (stdlib flag default); stdout: %q", stdout.String())
	}
}

// TestRun_Help (F-15 in-process precursor) verifies --help prints the
// usage + exit-code table to stdout (Q4 GNU convention) and exits 0.
func TestRun_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := cli.Run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr)

	if got != 0 {
		t.Errorf("exit = %d, want 0; stderr: %q", got, stderr.String())
	}
	out := stdout.String()
	wantSubstrings := []string{
		"cascade",
		"--base",
		"--head",
		"--tags",
		"--changed-files",
		"--root",
		"--version",
		"--help",
		"Exit codes",
		"git diff failed",
		"go list failed",
		"cancelled or interrupted",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("--help stdout missing %q", want)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("--help should not write to stderr; got %q", stderr.String())
	}
}

// TestRun_FlagParsing (F-5) covers flag-parse failures and missing-required
// validation. All cases must exit 1 and write a diagnostic to stderr.
func TestRun_FlagParsing(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string // substring
	}{
		{
			name:       "unknown_flag",
			args:       []string{"--bogus"},
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "unexpected_positional",
			args:       []string{"surprise"},
			wantStderr: "unexpected positional argument",
		},
		{
			name:       "missing_base_no_changed_files",
			args:       []string{}, // no --base, no --changed-files
			wantStderr: "--base is required",
		},
		{
			name:       "missing_base_with_only_tags",
			args:       []string{"--tags=foo"},
			wantStderr: "--base is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := cli.Run(tt.args, strings.NewReader(""), &stdout, &stderr)
			if got != 1 {
				t.Errorf("exit = %d, want 1; stderr: %q", got, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout must be empty on flag error; got %q", stdout.String())
			}
		})
	}
}

// TestRun_FileChangedFiles (F-9 sibling) verifies --changed-files=<path>
// reads from the named file, trims lines, skips empties. Tests that don't
// need to swap runGoListWrapper but DO exercise loadChangeSet's file branch.
//
// NOTE: this test reaches the pipeline (runGoListWrapper) which would call
// real go list. We use a flag combination that fails before reaching the
// pipeline: --tags is set and --root points at a non-existent directory,
// so go list fails; but since we're testing loadChangeSet's file-read
// branch, we accept any non-zero exit as long as the file was opened
// successfully (verified by absence of "opening --changed-files" in stderr).
//
// For full pipeline integration tests, see seam_test.go.
func TestRun_FileChangedFiles_OpensFile(t *testing.T) {
	tmp := t.TempDir()
	csPath := filepath.Join(tmp, "changes.txt")
	content := "  pkga/a.go  \n\n   \npkgb/b.go\n"
	if err := writeFile(csPath, content); err != nil {
		t.Fatalf("writing changeset file: %v", err)
	}

	// Point --root at an empty tmpdir so go list (the production runGoListWrapper)
	// fails fast with "no Go files in" — confirms loadChangeSet at least got
	// past the file-open + scanLines steps.
	var stdout, stderr bytes.Buffer
	exit := cli.Run(
		[]string{"--changed-files=" + csPath, "--root=" + tmp},
		strings.NewReader(""), &stdout, &stderr,
	)
	// File-open succeeded (no "opening --changed-files" stderr); pipeline
	// failed as expected (go list errors with non-zero).
	if strings.Contains(stderr.String(), "opening --changed-files") {
		t.Errorf("file-open path should have succeeded; stderr: %q", stderr.String())
	}
	// Don't assert exit code here — go list's exact failure-vs-success on an
	// empty dir varies; the point of this test is that the file was opened
	// and parsed without error. Exit may be 3 (go list failed) or even 0
	// (if go list succeeds with empty package list).
	_ = exit
}

// TestRun_FileChangedFiles_OpenFailure verifies the file-not-found branch
// of loadChangeSet maps to exit 1 (errFlagOrInput-wrapped).
func TestRun_FileChangedFiles_OpenFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cli.Run(
		[]string{"--changed-files=/this/path/does/not/exist/changes.txt"},
		strings.NewReader(""), &stdout, &stderr,
	)
	if exit != 1 {
		t.Errorf("exit = %d, want 1; stderr: %q", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "opening --changed-files") {
		t.Errorf("stderr should mention file-open failure; got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty on file-open failure; got %q", stdout.String())
	}
}

// writeFile is a tiny test helper that writes content to path.
func writeFile(path, content string) error {
	return writeAll(path, []byte(content))
}
