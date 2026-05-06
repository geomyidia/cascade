package golist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"
)

// Package mirrors the subset of `go list -deps -json` output that
// cascade's downstream packages (depgraph, changeset) consume. Fields
// not consumed are deliberately omitted — adding a field is a decision
// driven by a downstream need, not a default passthrough.
//
// All path fields are absolute (as `go list` reports them). All slice
// fields are nil-safe (a package with no test imports has TestImports ==
// nil, not []string{}; callers should not distinguish).
type Package struct {
	// Identity
	ImportPath string `json:"ImportPath"`
	Dir        string `json:"Dir"`

	// Source files in this package, by category
	GoFiles        []string `json:"GoFiles,omitempty"`
	TestGoFiles    []string `json:"TestGoFiles,omitempty"`
	XTestGoFiles   []string `json:"XTestGoFiles,omitempty"`
	IgnoredGoFiles []string `json:"IgnoredGoFiles,omitempty"`

	// Direct imports, by source category
	Imports      []string `json:"Imports,omitempty"`
	TestImports  []string `json:"TestImports,omitempty"`
	XTestImports []string `json:"XTestImports,omitempty"`

	// Categorisation (used downstream to filter stdlib / external deps)
	Standard bool    `json:"Standard,omitempty"`
	Module   *Module `json:"Module,omitempty"`
}

// Module identifies which Go module a Package belongs to. Nil on Package
// when the package is from the standard library.
type Module struct {
	// Path is the module path, e.g. "github.com/geomyidia/cascade".
	Path string `json:"Path"`

	// Main is true when this is the main module being built (the one
	// the working directory belongs to), false for dependencies.
	Main bool `json:"Main,omitempty"`
}

// runConfig holds the configuration assembled from functional Options.
// Unexported: callers compose configuration only via WithXxx constructors.
type runConfig struct {
	dir   string
	env   []string
	goBin string
}

// Option configures a Run call. Apply options after the required
// positional args. See WithDir, WithEnv, WithGoBin.
type Option func(*runConfig)

// WithDir sets the working directory for the spawned `go list` process.
// Defaults to the caller's current working directory.
func WithDir(dir string) Option {
	return func(c *runConfig) { c.dir = dir }
}

// WithEnv overrides the environment for the spawned `go list` process.
// Defaults to os.Environ() (i.e. inheriting the parent process env).
// Useful for hermetic test setups.
func WithEnv(env []string) Option {
	return func(c *runConfig) { c.env = env }
}

// WithGoBin overrides the `go` binary path. Defaults to "go" (resolved
// via $PATH). Useful for testing against non-default toolchains.
func WithGoBin(bin string) Option {
	return func(c *runConfig) { c.goBin = bin }
}

// runResult is the io seam's return shape: stdout (a Reader so tests
// can supply bytes.Buffer / strings.Reader / etc.), the raw cmd.Run
// error (nil on success; *exec.ExitError on non-zero; other errors
// for fork failures / cancellation / lookup failures), and the
// captured stderr text.
type runResult struct {
	stdout io.Reader
	err    error
	stderr string
}

// runGoList is the function-variable seam over the os/exec call. Production
// builds use defaultRunGoList; tests can replace it to drive specific
// branches (parse-error-after-successful-exec, unexpected exec failures,
// etc.) without spawning a subprocess.
var runGoList = defaultRunGoList

// defaultRunGoList shells out to `go list` and captures stdout / stderr.
// The single line that's not statement-coverable from in-process tests
// is cmd.Run() — covered by Layer-2 subprocess tests instead.
func defaultRunGoList(ctx context.Context, argv []string, dir string, env []string) runResult {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return runResult{stdout: &stdout, err: err, stderr: stderr.String()}
}

// Run shells out to `go list -deps -json -tags=<tags> <patterns...>` in
// the configured working directory, parses the streaming JSON output,
// and returns the parsed packages in encounter order (alphabetical by
// import path within each module — `go list`'s own order).
//
// Behaviour on error:
//   - If `go` is not on PATH (or at WithGoBin's path): returns an error
//     wrapping ErrGoNotFound. errors.Is(err, ErrGoNotFound) is true.
//   - If `go list` exits non-zero: returns a *ExitError with stderr
//     captured. errors.Is(err, ErrGoListFailed) is true; errors.As(&e)
//     extracts the *ExitError.
//   - If JSON parsing fails: returns a *ParseError with the offending
//     payload. errors.Is(err, ErrParseFailed) is true; errors.As(&e)
//     extracts the *ParseError.
//   - If ctx is cancelled or its deadline expires: the spawned `go list`
//     process is killed (via exec.CommandContext); the returned error
//     wraps ctx.Err() so errors.Is(err, context.Canceled) /
//     errors.Is(err, context.DeadlineExceeded) match.
//
// Concurrency: Run may be called concurrently from multiple goroutines.
// Each call spawns its own subprocess. The returned []Package is safe
// for concurrent reads after Run returns; callers must not mutate it.
//
// Defaults applied to inputs:
//   - patterns: nil or empty → []string{"./..."} (the common case).
//   - tags: nil or empty → the -tags flag is omitted entirely from the
//     argv (not passed as -tags="").
func Run(ctx context.Context, tags []string, patterns []string, opts ...Option) ([]Package, error) {
	cfg := applyOptions(opts...)
	argv := buildArgv(cfg.goBin, tags, patterns)

	res := runGoList(ctx, argv, cfg.dir, cfg.env)
	if res.err != nil {
		return nil, classifyRunError(res.err, argv, cfg.dir, res.stderr, ctx)
	}

	pkgs, perr := parseStream(res.stdout)
	if perr != nil {
		return nil, perr
	}
	return pkgs, nil
}

// applyOptions composes a runConfig from the supplied functional
// options. Defaults: dir="" (cwd), env=nil (inherit), goBin="go".
func applyOptions(opts ...Option) *runConfig {
	cfg := &runConfig{goBin: "go"}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// buildArgv constructs the argv for the `go list` invocation. The
// flag layout is fixed: `<goBin> list -deps -json [-tags=<csv>] <patterns...>`.
// Empty tags omits the -tags flag; empty patterns substitutes ./...
func buildArgv(goBin string, tags []string, patterns []string) []string {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	argv := []string{goBin, "list", "-deps", "-json"}
	if len(tags) > 0 {
		argv = append(argv, "-tags="+strings.Join(tags, ","))
	}
	argv = append(argv, patterns...)
	return argv
}

// classifyRunError converts an exec.Cmd.Run error into the appropriate
// typed error per the Run contract. It distinguishes:
//
//   - context cancellation/deadline: returns ctx.Err() wrapped with %w.
//   - go binary not on PATH (exec.ErrNotFound): returns a wrapped
//     ErrGoNotFound (errors.Is matches).
//   - subprocess non-zero exit (*exec.ExitError): returns *ExitError.
//   - other exec errors (rare): returned wrapped with %w under
//     ErrGoListFailed for category clarity.
//
// ctx is the last param (against EH-08 ordering convention) because it's
// only used to detect cancellation post-hoc, not for propagation.
func classifyRunError(err error, argv []string, dir string, stderr string, ctx context.Context) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("go list: %w", ctxErr)
	}
	// exec.ErrNotFound covers `exec.LookPath` failure (name without slashes
	// not on PATH). fs.ErrNotExist covers absolute / relative path that
	// doesn't exist on disk (the WithGoBin("/bogus/path") case). Both mean
	// "go binary not found" from the caller's perspective.
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrGoNotFound, err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{
			Cmd:        append([]string(nil), argv...),
			Dir:        dir,
			ExitCode:   exitErr.ExitCode(),
			Stderr:     stderr,
			underlying: exitErr,
		}
	}
	// Unexpected exec failure (e.g. permission denied on the go binary).
	// Wrap under ErrGoListFailed so callers can still classify.
	return fmt.Errorf("%w: %w", ErrGoListFailed, err)
}

// Compile-time assertions that *ExitError and *ParseError satisfy the
// error interface and that Module is exported (avoiding accidental
// regressions). Not strictly required, but gives staticcheck a hook.
var (
	_ error  = (*ExitError)(nil)
	_ error  = (*ParseError)(nil)
	_ Option = WithDir("")
	// Reference io to keep the import alive for future use; the streaming
	// decoder lives in parse.go and consumes this package's io types via
	// the parseStream helper.
	_ = io.EOF
)
