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
// cancellation / lookup failures), and the captured stderr text. Mirrors
// pkg/golist's runResult.
type gitDiffResult struct {
	stdout io.Reader
	err    error
	stderr string
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
// the configured working directory and captures stdout / stderr.
//
// gosec G204 (subprocess from variable) is suppressed: argv[0] is the literal
// string "git" (a constant); argv[1:] is constructed from caller-supplied
// refs that are documented as caller-controlled. The package contract makes
// this explicit.
func defaultRunGitDiff(ctx context.Context, base, head, dir string) gitDiffResult {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", base+".."+head) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitDiffResult{stdout: &stdout, err: err, stderr: stderr.String()}
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
// ctx is the last param (against EH-08 ordering convention) because it's only
// used to detect cancellation post-hoc, not for propagation. Mirrors
// pkg/golist's classifyRunError.
func classifyGitDiffError(err error, argv []string, _, stderr string, ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("git diff: %w", ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &GitDiffError{
			Cmd:        append([]string(nil), argv...),
			ExitCode:   exitErr.ExitCode(),
			Stderr:     stderr,
			underlying: exitErr,
		}
	}
	// Unexpected exec failure (e.g. git not on PATH, permission denied).
	// Wrap under ErrGitDiffFailed so callers can still classify by category.
	return fmt.Errorf("%w: %w", ErrGitDiffFailed, err)
}
