package changeset

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geomyidia/cascade/pkg/golist"
)

// Option configures a Resolve call. Apply via Resolve's variadic parameter.
// The only option in v0.x is WithModuleRoot.
type Option func(*config)

// config holds the resolved option state for a single Resolve call.
// Unexported: callers compose configuration only via With* constructors.
type config struct {
	moduleRoot    string
	moduleRootSet bool
}

// WithModuleRoot sets the module root used to resolve relative entries in
// changedFiles. Both absolute and relative paths are accepted; Resolve calls
// filepath.Abs on the supplied value before use, so a relative path is
// resolved against the process cwd at call time. (Closes bug #12: a literal
// "." or any other relative path now resolves correctly rather than silently
// failing the absolute-keyed dirMap lookup.)
//
// If WithModuleRoot is not supplied, Resolve falls back to os.Getwd at call
// time. Tests should pass WithModuleRoot with an absolute path so test
// outcomes don't depend on the working directory and the io is bypassed.
//
// An empty argument and "." both resolve to the cwd at call time (because
// filepath.Abs("") and filepath.Abs(".") are equivalent). The distinction
// between "explicitly empty" and "not set" remains in the implementation
// (moduleRootSet flag) so tests that need to drive the os.Getwd fallback
// branch can still do so by omitting WithModuleRoot, but the user-visible
// behaviour of "." vs "" is now identical and correct.
func WithModuleRoot(dir string) Option {
	return func(c *config) {
		c.moduleRoot = dir
		c.moduleRootSet = true
	}
}

// getCwd is the function-variable seam over os.Getwd. Production builds use
// os.Getwd; tests can replace it via the seam helper to drive the fallback's
// success and error branches without depending on the actual test cwd. The
// pattern matches pkg/golist's runGoList seam.
var getCwd = os.Getwd

// Resolve maps changed file paths to the import paths of the packages whose
// Go files they are. Returns the set of distinct import paths sorted
// lexicographically for deterministic CI behaviour.
//
// changedFiles are paths to Go files that have been modified, added,
// renamed, or removed (typically `git diff --name-only` output, one entry
// per line). Paths may be relative (resolved against moduleRoot) or
// absolute; filepath.Clean is applied before lookup.
//
// pkgs is the package list returned by golist.Run; each Package's Dir field
// (absolute path) drives the lookup.
//
// opts configure the call. The only option in v0.x is WithModuleRoot; if
// not supplied, Resolve falls back to os.Getwd. If os.Getwd itself returns
// an error (rare), moduleRoot stays empty and relative paths silently fail
// to resolve — same outcome as passing WithModuleRoot("").
//
// Mapping rules:
//   - A file ending in ".go" whose parent directory exactly matches some
//     package's Dir maps to that package's ImportPath. The parent-directory
//     match works regardless of whether the file currently exists on disk,
//     so removed Go files map correctly.
//   - _test.go files map to the same package as non-test files in the same
//     directory. Internal-test (package foo) and external-test (package
//     foo_test) both yield foo's ImportPath.
//   - Files in a package's IgnoredGoFiles (build-tag-excluded) map to the
//     package — the rule is parent-dir match; file-list membership is
//     irrelevant to the lookup.
//   - Non-Go files (no ".go" extension) are silently skipped.
//   - Files in subdirectories of a package's Dir (e.g. testdata/) are
//     skipped — the parent directory does not match any package's Dir.
//   - Files outside any package's Dir are silently skipped.
//
// Error returns: none. Every input shape is handled gracefully — empty
// slices, nil pkgs, blank moduleRoot, paths that don't resolve to any
// package, removed files, and os.Getwd failures all produce sensible
// output (typically an empty result).
//
// Symlinks are not followed. If changedFiles contains a path that's a
// symlink and pkg.Dir contains the resolved real path (or vice versa), the
// lexical comparison may miss. Callers that need symlink-aware mapping
// should canonicalise before calling Resolve.
//
// Determinism: identical inputs yield identical output across runs.
// Duplicates are deduplicated.
//
// Complexity: O(P + F) where P is len(pkgs) and F is len(changedFiles),
// plus O(k log k) for the final sort on the result-set size k.
func Resolve(changedFiles []string, pkgs []golist.Package, opts ...Option) []string {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if !cfg.moduleRootSet {
		if cwd, err := getCwd(); err == nil {
			cfg.moduleRoot = cwd
		}
	}

	// Absolutize moduleRoot so the parent-dir comparison against pkg.Dir
	// (which golist documents as absolute) succeeds even when the caller
	// supplies a relative path. filepath.Abs("") and filepath.Abs(".")
	// both resolve to the cwd, so this also handles the empty-string case
	// gracefully. On error (rare; documented in os.Getwd), leave as-is —
	// the lookup will fail consistently with the unfixed relative-input
	// case rather than panic. Closes bug #12.
	if abs, err := filepath.Abs(cfg.moduleRoot); err == nil {
		cfg.moduleRoot = abs
	}

	if len(pkgs) == 0 || len(changedFiles) == 0 {
		return nil
	}

	// Build dir → ImportPath map from pkgs (one entry per package). Skip
	// entries with empty Dir or ImportPath defensively — go list -deps -json
	// shouldn't emit them, but synthetic test inputs sometimes do, and a
	// silent skip is friendlier than a panic.
	dirMap := make(map[string]string, len(pkgs))
	for _, p := range pkgs {
		if p.Dir == "" || p.ImportPath == "" {
			continue
		}
		dirMap[p.Dir] = p.ImportPath
	}

	// Collect unique import paths via a set; convert to a sorted slice once.
	seen := make(map[string]struct{})
	for _, file := range changedFiles {
		if file == "" {
			continue
		}
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		var abs string
		if filepath.IsAbs(file) {
			abs = filepath.Clean(file)
		} else {
			abs = filepath.Clean(filepath.Join(cfg.moduleRoot, file))
		}
		parent := filepath.Dir(abs)
		if importPath, ok := dirMap[parent]; ok {
			seen[importPath] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for ip := range seen {
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}
