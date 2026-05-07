package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/geomyidia/cascade/pkg/golist"
)

// withGitDiffSeam swaps the package-level runGitDiff for the duration of a
// test, restoring the default afterward. Mirrors pkg/golist's withSeam.
func withGitDiffSeam(t *testing.T, fn func(ctx context.Context, base, head, dir string) gitDiffResult) {
	t.Helper()
	saved := runGitDiff
	t.Cleanup(func() { runGitDiff = saved })
	runGitDiff = fn
}

// withGoListSeam swaps runGoListWrapper for the duration of a test.
func withGoListSeam(t *testing.T, fn func(ctx context.Context, tags, patterns []string, opts ...golist.Option) ([]golist.Package, error)) {
	t.Helper()
	saved := runGoListWrapper
	t.Cleanup(func() { runGoListWrapper = saved })
	runGoListWrapper = fn
}

// withSignalContextSeam swaps signalContext for the duration of a test. Most
// tests use a context.Background-derived context so cancellation never fires;
// the cancellation test uses a pre-cancelled context to drive exit 5.
func withSignalContextSeam(t *testing.T, fn func(parent context.Context, sigs ...os.Signal) (context.Context, context.CancelFunc)) {
	t.Helper()
	saved := signalContext
	t.Cleanup(func() { signalContext = saved })
	signalContext = fn
}

// quietSignalContext returns a non-cancelling context for tests that don't
// want signal.NotifyContext's real signal handler running during the test.
// (Without this, registering a SIGINT handler in a test process is benign
// but can interact poorly with -timeout.)
func quietSignalContext(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// installQuietSignalContext is convenience for tests that just want to
// suppress real signal-handler installation.
func installQuietSignalContext(t *testing.T) {
	withSignalContextSeam(t, quietSignalContext)
}

// TestRun_PipelineIntegration (F-6, F-12) drives the full pipeline with
// synthetic seams: runGitDiff returns a synthetic change-set; runGoListWrapper
// returns synthetic []golist.Package with edges. The real depgraph.Build,
// changeset.Resolve, and g.RevDepClosure run on that data and the affected
// set lands on stdout.
//
// This is the structural verification of the M4 retro's claim that the trio
// of pure packages composes without adapter code: runPipeline is the entire
// adapter, four lines.
func TestRun_PipelineIntegration(t *testing.T) {
	installQuietSignalContext(t)
	withGitDiffSeam(t, func(_ context.Context, _, _, _ string) gitDiffResult {
		return gitDiffResult{stdout: strings.NewReader("pkga/a.go\n"), err: nil}
	})
	withGoListSeam(t, func(_ context.Context, _, _ []string, _ ...golist.Option) ([]golist.Package, error) {
		// pkga is the seed; pkgb imports pkga; pkgc is unrelated.
		return []golist.Package{
			{ImportPath: "ex/pkga", Dir: "/m/pkga"},
			{ImportPath: "ex/pkgb", Dir: "/m/pkgb", Imports: []string{"ex/pkga"}},
			{ImportPath: "ex/pkgc", Dir: "/m/pkgc"},
		}, nil
	})

	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--base=origin/main", "--head=HEAD", "--root=/m"},
		strings.NewReader(""), &stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %q", exit, stderr.String())
	}

	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	want := []string{"ex/pkga", "ex/pkgb"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("output not sorted: %v", got)
	}
}

// TestRun_GitDiffFails (F-7) verifies a non-zero git-diff exit maps to exit 2
// and the captured stderr is propagated.
func TestRun_GitDiffFails(t *testing.T) {
	installQuietSignalContext(t)
	withGitDiffSeam(t, func(_ context.Context, _, _, _ string) gitDiffResult {
		return gitDiffResult{
			err:    &exec.ExitError{ProcessState: nil}, // ExitCode() returns -1 with nil ProcessState; that's fine for our test
			stderr: "fatal: bad revision 'origin/main'\n",
		}
	})

	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--base=origin/main"},
		strings.NewReader(""), &stdout, &stderr,
	)
	if exit != 2 {
		t.Errorf("exit = %d, want 2; stderr: %q", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "bad revision") {
		t.Errorf("stderr should propagate git's stderr; got %q", stderr.String())
	}
}

// TestRun_GoListFails (F-8) verifies a go-list failure maps to exit 3.
func TestRun_GoListFails(t *testing.T) {
	installQuietSignalContext(t)
	withGitDiffSeam(t, func(_ context.Context, _, _, _ string) gitDiffResult {
		return gitDiffResult{stdout: strings.NewReader("pkga/a.go\n"), err: nil}
	})
	withGoListSeam(t, func(_ context.Context, _, _ []string, _ ...golist.Option) ([]golist.Package, error) {
		return nil, &golist.ExitError{
			Cmd:      []string{"go", "list", "-deps", "-json", "./..."},
			Dir:      "/m",
			ExitCode: 1,
			Stderr:   "go: updates to go.mod needed\n",
		}
	})

	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--base=origin/main"},
		strings.NewReader(""), &stdout, &stderr,
	)
	if exit != 3 {
		t.Errorf("exit = %d, want 3; stderr: %q", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "go.mod") {
		t.Errorf("stderr should propagate go list's stderr; got %q", stderr.String())
	}
}

// TestRun_StdinChangedFiles (F-9) verifies --changed-files=- reads from
// stdin, trims lines, skips empties.
func TestRun_StdinChangedFiles(t *testing.T) {
	installQuietSignalContext(t)
	withGoListSeam(t, func(_ context.Context, _, _ []string, _ ...golist.Option) ([]golist.Package, error) {
		return []golist.Package{
			{ImportPath: "ex/pkga", Dir: "/m/pkga"},
		}, nil
	})

	stdin := strings.NewReader("  pkga/a.go  \n\n   \n")
	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--changed-files=-", "--root=/m"},
		stdin, &stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %q", exit, stderr.String())
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "ex/pkga" {
		t.Errorf("stdout = %q, want %q", got, "ex/pkga")
	}
}

// TestRun_EmptyResult (F-10) verifies that a change-set with no Go files
// affecting any in-package code yields empty stdout and exit 0.
func TestRun_EmptyResult(t *testing.T) {
	installQuietSignalContext(t)
	withGoListSeam(t, func(_ context.Context, _, _ []string, _ ...golist.Option) ([]golist.Package, error) {
		return []golist.Package{
			{ImportPath: "ex/pkga", Dir: "/m/pkga"},
		}, nil
	})

	// Stdin contains only a non-Go file; changeset.Resolve skips it.
	stdin := strings.NewReader("README.md\n")
	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--changed-files=-", "--root=/m"},
		stdin, &stdout, &stderr,
	)
	if exit != 0 {
		t.Errorf("exit = %d, want 0; stderr: %q", exit, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty; got %q", stdout.String())
	}
}

// TestRun_ContextCancellation (F-11) verifies that a cancelled context
// flowing through the git-diff seam maps to exit 5. The signalContext seam
// is replaced with a pre-cancellable context-creation function so the test
// doesn't need to send real OS signals.
func TestRun_ContextCancellation(t *testing.T) {
	withSignalContextSeam(t, func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		cancel() // pre-cancel: the seam returns an already-cancelled context
		return ctx, cancel
	})
	withGitDiffSeam(t, func(ctx context.Context, _, _, _ string) gitDiffResult {
		// Reflect ctx.Err() back through the seam — simulates the production
		// defaultRunGitDiff's behaviour when exec.CommandContext sees a
		// cancelled context (cmd.Run returns context.Canceled wrapped).
		return gitDiffResult{err: ctx.Err()}
	})

	var stdout, stderr bytes.Buffer
	exit := Run(
		[]string{"--base=origin/main"},
		strings.NewReader(""), &stdout, &stderr,
	)
	if exit != 5 {
		t.Errorf("exit = %d, want 5 (cancelled); stderr: %q", exit, stderr.String())
	}
}

// TestClassifyGitDiffError covers the three-way branch in classifyGitDiffError
// directly: cancelled-context wins, exec.ExitError yields *GitDiffError,
// other errors wrap under ErrGitDiffFailed.
func TestClassifyGitDiffError(t *testing.T) {
	argv := []string{"git", "diff", "--name-only", "a..b"}

	t.Run("cancelled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := classifyGitDiffError(errors.New("subprocess died"), argv, "/m", "stderr text", ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want errors.Is(err, context.Canceled) true", err)
		}
	})

	t.Run("exec_exit_error", func(t *testing.T) {
		ctx := context.Background()
		execErr := &exec.ExitError{}
		err := classifyGitDiffError(execErr, argv, "/m", "fatal: bad ref\n", ctx)
		if !errors.Is(err, ErrGitDiffFailed) {
			t.Errorf("err should be Is ErrGitDiffFailed; got %v", err)
		}
		var ge *GitDiffError
		if !errors.As(err, &ge) {
			t.Fatalf("err should be As *GitDiffError; got %v", err)
		}
		if ge.Stderr != "fatal: bad ref\n" {
			t.Errorf("ge.Stderr = %q, want %q", ge.Stderr, "fatal: bad ref\n")
		}
	})

	t.Run("other_exec_error", func(t *testing.T) {
		ctx := context.Background()
		err := classifyGitDiffError(errors.New("permission denied"), argv, "/m", "", ctx)
		if !errors.Is(err, ErrGitDiffFailed) {
			t.Errorf("err should be Is ErrGitDiffFailed; got %v", err)
		}
	})
}

// TestGitDiffError_Methods covers Error/Is/Unwrap on *GitDiffError directly.
func TestGitDiffError_Methods(t *testing.T) {
	t.Run("error_with_stderr_first_line", func(t *testing.T) {
		ge := &GitDiffError{
			Cmd:      []string{"git", "diff", "--name-only", "a..b"},
			ExitCode: 128,
			Stderr:   "fatal: bad revision 'a'\nmore noise\n",
		}
		got := ge.Error()
		want := "git diff --name-only a..b: exit 128: fatal: bad revision 'a'"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("error_with_empty_stderr", func(t *testing.T) {
		ge := &GitDiffError{
			Cmd:      []string{"git", "diff", "--name-only", "a..b"},
			ExitCode: 1,
			Stderr:   "",
		}
		got := ge.Error()
		want := "git diff --name-only a..b: exit 1"
		if got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("is_matches_sentinel", func(t *testing.T) {
		ge := &GitDiffError{}
		if !ge.Is(ErrGitDiffFailed) {
			t.Error("Is(ErrGitDiffFailed) = false, want true")
		}
		if ge.Is(errors.New("other")) {
			t.Error("Is(other) = true, want false")
		}
	})

	t.Run("unwrap_returns_underlying", func(t *testing.T) {
		execErr := &exec.ExitError{}
		ge := &GitDiffError{underlying: execErr}
		if got := ge.Unwrap(); got != execErr {
			t.Errorf("Unwrap() = %v, want %v", got, execErr)
		}
	})
}

// TestMapError covers each error category against its expected exit code.
func TestMapError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"context_canceled", context.Canceled, 5},
		{"context_deadline", context.DeadlineExceeded, 5},
		{"git_diff_failed_sentinel", ErrGitDiffFailed, 2},
		{"git_diff_failed_typed", &GitDiffError{Cmd: []string{"git"}, ExitCode: 1}, 2},
		{"go_list_failed", golist.ErrGoListFailed, 3},
		{"go_not_found", golist.ErrGoNotFound, 3},
		{"go_list_parse_failed", golist.ErrParseFailed, 3},
		{"flag_or_input", errFlagOrInput, 1},
		{"unknown", errors.New("synthetic unmapped error"), 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapError(tt.err); got != tt.want {
				t.Errorf("mapError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestDefaultRunGitDiff_Smoke covers the actual exec line in
// defaultRunGitDiff. Skipped under -short because it requires the `git`
// binary on PATH and runs against the cascade repo itself (any git repo
// would do; the test cwd is internal/cli/, which is part of the cascade
// repo).
//
// Mirrors pkg/golist/golist_test.go's TestRun_SampleModule strategy: cover
// the otherwise-untestable subprocess line via a real-world smoke test that
// CI runs (no -short). The seam-replaceable runGitDiff variable hooks the
// rest of the code path; this single test covers the production default.
func TestDefaultRunGitDiff_Smoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess smoke test in short mode")
	}

	// HEAD..HEAD is always an empty diff and always succeeds.
	// Empty dir → cmd inherits the test process's cwd (internal/cli, inside
	// the cascade repo). Explicit dir="." → cmd uses that directory; both
	// branches are covered by running both subcases.
	for _, dir := range []string{"", "."} {
		t.Run("dir="+dir, func(t *testing.T) {
			res := defaultRunGitDiff(context.Background(), "HEAD", "HEAD", dir)
			if res.err != nil {
				t.Fatalf("defaultRunGitDiff(HEAD..HEAD, dir=%q) error: %v\nstderr: %s",
					dir, res.err, res.stderr)
			}
			if res.stdout == nil {
				t.Error("res.stdout is nil; should be a Reader (possibly empty)")
			}
		})
	}
}

// TestScanLines_Errors covers the bufio.Scanner error branch.
func TestScanLines_Errors(t *testing.T) {
	r := &errReader{err: errors.New("synthetic read error")}
	_, err := scanLines(r)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errFlagOrInput) {
		t.Errorf("err should wrap errFlagOrInput; got %v", err)
	}
}

// errReader returns an error on every Read call. Used to drive scanLines'
// scanner-error branch.
type errReader struct{ err error }

func (r *errReader) Read(_ []byte) (int, error) { return 0, r.err }

// Compile-time assertion that errReader implements io.Reader.
var _ io.Reader = (*errReader)(nil)
