package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Layer 2 — end-to-end binary tests. M1's in-process unit tests of run()
// retired in M5: the orchestration moved to internal/cli (Run is testable
// in-process there with full coverage). These binary tests verify the
// build → exec → wire chain end-to-end against the real binary.

// TestCascadeBinaryVersion (F-16) is the M1 carry-over: builds the cascade
// binary with -ldflags injecting known version metadata, then exec's it
// with --version to verify the injection chain. Skipped under -short.
func TestCascadeBinaryVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end binary test in short mode")
	}

	binPath := buildTestBinary(t)

	cmd := exec.Command(binPath, "--version") //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s --version failed: %v\nstderr: %s", binPath, err, stderr.String())
	}

	out := stdout.String()
	wantSubstrings := []string{
		"cascade " + injectedVersion,
		injectedBranch + "@" + injectedCommit,
		injectedDate,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("--version output missing %q\nfull output: %q", want, out)
		}
	}
}

// TestCascadeBinaryHelp (F-15) verifies the binary's --help output contains
// the full flag reference and exit-code table on stdout.
func TestCascadeBinaryHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end binary test in short mode")
	}

	binPath := buildTestBinary(t)

	cmd := exec.Command(binPath, "--help") //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s --help failed: %v\nstderr: %s", binPath, err, stderr.String())
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
		t.Errorf("--help should not write to stderr; got: %q", stderr.String())
	}
}

// TestCascadeBinaryEndToEnd (F-17) drives the full pipeline against the
// pkg/golist sample-module fixture. Uses --changed-files=- with synthetic
// stdin (Q6: cmd.Stdin = strings.NewReader) so git diff is bypassed and the
// test exercises golist.Run + depgraph.Build + changeset.Resolve +
// g.RevDepClosure end-to-end against real Go code. Skipped under -short.
//
// The sample module has pkga/pkgb/pkgc/pkgd; pkgb imports pkga, pkgc imports
// pkga via test files, pkgd has build-tag-conditional files. Editing
// pkga/a.go should affect pkga and its importers.
func TestCascadeBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end binary test in short mode")
	}

	binPath := buildTestBinary(t)

	// Locate sample module — pkg/golist/testdata/sample-module/ from the
	// cascade repo root. main_test.go runs with cwd = cmd/cascade/, so the
	// fixture is two levels up.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	sampleModule := filepath.Join(repoRoot, "pkg", "golist", "testdata", "sample-module")

	cmd := exec.Command(binPath, "--changed-files=-", "--root="+sampleModule) //nolint:gosec
	cmd.Stdin = strings.NewReader("pkga/a.go\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cascade end-to-end failed: %v\nstderr: %s\nstdout: %s",
			err, stderr.String(), stdout.String())
	}

	out := strings.TrimRight(stdout.String(), "\n")
	if out == "" {
		t.Fatalf("expected non-empty output for pkga/a.go change; got empty\nstderr: %s", stderr.String())
	}
	// pkga itself must be in the affected set; pkgb (imports pkga) too.
	wantPaths := []string{
		"example.test/sample/pkga",
		"example.test/sample/pkgb",
	}
	for _, want := range wantPaths {
		if !strings.Contains(out, want) {
			t.Errorf("affected set missing %q\nfull output: %s", want, out)
		}
	}
}

// buildTestBinary compiles the cascade binary into a tmpdir with -ldflags
// injecting the test's version constants. Returns the absolute path to the
// built binary. Shared by TestCascadeBinaryVersion / TestCascadeBinaryHelp /
// TestCascadeBinaryEndToEnd.
func buildTestBinary(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	binName := "cascade"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmp, binName)

	ldflags := strings.Join([]string{
		"-X " + versionPkg + ".Version=" + injectedVersion,
		"-X " + versionPkg + ".GitCommit=" + injectedCommit,
		"-X " + versionPkg + ".GitBranch=" + injectedBranch,
		"-X " + versionPkg + ".BuildDate=" + injectedDate,
	}, " ")

	build := exec.Command("go", "build", "-o", binPath, "-ldflags", ldflags, ".")
	build.Stderr = newPrefixWriter(t, "go build: ")
	if err := build.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}
	return binPath
}

// Test-only constants for ldflags injection; shared across binary tests.
const (
	versionPkg      = "github.com/geomyidia/cascade/internal/project"
	injectedVersion = "test-1.0.0"
	injectedCommit  = "abcdef0"
	injectedBranch  = "test-branch"
	injectedDate    = "2026-05-07T18:30:00Z"
)

type prefixWriter struct {
	t      *testing.T
	prefix string
}

func newPrefixWriter(t *testing.T, prefix string) *prefixWriter {
	return &prefixWriter{t: t, prefix: prefix}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s%s", w.prefix, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
