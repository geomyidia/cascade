package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/geomyidia/cascade/internal/project"
	"github.com/geomyidia/cascade/pkg/changeset"
	"github.com/geomyidia/cascade/pkg/depgraph"
	"github.com/geomyidia/cascade/pkg/golist"
)

// Process exit codes per the M5 contract. See doc.go for the full table.
const (
	exitSuccess      = 0
	exitFlagError    = 1
	exitGitFailed    = 2
	exitGoListFailed = 3
	exitInternal     = 4
	exitCancelled    = 5
)

// errFlagOrInput is the sentinel category for any failure rooted in user
// input — flag parse error, missing required flag, stdin/file read failure,
// or unexpected positional argument. All such errors map to exitFlagError.
var errFlagOrInput = errors.New("flag or input error")

// helpText is the inline --help body. Kept here so `cascade --help` is
// self-sufficient and the binary documents its own contract.
//
// keep in sync with README.md's CLI-usage section + exit-code table.
const helpText = `cascade — reverse-transitive closure of a Go change-set under the imports relation

Usage:
  cascade --base <ref> [--head <ref>] [--tags <tags>] [--root <dir>]
  cascade --changed-files=- [--tags <tags>] [--root <dir>]    (read from stdin)
  cascade --changed-files=<path> [--tags <tags>] [--root <dir>]
  cascade --version
  cascade --help

Flags:
  --tags          comma-separated build tags passed to 'go list -tags='
  --base          base git ref (e.g. origin/main); required unless --changed-files
  --head          head git ref (default: HEAD)
  --changed-files path with one change-set entry per line; '-' reads stdin
  --root          working directory for 'go list' and module-root for resolution (default: .)
  --version       print version metadata and exit
  --help          print this usage and exit

Output:
  One affected import path per line, sorted lexicographically.

Exit codes:
  0  success (output may be empty if no Go files changed)
  1  flag-parse error, missing required flags, or stdin/file read failure
  2  git diff failed
  3  go list failed
  4  internal logic error (should never occur — surface as a real bug)
  5  cancelled or interrupted (SIGINT, SIGTERM, or context cancellation)

Documentation: https://github.com/geomyidia/cascade
`

// config holds the parsed flag values plus pipeline-step inputs.
type config struct {
	tags         []string
	base         string
	head         string
	changedFiles string
	root         string
	showVersion  bool
	showHelp     bool
}

// runGoListWrapper is the function-variable seam over golist.Run. Tests in
// seam_test.go replace it to drive the pipeline with synthetic package data
// (or synthetic errors) without a real go list invocation. Production builds
// dispatch to golist.Run unchanged.
var runGoListWrapper = golist.Run

// signalContext is the function-variable seam over signal.NotifyContext.
// Tests in seam_test.go replace it with a deterministic context-creation
// function (typically a pre-cancellable context) so cancellation behaviour
// can be exercised without sending real OS signals to the test process.
var signalContext = signal.NotifyContext

// Run is the testable entry point for the cascade CLI. It parses args, runs
// the pipeline, writes the affected-package set to stdout, and returns the
// process exit code per the contract documented in the package comment.
//
// stdin is read only when --changed-files=- is passed; otherwise ignored.
// stderr is used for diagnostic and error output; never for primary output.
//
// Errors are wrapped, never swallowed. Every failure path maps to a specific
// exit code per the contract; unmapped errors map to exit 4 (internal).
//
// SIGINT and SIGTERM are caught via signal.NotifyContext; cancelling the
// context kills the in-flight subprocess (git diff or go list) and returns
// exit 5.
//
// The --help flag prints to stdout per GNU convention; `cascade -h` (or any
// flag-parse failure) prints to stderr per stdlib flag's default. Both go
// through the same helpText source so the content is identical.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, cancel := signalContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// `-h` triggered flag's built-in help routing; flag already
			// printed helpText to stderr via the FlagSet's Usage. Return
			// success — the user explicitly asked for help.
			return exitSuccess
		}
		// For flag.Parse failures, flag's auto-Usage already wrote the
		// bare error + helpText to stderr; the explicit "cascade: ..."
		// line below adds our wrapped-error summary so the diagnostic
		// chain is visible. For our own validation errors (NArg>0,
		// missing required flags), this is the only stderr write.
		fmt.Fprintln(stderr, "cascade:", err)
		return mapError(err)
	}

	if cfg.showVersion {
		fmt.Fprintf(stdout, "cascade %s (build %s)\n",
			project.VersionString(), project.BuildString())
		return exitSuccess
	}

	if cfg.showHelp {
		// Explicit --help: route to stdout per GNU convention (Q4 lean).
		fmt.Fprint(stdout, helpText)
		return exitSuccess
	}

	// Absolutize cfg.root once after flag parse so the four downstream
	// consumers (runGitDiff, classifyGitDiffError, golist.WithDir,
	// changeset.WithModuleRoot) all receive a path resolved against the
	// process cwd exactly once. filepath.Abs(".") and filepath.Abs("")
	// both resolve to the cwd. On error, leave as-is and let the
	// library-layer absolutization in changeset.Resolve catch it.
	// Closes bug #12 at the CLI layer (defense in depth alongside the
	// library-layer fix).
	if abs, err := filepath.Abs(cfg.root); err == nil {
		cfg.root = abs
	}

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(stderr, "cascade:", err)
		return mapError(err)
	}

	changedFiles, err := loadChangeSet(ctx, cfg, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "cascade:", err)
		return mapError(err)
	}

	affected, err := runPipeline(ctx, cfg, changedFiles)
	if err != nil {
		fmt.Fprintln(stderr, "cascade:", err)
		return mapError(err)
	}

	// Surface a diagnostic when changedFiles contained Go files but the
	// affected-set is empty — that's a suspicious zero-result (any .go
	// file change should produce at least the seed package itself).
	// Filter on .go suffix so docs-only PRs (legitimately empty) stay
	// silent. Doesn't change exit code; just adds a stderr breadcrumb.
	// Bug #12's silent-empty case would have surfaced in seconds with
	// this check.
	if len(affected) == 0 {
		nGoFiles := 0
		for _, f := range changedFiles {
			if strings.HasSuffix(f, ".go") {
				nGoFiles++
			}
		}
		if nGoFiles > 0 {
			fmt.Fprintf(stderr,
				"cascade: %d changed Go file(s) did not resolve to any package; check --root\n",
				nGoFiles)
		}
	}

	for _, path := range affected {
		fmt.Fprintln(stdout, path)
	}
	return exitSuccess
}

// parseFlags parses args using a fresh FlagSet wired to stderr per stdlib
// flag's default. Returns config + error. flag.ErrHelp is returned as-is so
// the caller can distinguish "user asked for help via -h" (exit 0) from
// "flag parse failed" (exit 1).
//
// --help is registered as a regular bool flag, so `cascade --help` parses
// without triggering flag.ErrHelp; the caller handles cfg.showHelp directly
// and routes helpText to stdout.
func parseFlags(args []string, stderr io.Writer) (*config, error) {
	fs := flag.NewFlagSet("cascade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), helpText)
	}

	cfg := &config{}
	var tagsRaw string
	fs.StringVar(&tagsRaw, "tags", "", "comma-separated build tags passed to 'go list -tags='")
	fs.StringVar(&cfg.base, "base", "", "base git ref (required unless --changed-files)")
	fs.StringVar(&cfg.head, "head", "HEAD", "head git ref")
	fs.StringVar(&cfg.changedFiles, "changed-files", "", "path to change-set file or '-' for stdin")
	fs.StringVar(&cfg.root, "root", ".", "working directory for go list and module root")
	fs.BoolVar(&cfg.showVersion, "version", false, "print version metadata and exit")
	fs.BoolVar(&cfg.showHelp, "help", false, "print usage and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", errFlagOrInput, err)
	}

	if fs.NArg() > 0 {
		return nil, fmt.Errorf("%w: unexpected positional argument %q", errFlagOrInput, fs.Arg(0))
	}

	if tagsRaw != "" {
		cfg.tags = strings.Split(tagsRaw, ",")
	}
	return cfg, nil
}

// validateConfig checks for missing required flags after a successful parse.
// --base is required unless --changed-files (file path or '-' for stdin) is
// supplied. --version and --help bypass validation in Run before reaching here.
func validateConfig(cfg *config) error {
	if cfg.changedFiles == "" && cfg.base == "" {
		return fmt.Errorf("%w: --base is required when --changed-files is not supplied", errFlagOrInput)
	}
	return nil
}

// loadChangeSet returns the list of changed file paths from one of three
// sources, in priority order:
//
//  1. --changed-files=<path>: read newline-separated entries from the file.
//  2. --changed-files=-: read newline-separated entries from stdin.
//  3. otherwise: invoke runGitDiff(ctx, base, head, root) and parse stdout.
//
// Lines are trimmed; empty lines after trim are skipped. Errors from the
// scanner or os.Open are wrapped under errFlagOrInput so they map to exit 1.
// git diff errors are typed (*GitDiffError) and map to exit 2.
func loadChangeSet(ctx context.Context, cfg *config, stdin io.Reader) ([]string, error) {
	switch cfg.changedFiles {
	case "":
		res := runGitDiff(ctx, cfg.base, cfg.head, cfg.root)
		if res.err != nil {
			// res.argv comes back from the seam itself, so the diagnostic
			// chain reflects the actual exec invocation rather than a
			// reconstruction (closes F-14 in 0014-go-quality-audit.md).
			return nil, classifyGitDiffError(res.err, res.argv, cfg.root, res.stderr, ctx)
		}
		return scanLines(res.stdout)
	case "-":
		return scanLines(stdin)
	default:
		f, err := os.Open(cfg.changedFiles) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("%w: opening --changed-files=%s: %w", errFlagOrInput, cfg.changedFiles, err)
		}
		defer f.Close() //nolint:errcheck // read-only file; close error is not actionable
		return scanLines(f)
	}
}

// scanLines reads r line-by-line, trims each line, and returns non-empty
// trimmed lines. Errors from the scanner are wrapped under errFlagOrInput.
func scanLines(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%w: scanning input: %w", errFlagOrInput, err)
	}
	return out, nil
}

// runPipeline composes golist.Run + depgraph.Build + changeset.Resolve +
// g.RevDepClosure and returns the affected-package list. Pure assembly; no
// side effects beyond what the four primitives produce. Structural
// verification of the M4 retro's "trio composes without adapter code" claim.
func runPipeline(ctx context.Context, cfg *config, changedFiles []string) ([]string, error) {
	pkgs, err := runGoListWrapper(ctx, cfg.tags, []string{"./..."}, golist.WithDir(cfg.root))
	if err != nil {
		return nil, err
	}
	g := depgraph.Build(pkgs)
	seeds := changeset.Resolve(changedFiles, pkgs, changeset.WithModuleRoot(cfg.root))
	return g.RevDepClosure(seeds), nil
}

// mapError maps any error from Run's pipeline to the process exit code per
// the M5 contract. Unmapped errors return exitInternal (4).
//
// Order of checks matters: cancellation is checked first because a cancelled
// pipeline may surface a downstream go-list / git-diff error too, and we
// want the cancellation exit code to win when both are present.
func mapError(err error) int {
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return exitCancelled
	}
	if errors.Is(err, ErrGitDiffFailed) {
		return exitGitFailed
	}
	if errors.Is(err, golist.ErrGoListFailed) ||
		errors.Is(err, golist.ErrGoNotFound) ||
		errors.Is(err, golist.ErrParseFailed) {
		return exitGoListFailed
	}
	if errors.Is(err, errFlagOrInput) {
		return exitFlagError
	}
	return exitInternal
}
