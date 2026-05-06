package golist

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for category matching via errors.Is. The typed errors
// below (*ExitError, *ParseError) implement Is to match the corresponding
// sentinel; callers should match against the sentinel for category and use
// errors.As to extract the typed error for diagnostic context.
//
// These follow the EH-36 sentinel naming convention (ErrXxx) and are
// the only mechanism callers should use to classify errors — string-match
// against Error() output is forbidden (AP-02).
var (
	// ErrGoNotFound is returned when the `go` binary cannot be found on
	// $PATH (or at the path configured via WithGoBin). The wrapped error
	// is exec.ErrNotFound.
	ErrGoNotFound = errors.New("go binary not found")

	// ErrGoListFailed is returned when `go list` exits with a non-zero
	// status. Use errors.As(err, &e) with a *ExitError to extract the
	// stderr capture and full argv.
	ErrGoListFailed = errors.New("go list failed")

	// ErrParseFailed is returned when streaming JSON decoding of `go list`
	// output fails partway through. Use errors.As(err, &e) with a
	// *ParseError to extract the byte offset and offending payload.
	ErrParseFailed = errors.New("go list output parse failed")
)

// ParseErrorMaxPayload is the maximum number of bytes captured in
// ParseError.Payload. Larger payloads are truncated; the truncation is
// not separately marked because the offset + cause already convey the
// failure point.
const ParseErrorMaxPayload = 4096

// ExitError captures the diagnostic context when `go list` exits with a
// non-zero status. errors.Is(err, ErrGoListFailed) returns true; the
// wrapped *exec.ExitError (when applicable) is reachable via errors.As
// or by calling Unwrap directly.
type ExitError struct {
	// Cmd is the full argv as passed to exec, in order, for reproduction.
	Cmd []string

	// Dir is the working directory the command was run in (absolute path
	// when the configured WithDir was absolute; otherwise as configured).
	Dir string

	// ExitCode is the subprocess exit code (typically 1 for `go list`
	// errors; may be other values).
	ExitCode int

	// Stderr is the captured stderr output, verbatim and untruncated.
	// `go list`'s stderr on real failures (e.g. "go: updates to go.mod
	// needed") is small and the diagnostic value is high.
	Stderr string

	// underlying is the *exec.ExitError returned by exec.Cmd.Run when
	// available; nil if the failure was reconstructed from a different
	// path. Exposed via Unwrap.
	underlying error
}

// Error returns a one-line summary suitable for logging.
func (e *ExitError) Error() string {
	first := strings.TrimSpace(strings.SplitN(e.Stderr, "\n", 2)[0])
	if first == "" {
		return fmt.Sprintf("%s: exit %d", strings.Join(e.Cmd, " "), e.ExitCode)
	}
	return fmt.Sprintf("%s: exit %d: %s", strings.Join(e.Cmd, " "), e.ExitCode, first)
}

// Is reports whether target is ErrGoListFailed; this is what makes
// errors.Is(err, ErrGoListFailed) true when err is an *ExitError.
func (e *ExitError) Is(target error) bool {
	return target == ErrGoListFailed
}

// Unwrap returns the wrapped *exec.ExitError when one is captured, else nil.
// Callers can use errors.As to extract the *exec.ExitError for low-level
// inspection (e.g. signal info on Unix).
func (e *ExitError) Unwrap() error {
	return e.underlying
}

// ParseError captures the diagnostic context when JSON decoding of
// `go list` output fails. errors.Is(err, ErrParseFailed) returns true;
// the wrapped json error is reachable via errors.As(err, &e).Cause or
// errors.Unwrap.
type ParseError struct {
	// Offset is the byte offset in the streaming input where decoding
	// stopped. Useful for correlating with the captured Payload.
	Offset int64

	// Payload is the offending payload, truncated to ParseErrorMaxPayload
	// bytes. Captures bytes the decoder had buffered plus any remaining
	// readable input at the failure point.
	Payload string

	// Cause is the underlying json error (typically *json.SyntaxError or
	// io.ErrUnexpectedEOF for truncated input).
	Cause error
}

// Error returns a one-line summary suitable for logging.
func (e *ParseError) Error() string {
	return fmt.Sprintf("go list output parse failed at offset %d: %v", e.Offset, e.Cause)
}

// Is reports whether target is ErrParseFailed.
func (e *ParseError) Is(target error) bool {
	return target == ErrParseFailed
}

// Unwrap returns the underlying json error so errors.Is/errors.As can
// reach json.SyntaxError, io.ErrUnexpectedEOF, etc.
func (e *ParseError) Unwrap() error {
	return e.Cause
}
