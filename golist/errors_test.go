package golist

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestExitError_Is_MatchesSentinel(t *testing.T) {
	e := &ExitError{Cmd: []string{"go", "list"}, ExitCode: 1, Stderr: "boom"}
	if !errors.Is(e, ErrGoListFailed) {
		t.Errorf("errors.Is(%T, ErrGoListFailed) = false, want true", e)
	}
	if errors.Is(e, ErrParseFailed) {
		t.Errorf("errors.Is(%T, ErrParseFailed) = true, want false", e)
	}
}

func TestExitError_As_Extracts(t *testing.T) {
	want := &ExitError{Cmd: []string{"go", "list"}, ExitCode: 2, Stderr: "x"}
	var got *ExitError
	if !errors.As(want, &got) {
		t.Fatalf("errors.As did not extract *ExitError")
	}
	if got != want {
		t.Errorf("errors.As extracted wrong instance: got %p want %p", got, want)
	}
}

func TestExitError_Unwrap_ReturnsExecError(t *testing.T) {
	// We can't easily fabricate a real *exec.ExitError without running a
	// subprocess, but we can verify Unwrap returns the field as set.
	wrapped := errors.New("synthetic exec error")
	e := &ExitError{Cmd: []string{"go"}, underlying: wrapped}
	if got := errors.Unwrap(e); got != wrapped {
		t.Errorf("Unwrap = %v, want %v", got, wrapped)
	}
	// Empty case
	e2 := &ExitError{}
	if got := errors.Unwrap(e2); got != nil {
		t.Errorf("Unwrap on empty = %v, want nil", got)
	}
}

func TestExitError_Error_Format(t *testing.T) {
	tests := []struct {
		name     string
		e        *ExitError
		wantSubs []string
	}{
		{
			name: "with stderr first line",
			e: &ExitError{
				Cmd:      []string{"go", "list", "-deps", "./..."},
				ExitCode: 1,
				Stderr:   "go: updates to go.mod needed\n  more details\n",
			},
			wantSubs: []string{"go list -deps ./...", "exit 1", "go: updates to go.mod needed"},
		},
		{
			name: "without stderr",
			e: &ExitError{
				Cmd:      []string{"go", "list"},
				ExitCode: 2,
				Stderr:   "",
			},
			wantSubs: []string{"go list", "exit 2"},
		},
		{
			name: "stderr is whitespace only",
			e: &ExitError{
				Cmd:      []string{"go", "list"},
				ExitCode: 2,
				Stderr:   "   \n  \n",
			},
			wantSubs: []string{"go list", "exit 2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.e.Error()
			for _, sub := range tc.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("Error() = %q, missing substring %q", got, sub)
				}
			}
		})
	}
}

func TestParseError_Is_MatchesSentinel(t *testing.T) {
	e := &ParseError{Offset: 42, Cause: io.ErrUnexpectedEOF}
	if !errors.Is(e, ErrParseFailed) {
		t.Errorf("errors.Is(%T, ErrParseFailed) = false, want true", e)
	}
	if errors.Is(e, ErrGoListFailed) {
		t.Errorf("errors.Is(%T, ErrGoListFailed) = true, want false", e)
	}
}

func TestParseError_As_Extracts(t *testing.T) {
	want := &ParseError{Offset: 7, Cause: errors.New("syntax")}
	var got *ParseError
	if !errors.As(want, &got) {
		t.Fatalf("errors.As did not extract *ParseError")
	}
	if got != want {
		t.Errorf("errors.As extracted wrong instance: got %p want %p", got, want)
	}
}

func TestParseError_Unwrap_ReachesCause(t *testing.T) {
	cause := errors.New("inner")
	e := &ParseError{Offset: 0, Cause: cause}
	if got := errors.Unwrap(e); got != cause {
		t.Errorf("Unwrap = %v, want %v", got, cause)
	}
	// errors.Is reaches the cause as well
	wrappedCause := errors.New("specific-cause")
	e2 := &ParseError{Cause: wrappedCause}
	if !errors.Is(e2, wrappedCause) {
		t.Errorf("errors.Is failed to reach Cause via Unwrap chain")
	}
}

func TestParseError_Error_Format(t *testing.T) {
	e := &ParseError{Offset: 1234, Cause: io.ErrUnexpectedEOF}
	got := e.Error()
	for _, sub := range []string{"parse failed", "1234", "unexpected EOF"} {
		if !strings.Contains(got, sub) {
			t.Errorf("Error() = %q, missing substring %q", got, sub)
		}
	}
}

func TestErrGoNotFound_IsExecErrNotFoundCompatible(t *testing.T) {
	// Verify our wrapping pattern works: a wrapped ErrGoNotFound + exec.ErrNotFound
	// should match both via errors.Is.
	wrapped := errorJoin(ErrGoNotFound, exec.ErrNotFound)
	if !errors.Is(wrapped, ErrGoNotFound) {
		t.Errorf("errors.Is(wrapped, ErrGoNotFound) = false, want true")
	}
	if !errors.Is(wrapped, exec.ErrNotFound) {
		t.Errorf("errors.Is(wrapped, exec.ErrNotFound) = false, want true")
	}
}

// errorJoin builds an error whose chain reaches both inputs (mirrors
// what classifyRunError does with fmt.Errorf("%w: %v", a, b) — but
// without losing the second one to %v formatting). Used here only for
// the ErrGoNotFound chain test.
func errorJoin(outer, inner error) error {
	return errors.Join(outer, inner)
}
