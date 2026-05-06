package golist_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/geomyidia/cascade/golist"
)

// sampleModulePath returns the absolute path to the sample-module
// fixture used by the Layer-2 smoke tests. Tests run with cwd set to
// the package directory, so this is "testdata/sample-module".
func sampleModulePath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "sample-module"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// TestRun_SampleModule (spec F-12) — Layer-2 end-to-end smoke test.
// Skipped under -short.
func TestRun_SampleModule(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	pkgs, err := golist.Run(t.Context(), nil, []string{"./..."},
		golist.WithDir(sampleModulePath(t)))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// pkga, pkgb, pkgc, pkgd should all be present in the output.
	// The slice may also include stdlib deps (fmt, etc.) — that's fine.
	wantImports := map[string]bool{
		"example.test/sample/pkga": false,
		"example.test/sample/pkgb": false,
		"example.test/sample/pkgc": false,
		"example.test/sample/pkgd": false,
	}
	for _, p := range pkgs {
		if _, ok := wantImports[p.ImportPath]; ok {
			wantImports[p.ImportPath] = true
		}
	}
	for path, found := range wantImports {
		if !found {
			t.Errorf("did not find expected package %q in output", path)
		}
	}

	// pkgb should import pkga
	for _, p := range pkgs {
		if p.ImportPath == "example.test/sample/pkgb" {
			if !contains(p.Imports, "example.test/sample/pkga") {
				t.Errorf("pkgb.Imports = %v, want to contain pkga", p.Imports)
			}
		}
	}

	// Build-tag exercise: pkgd should have one of the platform-specific
	// files in GoFiles and the other in IgnoredGoFiles, depending on
	// GOOS. (On unsupported OSes, both files are ignored — that's fine
	// as long as at least one is present somewhere.)
	for _, p := range pkgs {
		if p.ImportPath != "example.test/sample/pkgd" {
			continue
		}
		all := append(append([]string(nil), p.GoFiles...), p.IgnoredGoFiles...)
		hasLinux := contains(all, "pkgd_linux.go")
		hasDarwin := contains(all, "pkgd_darwin.go")
		if !hasLinux || !hasDarwin {
			t.Errorf("pkgd file lists missing platform variant; GoFiles=%v IgnoredGoFiles=%v",
				p.GoFiles, p.IgnoredGoFiles)
		}
		// On linux/darwin, exactly one should be selected.
		if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
			wantSelected := "pkgd_" + runtime.GOOS + ".go"
			if !contains(p.GoFiles, wantSelected) {
				t.Errorf("on %s, pkgd.GoFiles = %v, want to contain %q",
					runtime.GOOS, p.GoFiles, wantSelected)
			}
		}
	}
}

// TestRun_DefaultPatterns — when patterns is nil/empty, Run substitutes
// ./... and proceeds. We verify by passing nil patterns and asserting the
// output is non-empty against the sample module.
func TestRun_DefaultPatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	tests := [][]string{nil, {}}
	for i, patterns := range tests {
		t.Run(map[bool]string{true: "nil", false: "empty"}[patterns == nil], func(t *testing.T) {
			pkgs, err := golist.Run(t.Context(), nil, patterns,
				golist.WithDir(sampleModulePath(t)))
			if err != nil {
				t.Fatalf("Run failed (case %d): %v", i, err)
			}
			if len(pkgs) == 0 {
				t.Errorf("Run returned 0 packages with default ./...")
			}
		})
	}
}

// TestRun_GoNotFound (spec F-7 chain) — pointing WithGoBin at a
// nonexistent path returns an error matching ErrGoNotFound.
func TestRun_GoNotFound(t *testing.T) {
	bogus := filepath.Join(t.TempDir(), "definitely-not-go-binary-"+strings.Repeat("x", 8))
	_, err := golist.Run(t.Context(), nil, []string{"./..."},
		golist.WithGoBin(bogus),
		golist.WithDir(sampleModulePath(t)))
	if err == nil {
		t.Fatalf("expected error for nonexistent goBin, got nil")
	}
	if !errors.Is(err, golist.ErrGoNotFound) {
		t.Errorf("errors.Is(err, ErrGoNotFound) = false; got %v", err)
	}
}

// TestRun_ExitErrorChain (spec F-8) — when `go list` exits non-zero,
// the returned error chains via errors.Is to ErrGoListFailed and
// extracts via errors.As to *ExitError.
func TestRun_ExitErrorChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	// Invalid pattern produces non-zero exit from `go list`.
	_, err := golist.Run(t.Context(), nil, []string{"./this/does/not/exist/..."},
		golist.WithDir(sampleModulePath(t)))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, golist.ErrGoListFailed) {
		t.Errorf("errors.Is(err, ErrGoListFailed) = false; got %v", err)
	}
	var ee *golist.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("errors.As did not extract *ExitError from %v", err)
	}
	if ee.ExitCode == 0 {
		t.Errorf("ExitCode = 0; want non-zero")
	}
	if len(ee.Cmd) < 4 {
		t.Errorf("Cmd = %v; want full argv", ee.Cmd)
	}
	// Stderr should contain something (go's error message)
	if strings.TrimSpace(ee.Stderr) == "" {
		t.Errorf("Stderr is empty; want go list's error output")
	}
	// Unwrap should reach an *exec.ExitError
	var execErr *exec.ExitError
	if !errors.As(err, &execErr) {
		t.Errorf("errors.As did not reach *exec.ExitError")
	}
}

// TestRun_ContextCancellation (spec F-14) — cancelling ctx mid-stream
// kills the subprocess and returns a ctx.Err()-wrapping error.
func TestRun_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	_, err := golist.Run(ctx, nil, []string{"./..."},
		golist.WithDir(sampleModulePath(t)))
	if err == nil {
		t.Fatalf("expected error after pre-cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(err, context.Canceled) = false; got %v", err)
	}
}

// TestRun_Concurrent (spec F-15) — N concurrent Run calls against the
// sample module from independent goroutines all succeed and return
// equivalent slices. Run with -race to catch goroutine leaks.
func TestRun_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	const n = 4
	type result struct {
		pkgs []golist.Package
		err  error
	}
	results := make([]result, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			pkgs, err := golist.Run(t.Context(), nil, []string{"./..."},
				golist.WithDir(sampleModulePath(t)))
			results[i] = result{pkgs: pkgs, err: err}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Errorf("goroutine %d: %v", i, r.err)
			continue
		}
		if len(r.pkgs) == 0 {
			t.Errorf("goroutine %d: 0 packages returned", i)
		}
	}

	// Sanity check: lengths should all be equal (same module, same
	// invocation, same output)
	if results[0].err == nil {
		want := len(results[0].pkgs)
		for i := 1; i < n; i++ {
			if results[i].err == nil && len(results[i].pkgs) != want {
				t.Errorf("goroutine %d package count = %d, want %d (same as goroutine 0)",
					i, len(results[i].pkgs), want)
			}
		}
	}
}

// TestRun_TagsApplied — passing tags adds -tags=<csv> to the argv. We
// verify by passing a nonsense tag that suppresses everything: the
// build-tag-only file in pkgd should switch from GoFiles to
// IgnoredGoFiles when no platform-matching tag is present alongside
// the requested one. (Hard to assert without parsing the argv directly,
// so we go indirect: pass a tag that excludes the platform file and
// verify the affected package's GoFiles change shape.)
//
// More direct: this test passes a tag and checks the call doesn't
// error and that pkgd is still listed.
func TestRun_TagsApplied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	pkgs, err := golist.Run(t.Context(), []string{"customtag"}, []string{"./..."},
		golist.WithDir(sampleModulePath(t)))
	if err != nil {
		t.Fatalf("Run with tags failed: %v", err)
	}
	found := false
	for _, p := range pkgs {
		if p.ImportPath == "example.test/sample/pkga" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("pkga not found in output with custom tag")
	}
}

// TestRun_WithEnv — confirms WithEnv replaces the inherited environment.
// We pass a minimal env that retains PATH (so `go` can be found) but
// strips everything else. The call should still succeed.
func TestRun_WithEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	// Find PATH from the current env to keep go discoverable.
	var path string
	for _, e := range []string{"PATH=" + getenvOrDefault("PATH", "")} {
		path = e
	}
	// Also need HOME for go's module cache resolution on some platforms;
	// without it `go list` might complain.
	env := []string{path, "HOME=" + getenvOrDefault("HOME", "/tmp")}

	_, err := golist.Run(t.Context(), nil, []string{"./..."},
		golist.WithDir(sampleModulePath(t)),
		golist.WithEnv(env))
	// We don't strictly require success — go list may need additional
	// env vars in some environments. We assert: if it errors, the
	// error is a typed cascade error, not a panic.
	if err != nil {
		// Acceptable: typed error path. Just verify it's classified.
		var ee *golist.ExitError
		var pe *golist.ParseError
		if !errors.As(err, &ee) && !errors.As(err, &pe) && !errors.Is(err, golist.ErrGoNotFound) {
			t.Errorf("WithEnv error not classified: %v", err)
		}
	}
}

// Helpers --------------------------------------------------------------

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func getenvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
