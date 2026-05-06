package golist

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// withSeam swaps the package-level runGoList for the duration of a
// test, restoring the default afterward.
func withSeam(t *testing.T, fn func(ctx context.Context, argv []string, dir string, env []string) runResult) {
	t.Helper()
	saved := runGoList
	t.Cleanup(func() { runGoList = saved })
	runGoList = fn
}

// TestRun_ParseErrorAfterSuccessfulExec — covers the "go list returned
// 0 but its stdout was malformed" path inside Run. Drives the seam to
// return success + bogus stdout.
func TestRun_ParseErrorAfterSuccessfulExec(t *testing.T) {
	withSeam(t, func(ctx context.Context, argv []string, dir string, env []string) runResult {
		return runResult{
			stdout: bytes.NewReader([]byte(`{this is not valid json,,,}`)),
			err:    nil,
			stderr: "",
		}
	})

	pkgs, err := Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected ParseError, got %d pkgs", len(pkgs))
	}
	if !errors.Is(err, ErrParseFailed) {
		t.Errorf("errors.Is(err, ErrParseFailed) = false; got %v", err)
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Errorf("errors.As did not extract *ParseError")
	}
}

// TestRun_UnexpectedExecError — covers the final fallthrough in
// classifyRunError ("not ctx-cancelled, not ErrNotFound, not
// *exec.ExitError"). Drives the seam to return an arbitrary error.
func TestRun_UnexpectedExecError(t *testing.T) {
	synthetic := errors.New("permission denied / generic exec failure")
	withSeam(t, func(ctx context.Context, argv []string, dir string, env []string) runResult {
		return runResult{stdout: nil, err: synthetic, stderr: "boom"}
	})

	_, err := Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrGoListFailed) {
		t.Errorf("errors.Is(err, ErrGoListFailed) = false; got %v", err)
	}
	if !errors.Is(err, synthetic) {
		t.Errorf("errors.Is(err, synthetic) = false; got %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("Error() = %q, want to contain synthetic error message", err.Error())
	}
}

// TestRun_SeamPropagatesArgsAndDir — confirms the seam receives the
// argv and dir that Run constructs, so future readers know the seam
// is the right place to inspect the actual command for testing.
func TestRun_SeamPropagatesArgsAndDir(t *testing.T) {
	var captured struct {
		argv []string
		dir  string
		env  []string
	}
	withSeam(t, func(ctx context.Context, argv []string, dir string, env []string) runResult {
		captured.argv = append([]string(nil), argv...)
		captured.dir = dir
		captured.env = append([]string(nil), env...)
		return runResult{stdout: bytes.NewReader(nil), err: nil, stderr: ""}
	})

	_, err := Run(context.Background(),
		[]string{"alpha", "beta"},
		[]string{"./pkga"},
		WithDir("/some/dir"),
		WithEnv([]string{"K=V"}),
		WithGoBin("/usr/local/bin/go"))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	wantArgv := []string{"/usr/local/bin/go", "list", "-deps", "-json", "-tags=alpha,beta", "./pkga"}
	if len(captured.argv) != len(wantArgv) {
		t.Fatalf("argv = %v, want %v", captured.argv, wantArgv)
	}
	for i, w := range wantArgv {
		if captured.argv[i] != w {
			t.Errorf("argv[%d] = %q, want %q", i, captured.argv[i], w)
		}
	}
	if captured.dir != "/some/dir" {
		t.Errorf("dir = %q, want /some/dir", captured.dir)
	}
	if len(captured.env) != 1 || captured.env[0] != "K=V" {
		t.Errorf("env = %v, want [K=V]", captured.env)
	}
}

// TestBuildArgv_TagsOmittedWhenEmpty — direct unit test on buildArgv
// for the "empty/nil tags omits the -tags flag" contract.
func TestBuildArgv_TagsOmittedWhenEmpty(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		patterns []string
		wantHas  []string
		wantNoT  bool // wants no -tags= flag in argv
	}{
		{"nil tags", nil, []string{"./..."}, []string{"list", "-deps", "-json", "./..."}, true},
		{"empty tags", []string{}, []string{"./..."}, []string{"list", "-deps", "-json", "./..."}, true},
		{"single tag", []string{"alpha"}, []string{"./..."}, []string{"-tags=alpha", "./..."}, false},
		{"multiple tags joined", []string{"alpha", "beta"}, []string{"./..."}, []string{"-tags=alpha,beta", "./..."}, false},
		{"nil patterns defaults to ./...", []string{"alpha"}, nil, []string{"-tags=alpha", "./..."}, false},
		{"empty patterns defaults to ./...", []string{"alpha"}, []string{}, []string{"-tags=alpha", "./..."}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argv := buildArgv("go", tc.tags, tc.patterns)
			joined := strings.Join(argv, " ")
			for _, want := range tc.wantHas {
				if !strings.Contains(joined, want) {
					t.Errorf("argv = %v, missing %q", argv, want)
				}
			}
			if tc.wantNoT {
				for _, a := range argv {
					if strings.HasPrefix(a, "-tags=") {
						t.Errorf("argv contains -tags flag: %v (expected omitted)", argv)
					}
				}
			}
		})
	}
}
