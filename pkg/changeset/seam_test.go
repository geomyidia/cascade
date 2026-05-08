package changeset

import (
	"errors"
	"testing"

	"github.com/geomyidia/cascade/pkg/golist"
)

// Parallel-unsafe: tests in this file mutate the package-level getCwd
// seam. Do not call t.Parallel() in any test that exercises it. See S-2
// in docs/dev/0014-go-quality-audit.md.

// withCwdSeam swaps the package-level getCwd for the duration of a test,
// restoring the default afterward. Mirrors pkg/golist's withSeam helper.
func withCwdSeam(t *testing.T, fn func() (string, error)) {
	t.Helper()
	saved := getCwd
	t.Cleanup(func() { getCwd = saved })
	getCwd = fn
}

// TestResolve_DefaultUsesGetCwd covers the os.Getwd-success branch of the
// fallback: when WithModuleRoot is not supplied, Resolve calls getCwd and
// uses its return value as the module root. The seam returns a stub cwd
// so the test outcome doesn't depend on the actual working directory.
func TestResolve_DefaultUsesGetCwd(t *testing.T) {
	withCwdSeam(t, func() (string, error) {
		return "/stub/cwd", nil
	})

	// Pkg's Dir matches the stub cwd's child; relative file should resolve.
	pkgs := []golist.Package{
		{ImportPath: "ex/pkga", Dir: "/stub/cwd/pkga"},
	}
	got := Resolve(
		[]string{"pkga/a.go"}, // relative
		pkgs,
		// no WithModuleRoot — triggers getCwd fallback
	)
	want := []string{"ex/pkga"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Resolve(...) = %v, want %v (Getwd fallback should resolve relative paths)", got, want)
	}
}

// TestResolve_GetCwdErrorTolerated covers the os.Getwd-error branch of the
// fallback: when getCwd returns an error, moduleRoot stays empty and
// relative paths silently fail to resolve (no panic, no error from
// Resolve). Same observable outcome as passing WithModuleRoot("").
func TestResolve_GetCwdErrorTolerated(t *testing.T) {
	withCwdSeam(t, func() (string, error) {
		return "", errors.New("getwd failed (synthetic)")
	})

	pkgs := []golist.Package{
		{ImportPath: "ex/pkga", Dir: "/anywhere/pkga"},
	}
	got := Resolve(
		[]string{"pkga/a.go"}, // relative; can't resolve without moduleRoot
		pkgs,
	)
	if got != nil {
		t.Errorf("Resolve(...) = %v, want nil (Getwd error must silently fail relative resolution)", got)
	}

	// Absolute paths still work even when getCwd fails — moduleRoot is
	// only consulted for relative paths.
	got = Resolve(
		[]string{"/anywhere/pkga/a.go"},
		pkgs,
	)
	want := []string{"ex/pkga"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("Resolve(...) = %v, want %v (absolute paths should resolve regardless of Getwd)", got, want)
	}
}
