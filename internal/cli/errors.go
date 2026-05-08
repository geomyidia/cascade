package cli

import (
	"errors"
	"fmt"
	"strings"
)

// ErrGitDiffFailed is returned when `git diff` exits with a non-zero status.
// Use errors.As(err, &e) with a *GitDiffError to extract the captured stderr
// and full argv. Mirrors pkg/golist's ErrGoListFailed semantics.
//
// Per EH-15, callers must classify with errors.Is, not string-match against
// Error() output (AP-13).
var ErrGitDiffFailed = errors.New("git diff failed")

// GitDiffError captures the diagnostic context when `git diff` exits with a
// non-zero status. errors.Is(err, ErrGitDiffFailed) returns true; the wrapped
// *exec.ExitError (when applicable) is reachable via errors.As or by calling
// Unwrap directly.
//
// The shape mirrors pkg/golist.ExitError so contributors who already know one
// know the other (EH-08 + EH-16). The Dir field was added to close F-5 in
// docs/dev/0014-go-quality-audit.md, restoring full cousin-shape parity.
type GitDiffError struct {
	// Cmd is the full argv as passed to exec, in order, for reproduction.
	Cmd []string

	// Dir is the working directory the command was run in (typically the
	// absolutized cfg.root from the cli layer). Mirrors *golist.ExitError.Dir.
	Dir string

	// ExitCode is the subprocess exit code (typically 1 or 128 for `git diff`
	// errors; may be other values).
	ExitCode int

	// Stderr is the captured stderr output, verbatim and untruncated.
	// `git diff`'s stderr on real failures (e.g. "fatal: bad revision
	// 'origin/main'") is small and the diagnostic value is high.
	Stderr string

	// underlying is the *exec.ExitError returned by exec.Cmd.Run when
	// available; nil if the failure was reconstructed from a different path.
	// Exposed via Unwrap.
	underlying error
}

// Error returns a one-line summary suitable for logging.
func (e *GitDiffError) Error() string {
	first := strings.TrimSpace(strings.SplitN(e.Stderr, "\n", 2)[0])
	if first == "" {
		return fmt.Sprintf("%s: exit %d", strings.Join(e.Cmd, " "), e.ExitCode)
	}
	return fmt.Sprintf("%s: exit %d: %s", strings.Join(e.Cmd, " "), e.ExitCode, first)
}

// Is reports whether target is ErrGitDiffFailed; this is what makes
// errors.Is(err, ErrGitDiffFailed) true when err is a *GitDiffError.
func (e *GitDiffError) Is(target error) bool {
	return target == ErrGitDiffFailed
}

// Unwrap returns the wrapped *exec.ExitError when one is captured, else nil.
// Callers can use errors.As to extract the *exec.ExitError for low-level
// inspection (e.g. signal info on Unix).
func (e *GitDiffError) Unwrap() error {
	return e.underlying
}
