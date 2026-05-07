package golist

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturePath returns the path to a fixture under golist/testdata.
// Tests run with their cwd set to the package directory, so
// "testdata/<name>" is the conventional spelling.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

// readFixture returns the raw bytes of the named fixture, failing the
// test if it's unreadable.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(fixturePath(t, name))
	if err != nil {
		t.Fatalf("readFixture(%q): %v", name, err)
	}
	return b
}

func TestParseStream_SuccessFixtures(t *testing.T) {
	tests := []struct {
		name              string
		fixture           string
		wantPackageCount  int
		wantImportPaths   []string // expected ImportPaths in order
		wantContainsField func(*testing.T, []Package)
	}{
		{
			name:             "single-package",
			fixture:          "single-package.json",
			wantPackageCount: 1,
			wantImportPaths:  []string{"example.test/sample/pkga"},
			wantContainsField: func(t *testing.T, pkgs []Package) {
				p := pkgs[0]
				if p.Module == nil || p.Module.Path != "example.test/sample" || !p.Module.Main {
					t.Errorf("unexpected Module: %+v", p.Module)
				}
				if len(p.GoFiles) != 1 || p.GoFiles[0] != "a.go" {
					t.Errorf("unexpected GoFiles: %v", p.GoFiles)
				}
				if len(p.Imports) != 1 || p.Imports[0] != "fmt" {
					t.Errorf("unexpected Imports: %v", p.Imports)
				}
			},
		},
		{
			name:             "multi-package",
			fixture:          "multi-package.json",
			wantPackageCount: 3,
			wantImportPaths: []string{
				"example.test/sample/pkga",
				"example.test/sample/pkgb",
				"example.test/sample/pkgc",
			},
			wantContainsField: func(t *testing.T, pkgs []Package) {
				if len(pkgs[1].Imports) != 1 || pkgs[1].Imports[0] != "example.test/sample/pkga" {
					t.Errorf("pkgb Imports = %v, want [%s]", pkgs[1].Imports, "example.test/sample/pkga")
				}
				if len(pkgs[2].Imports) != 2 {
					t.Errorf("pkgc Imports len = %d, want 2", len(pkgs[2].Imports))
				}
			},
		},
		{
			name:             "with-tests",
			fixture:          "with-tests.json",
			wantPackageCount: 1,
			wantImportPaths:  []string{"example.test/sample/pkgc"},
			wantContainsField: func(t *testing.T, pkgs []Package) {
				p := pkgs[0]
				if len(p.TestGoFiles) != 1 || p.TestGoFiles[0] != "c_test.go" {
					t.Errorf("TestGoFiles = %v", p.TestGoFiles)
				}
				if len(p.XTestGoFiles) != 1 || p.XTestGoFiles[0] != "c_xtest.go" {
					t.Errorf("XTestGoFiles = %v", p.XTestGoFiles)
				}
				if len(p.TestImports) != 1 || p.TestImports[0] != "testing" {
					t.Errorf("TestImports = %v", p.TestImports)
				}
				if len(p.XTestImports) != 2 {
					t.Errorf("XTestImports len = %d, want 2", len(p.XTestImports))
				}
			},
		},
		{
			name:             "build-tag",
			fixture:          "build-tag.json",
			wantPackageCount: 1,
			wantImportPaths:  []string{"example.test/sample/pkgd"},
			wantContainsField: func(t *testing.T, pkgs []Package) {
				p := pkgs[0]
				if len(p.GoFiles) != 1 || p.GoFiles[0] != "pkgd_linux.go" {
					t.Errorf("GoFiles = %v", p.GoFiles)
				}
				if len(p.IgnoredGoFiles) != 1 || p.IgnoredGoFiles[0] != "pkgd_darwin.go" {
					t.Errorf("IgnoredGoFiles = %v", p.IgnoredGoFiles)
				}
			},
		},
		{
			name:             "stdlib-mixed",
			fixture:          "stdlib-mixed.json",
			wantPackageCount: 3,
			wantImportPaths:  []string{"fmt", "example.test/sample/pkga", "errors"},
			wantContainsField: func(t *testing.T, pkgs []Package) {
				if !pkgs[0].Standard || pkgs[0].Module != nil {
					t.Errorf("fmt: Standard=%v Module=%v", pkgs[0].Standard, pkgs[0].Module)
				}
				if pkgs[1].Standard || pkgs[1].Module == nil {
					t.Errorf("non-stdlib pkga: Standard=%v Module=%v", pkgs[1].Standard, pkgs[1].Module)
				}
				if !pkgs[2].Standard {
					t.Errorf("errors: Standard=%v, want true", pkgs[2].Standard)
				}
			},
		},
		{
			name:             "empty",
			fixture:          "empty.json",
			wantPackageCount: 0,
			wantImportPaths:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := readFixture(t, tc.fixture)
			pkgs, err := parseStream(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("parseStream(%s) returned error: %v", tc.fixture, err)
			}
			if len(pkgs) != tc.wantPackageCount {
				t.Errorf("got %d packages, want %d", len(pkgs), tc.wantPackageCount)
			}
			for i, want := range tc.wantImportPaths {
				if i >= len(pkgs) {
					break
				}
				if pkgs[i].ImportPath != want {
					t.Errorf("pkgs[%d].ImportPath = %q, want %q", i, pkgs[i].ImportPath, want)
				}
			}
			if tc.wantContainsField != nil && len(pkgs) > 0 {
				tc.wantContainsField(t, pkgs)
			}
		})
	}
}

func TestParseStream_ErrorFixtures(t *testing.T) {
	tests := []struct {
		name           string
		fixture        string
		wantOffsetMin  int64 // some positive offset (we don't pin exact bytes)
		wantPayloadSub string
		wantCauseAs    func(error) bool
	}{
		{
			name:           "truncated mid-record",
			fixture:        "truncated.json",
			wantOffsetMin:  100, // first record + start of second
			wantPayloadSub: "example.test/",
			// Cause depends on where the cut lands in the JSON token
			// stream (could be io.ErrUnexpectedEOF or a SyntaxError).
			// Either is a valid parse failure; we only require non-nil.
			wantCauseAs: nil,
		},
		{
			name:           "malformed",
			fixture:        "malformed.json",
			wantOffsetMin:  0,
			wantPayloadSub: "this is not valid",
			wantCauseAs:    nil, // any non-nil cause is acceptable
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := readFixture(t, tc.fixture)
			pkgs, err := parseStream(bytes.NewReader(data))
			if err == nil {
				t.Fatalf("parseStream(%s) returned no error; got %d pkgs", tc.fixture, len(pkgs))
			}
			if pkgs != nil {
				t.Errorf("parseStream returned non-nil packages on error: %v", pkgs)
			}
			if !errors.Is(err, ErrParseFailed) {
				t.Errorf("errors.Is(err, ErrParseFailed) = false; got %v", err)
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("errors.As did not extract *ParseError from %v", err)
			}
			if pe.Offset < tc.wantOffsetMin {
				t.Errorf("Offset = %d, want >= %d", pe.Offset, tc.wantOffsetMin)
			}
			if !strings.Contains(pe.Payload, tc.wantPayloadSub) {
				t.Errorf("Payload = %q, missing substring %q", pe.Payload, tc.wantPayloadSub)
			}
			if pe.Cause == nil {
				t.Errorf("Cause is nil; want non-nil json error")
			}
			if tc.wantCauseAs != nil && !tc.wantCauseAs(err) {
				t.Errorf("Cause didn't match expected category; got %v", pe.Cause)
			}
		})
	}
}

func TestParseStream_PayloadCappedAtMax(t *testing.T) {
	// Build an input that contains a malformed record after a long lead-in.
	// We expect Payload to be capped at ParseErrorMaxPayload bytes.
	var buf bytes.Buffer
	buf.WriteString(`{"ImportPath":"good"}` + "\n")
	// A long stretch of garbage to ensure capture cap is exercised
	buf.WriteString(strings.Repeat("X", ParseErrorMaxPayload*2))

	pkgs, err := parseStream(&buf)
	if err == nil {
		t.Fatalf("expected error, got %d pkgs", len(pkgs))
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("not a *ParseError: %v", err)
	}
	if len(pe.Payload) > ParseErrorMaxPayload {
		t.Errorf("Payload len = %d, want <= %d", len(pe.Payload), ParseErrorMaxPayload)
	}
}
