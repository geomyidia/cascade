package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// gitDiffResult is the io seam's return shape: stdout (a Reader so tests can
// supply bytes.Buffer / strings.Reader / etc.), the raw cmd.Run error (nil on
// success; *exec.ExitError on non-zero; other errors for fork failures /
// cancellation / lookup failures), the captured stderr text, and the actual
// argv the seam used (so the cli layer's diagnostic doesn't drift from the
// real exec invocation; closes F-14). Mirrors pkg/golist's runResult.
type gitDiffResult struct {
	stdout io.Reader
	err    error
	stderr string
	argv   []string
}

// runGitDiff is the function-variable seam over the os/exec call. Production
// builds use defaultRunGitDiff; tests can replace it via withGitDiffSeam to
// drive specific branches (parse-error-after-successful-exec, unexpected exec
// failures, context cancellation, etc.) without spawning a subprocess.
//
// Pattern matches pkg/golist's runGoList seam; see that package's doc for the
// rationale.
var runGitDiff = defaultRunGitDiff

// defaultRunGitDiff shells out to `git diff --name-only <base>..<head>` in
// the configured working directory and captures stdout / stderr. The argv
// it constructs is returned verbatim in the gitDiffResult so the cli layer's
// diagnostic chain (in *GitDiffError.Cmd) reflects the actual exec, not a
// reconstruction that could drift.
//
// gosec G204 (subprocess from variable) is suppressed: argv[0] is the literal
// string "git" (a constant); argv[1:] is constructed from caller-supplied
// refs that are documented as caller-controlled. The package contract makes
// this explicit.
func defaultRunGitDiff(ctx context.Context, base, head, dir string) gitDiffResult {
	argv := []string{"git", "diff", "--name-only", base + ".." + head}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitDiffResult{stdout: &stdout, err: err, stderr: stderr.String(), argv: argv}
}

// classifyGitDiffError converts an exec.Cmd.Run error into the appropriate
// typed error per the package contract. It distinguishes:
//
//   - context cancellation/deadline: returns ctx.Err() wrapped with %w so
//     callers can errors.Is(err, context.Canceled) / errors.Is(err,
//     context.DeadlineExceeded).
//   - subprocess non-zero exit (*exec.ExitError): returns *GitDiffError
//     carrying the full argv, exit code, and captured stderr.
//   - other exec errors (rare; e.g. permission denied on git binary, or git
//     not on PATH): wrapped under ErrGitDiffFailed for category clarity.
//
// ctx is the last param, deliberately deviating from CC-08 (ctx is the
// first parameter). The CC-08 MUST is calibrated to *propagation* — a
// function that passes ctx forward into a blocking call. This classifier
// consults ctx.Err() exactly once, post-hoc, to disambiguate "is this
// error a cancellation?" and never propagates ctx anywhere else; that is
// outside CC-08's intended scope. Mirrors pkg/golist's classifyRunError so
// readers who learn one know the other.
//
// dir is the working directory the seam ran the subprocess in (typically the
// absolutized cfg.root); stored on *GitDiffError.Dir for cousin-shape parity
// with *golist.ExitError (closes F-5 in docs/dev/0014-go-quality-audit.md).
func classifyGitDiffError(err error, argv []string, dir, stderr string, ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("git diff: %w", ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &GitDiffError{
			Cmd:        append([]string(nil), argv...),
			Dir:        dir,
			ExitCode:   exitErr.ExitCode(),
			Stderr:     stderr,
			underlying: exitErr,
		}
	}
	// Unexpected exec failure (e.g. git not on PATH, permission denied).
	// Wrap under ErrGitDiffFailed so callers can still classify by category.
	return fmt.Errorf("%w: %w", ErrGitDiffFailed, err)
}
