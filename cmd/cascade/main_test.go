package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Layer 1: in-process unit tests of run(). These own the package's
// statement coverage so make coverage-check (90% Makefile gate) stays
// green in M1 with no implementation in golist/depgraph/changeset yet.

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // substring; "" means stdout must be empty
		wantStderr string // substring; "" means stderr is unchecked
	}{
		{
			name:       "version flag",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "cascade dev",
			wantStderr: "",
		},
		{
			name:       "help flag",
			args:       []string{"--help"},
			wantCode:   0,
			wantStdout: "",
			wantStderr: "Usage of cascade",
		},
		{
			name:       "flag parse error",
			args:       []string{"--bogus"},
			wantCode:   1,
			wantStdout: "",
			wantStderr: "flag provided but not defined",
		},
		{
			name:       "no flags",
			args:       []string{},
			wantCode:   2,
			wantStdout: "",
			wantStderr: "not yet implemented",
		},
		{
			name:       "unexpected positional argument",
			args:       []string{"surprise"},
			wantCode:   2,
			wantStdout: "",
			wantStderr: `unexpected argument "surprise"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tc.args, &stdout, &stderr)

			if got != tc.wantCode {
				t.Errorf("run(%v) exit = %d, want %d\nstdout: %q\nstderr: %q",
					tc.args, got, tc.wantCode, stdout.String(), stderr.String())
			}
			if tc.wantStdout == "" {
				if stdout.Len() != 0 {
					t.Errorf("run(%v) wrote unexpected stdout: %q", tc.args, stdout.String())
				}
			} else if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("run(%v) stdout = %q, want substring %q",
					tc.args, stdout.String(), tc.wantStdout)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("run(%v) stderr = %q, want substring %q",
					tc.args, stderr.String(), tc.wantStderr)
			}
		})
	}
}

// Layer 2: end-to-end smoke test. Builds the binary with ldflags injecting
// known values, then exec's it with --version. Proves the build +
// version-injection chain end-to-end. Skipped under -short because it
// shells out to `go build` and is meaningfully slower than the unit tests.

func TestCascadeBinaryVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end binary test in short mode")
	}

	tmp := t.TempDir()
	binName := "cascade"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmp, binName)

	const (
		injectedVersion = "test-1.0.0"
		injectedCommit  = "abcdef"
	)

	ldflags := "-X main.Version=" + injectedVersion +
		" -X main.GitCommit=" + injectedCommit

	build := exec.Command("go", "build", "-o", binPath, "-ldflags", ldflags, ".")
	build.Stderr = newPrefixWriter(t, "go build: ")
	if err := build.Run(); err != nil {
		t.Fatalf("go build failed: %v", err)
	}

	cmd := exec.Command(binPath, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s --version failed: %v\nstderr: %s", binPath, err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "cascade "+injectedVersion) {
		t.Errorf("--version output missing injected Version: %q", out)
	}
	if !strings.Contains(out, "commit "+injectedCommit) {
		t.Errorf("--version output missing injected GitCommit: %q", out)
	}
}

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
