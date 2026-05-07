package changeset_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/geomyidia/cascade/pkg/changeset"
	"github.com/geomyidia/cascade/pkg/golist"
)

// defaultPkgs is a 3-package fixture used by most StandardCases rows. Dir
// values are POSIX-style absolute paths; tests run on Linux/macOS.
func defaultPkgs() []golist.Package {
	return []golist.Package{
		{ImportPath: "ex/pkga", Dir: "/m/pkga"},
		{ImportPath: "ex/pkgb", Dir: "/m/pkgb"},
		{ImportPath: "ex/pkgc", Dir: "/m/pkgc"},
	}
}

// TestResolve_StandardCases (F-3, plus F-6/F-7/F-8/F-9 as named subtests
// per the spec's verify regexes) walks 16 synthetic mapping topologies that
// together cover every branch in Resolve plus the spec's contractual rules.
func TestResolve_StandardCases(t *testing.T) {
	tests := []struct {
		name         string
		changedFiles []string
		pkgs         []golist.Package
		moduleRoot   string
		want         []string
	}{
		{
			name:         "empty_changedFiles",
			changedFiles: nil,
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         nil,
		},
		{
			name:         "nil_pkgs",
			changedFiles: []string{"pkga/a.go"},
			pkgs:         nil,
			moduleRoot:   "/m",
			want:         nil,
		},
		{
			name:         "single_Go_file_in_pkga",
			changedFiles: []string{"pkga/a.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "two_Go_files_in_pkga_deduped",
			changedFiles: []string{"pkga/a.go", "pkga/b.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "files_in_pkga_and_pkgb_sorted",
			changedFiles: []string{"pkgb/b.go", "pkga/a.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga", "ex/pkgb"},
		},
		{
			name:         "_test.go_in_pkga",
			changedFiles: []string{"pkga/a_test.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "xtest_dot_go_in_pkga",
			changedFiles: []string{"pkga/a_x_test.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "mixed_go_and_non_go",
			changedFiles: []string{"pkga/a.go", "README.md", "pkgb/data.json"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "Go_file_outside_any_package",
			changedFiles: []string{"cmd/cascade/main.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         nil,
		},
		{
			name:         "Go_file_in_subdirectory_of_pkg_dir",
			changedFiles: []string{"pkga/testdata/foo.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         nil,
		},
		{
			name:         "removed_Go_file_in_pkga",
			changedFiles: []string{"pkga/deleted.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "relative_path_resolved_against_moduleRoot",
			changedFiles: []string{"pkga/a.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "absolute_path_used_directly",
			changedFiles: []string{"/m/pkgb/b.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/elsewhere", // ignored when path is absolute
			want:         []string{"ex/pkgb"},
		},
		{
			name:         "path_with_dot_dot_components_cleaned",
			changedFiles: []string{"pkga/../pkgb/b.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkgb"},
		},
		{
			name:         "duplicate_entries_in_changedFiles",
			changedFiles: []string{"pkga/a.go", "pkga/a.go", "pkga/a.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "unsorted_entries_yield_sorted_output",
			changedFiles: []string{"pkgc/c.go", "pkga/a.go", "pkgb/b.go"},
			pkgs:         defaultPkgs(),
			moduleRoot:   "/m",
			want:         []string{"ex/pkga", "ex/pkgb", "ex/pkgc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeset.Resolve(tt.changedFiles, tt.pkgs,
				changeset.WithModuleRoot(tt.moduleRoot))
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("Resolve(%v, ..., WithModuleRoot(%q)) = %v, want %v",
					tt.changedFiles, tt.moduleRoot, got, tt.want)
			}
			if !sort.StringsAreSorted(got) {
				t.Errorf("Resolve(...) = %v is not sorted lexicographically", got)
			}
		})
	}
}

// TestResolve_HandTraceable (F-4) is the spec's exit-criterion example —
// a 4-package graph with hand-derived expected output. Documents the
// mapping semantics for future maintainers.
//
//	Packages:
//	  ex/pkga    Dir=/m/pkga
//	  ex/pkgb    Dir=/m/pkgb    (xtest file b_x_test.go in pkgb's Dir)
//	  ex/pkgc    Dir=/m/pkgc
//	  ex/pkgd    Dir=/m/pkgd    (build-tag-excluded d_linux.go in pkgd's Dir)
func TestResolve_HandTraceable(t *testing.T) {
	pkgs := []golist.Package{
		{ImportPath: "ex/pkga", Dir: "/m/pkga"},
		{ImportPath: "ex/pkgb", Dir: "/m/pkgb"},
		{ImportPath: "ex/pkgc", Dir: "/m/pkgc"},
		{ImportPath: "ex/pkgd", Dir: "/m/pkgd"},
	}
	tests := []struct {
		name         string
		changedFiles []string
		want         []string
	}{
		{"pkga_a_go", []string{"pkga/a.go"}, []string{"ex/pkga"}},
		{"pkga_a_test_go", []string{"pkga/a_test.go"}, []string{"ex/pkga"}},
		{"pkgb_xtest_go", []string{"pkgb/b_x_test.go"}, []string{"ex/pkgb"}},
		{"pkga_and_pkgc", []string{"pkga/a.go", "pkgc/c.go"}, []string{"ex/pkga", "ex/pkgc"}},
		{"subdirectory_of_pkga_skipped", []string{"pkga/sub/x.go"}, nil},
		{"non_go_file_skipped", []string{"README.md"}, nil},
		{"empty_changeset", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeset.Resolve(tt.changedFiles, pkgs,
				changeset.WithModuleRoot("/m"))
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("Resolve(%v, pkgs, WithModuleRoot(/m)) = %v, want %v",
					tt.changedFiles, got, tt.want)
			}
		})
	}
}

// TestResolve_PathNormalisation (F-5) covers OS-portable path handling:
// redundant separators, .. traversal, ./ prefixes, and absolute-vs-relative
// paths. filepath.Clean must normalise all of them before lookup.
func TestResolve_PathNormalisation(t *testing.T) {
	pkgs := defaultPkgs()
	tests := []struct {
		name         string
		changedFiles []string
		moduleRoot   string
		want         []string
	}{
		{
			name:         "redundant_separators",
			changedFiles: []string{"pkga//a.go", "pkgb///b.go"},
			moduleRoot:   "/m",
			want:         []string{"ex/pkga", "ex/pkgb"},
		},
		{
			name:         "leading_dot_slash",
			changedFiles: []string{"./pkga/a.go"},
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "interior_dot_slash",
			changedFiles: []string{"pkga/./a.go"},
			moduleRoot:   "/m",
			want:         []string{"ex/pkga"},
		},
		{
			name:         "dot_dot_traversal",
			changedFiles: []string{"pkga/../pkgb/b.go"},
			moduleRoot:   "/m",
			want:         []string{"ex/pkgb"},
		},
		{
			name:         "absolute_path_normalised",
			changedFiles: []string{"/m//pkga//a.go"},
			moduleRoot:   "/elsewhere",
			want:         []string{"ex/pkga"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeset.Resolve(tt.changedFiles, pkgs,
				changeset.WithModuleRoot(tt.moduleRoot))
			if !stringSlicesEqual(got, tt.want) {
				t.Errorf("Resolve(%v, ..., WithModuleRoot(%q)) = %v, want %v",
					tt.changedFiles, tt.moduleRoot, got, tt.want)
			}
		})
	}
}

// TestResolve_IgnoredGoFiles_MappedRegardless verifies the Q5 decision: a
// file in a package's Dir maps to that package even when it's not in any
// of GoFiles / TestGoFiles / XTestGoFiles (e.g. an IgnoredGoFiles entry,
// or an entirely synthetic name). The lookup is parent-dir match;
// file-list membership is irrelevant to the algorithm.
func TestResolve_IgnoredGoFiles_MappedRegardless(t *testing.T) {
	pkgs := []golist.Package{
		{
			ImportPath:     "ex/pkga",
			Dir:            "/m/pkga",
			GoFiles:        []string{"a.go"},
			IgnoredGoFiles: []string{"a_linux.go"},
		},
	}
	// Both a.go (in GoFiles) and a_linux.go (in IgnoredGoFiles) live in the
	// same Dir; both should map to ex/pkga regardless of file-list membership.
	// Even a_speculative.go (in neither file list) maps because the lookup
	// only considers parent-dir.
	got := changeset.Resolve(
		[]string{"pkga/a.go", "pkga/a_linux.go", "pkga/a_speculative.go"},
		pkgs,
		changeset.WithModuleRoot("/m"),
	)
	want := []string{"ex/pkga"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("Resolve = %v, want %v (file-list membership must be irrelevant)", got, want)
	}
}

// TestResolve_PackagesWithEmptyFieldsSkipped covers the defensive branches
// in the dirMap-build loop: packages with empty Dir or empty ImportPath
// are silently skipped (synthetic test inputs sometimes produce these;
// production go list output shouldn't).
func TestResolve_PackagesWithEmptyFieldsSkipped(t *testing.T) {
	pkgs := []golist.Package{
		{ImportPath: "", Dir: "/m/empty-import-path"}, // skipped
		{ImportPath: "ex/no-dir"},                     // skipped (empty Dir)
		{ImportPath: "ex/pkga", Dir: "/m/pkga"},       // kept
	}
	got := changeset.Resolve(
		[]string{"pkga/a.go", "empty-import-path/x.go", "no-dir/x.go"},
		pkgs,
		changeset.WithModuleRoot("/m"),
	)
	want := []string{"ex/pkga"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("Resolve = %v, want %v (packages with empty Dir/ImportPath must be skipped)", got, want)
	}
}

// TestResolve_EmptyFilePathSkipped covers the defensive `if file == ""`
// branch inside the changedFiles loop.
func TestResolve_EmptyFilePathSkipped(t *testing.T) {
	got := changeset.Resolve(
		[]string{"", "pkga/a.go", ""},
		defaultPkgs(),
		changeset.WithModuleRoot("/m"),
	)
	want := []string{"ex/pkga"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("Resolve = %v, want %v (empty file paths must be skipped)", got, want)
	}
}

// TestResolve_RelativeModuleRoot_AbsolutizedInternally is the regression test
// for bug #12. Pre-fix, passing a relative moduleRoot (notably ".") with
// relative changedFiles produced an empty result silently because filepath.
// Join(".", "rel/path") returns relative output, which can't match the
// absolute keys built from golist.Package.Dir. Post-fix, Resolve absolutizes
// moduleRoot via filepath.Abs before the dirMap lookup, so relative
// moduleRoots resolve correctly against the process cwd.
//
// The test sets up packages with Dir values rooted at the test's actual cwd
// (via os.Getwd) so the post-absolutize lookup succeeds with predictable
// values. Pre-fix this test fails (empty result); post-fix it passes.
func TestResolve_RelativeModuleRoot_AbsolutizedInternally(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	pkgs := []golist.Package{
		// Dir values must be absolute (golist contract); construct them
		// rooted at the test cwd so absolutize-of-"." resolves to the same
		// prefix and the parent-dir lookup succeeds.
		{ImportPath: "ex/pkga", Dir: filepath.Join(cwd, "pkga")},
		{ImportPath: "ex/pkgb", Dir: filepath.Join(cwd, "pkgb")},
	}

	tests := []struct {
		name       string
		moduleRoot string
	}{
		// Bug #12's headline case: literal "." (the CLI flag default).
		{name: "dot", moduleRoot: "."},
		// Empty string: filepath.Abs("") also resolves to cwd; same fix.
		{name: "empty", moduleRoot: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := changeset.Resolve(
				[]string{"pkga/a.go", "pkgb/b.go"},
				pkgs,
				changeset.WithModuleRoot(tt.moduleRoot),
			)
			want := []string{"ex/pkga", "ex/pkgb"}
			if !stringSlicesEqual(got, want) {
				t.Errorf("Resolve(...WithModuleRoot(%q)) = %v, want %v\n"+
					"(pre-fix empty output would indicate bug #12 regressed)",
					tt.moduleRoot, got, want)
			}
		})
	}
}
