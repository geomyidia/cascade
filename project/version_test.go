package project

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// withMetadata sets the package-level build vars for the duration of the
// test and restores them afterward. Tests use this to exercise both the
// empty-default path and the populated-via-ldflags path without relying on
// actual link-time injection.
func withMetadata(t *testing.T, version, commit, branch, summary, date string, body func()) {
	t.Helper()
	saveVersion, saveCommit, saveBranch, saveSummary, saveDate :=
		Version, GitCommit, GitBranch, GitSummary, BuildDate
	t.Cleanup(func() {
		Version, GitCommit, GitBranch, GitSummary, BuildDate =
			saveVersion, saveCommit, saveBranch, saveSummary, saveDate
	})
	Version, GitCommit, GitBranch, GitSummary, BuildDate =
		version, commit, branch, summary, date
	body()
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"empty returns N/A", "", "N/A"},
		{"populated returns value", "0.1.0", "0.1.0"},
		{"populated with prerelease", "0.1.0-rc1", "0.1.0-rc1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMetadata(t, tc.version, "", "", "", "", func() {
				if got := VersionString(); got != tc.want {
					t.Errorf("VersionString() = %q, want %q", got, tc.want)
				}
			})
		})
	}
}

func TestBuildString(t *testing.T) {
	tests := []struct {
		name   string
		commit string
		branch string
		date   string
		want   string
	}{
		{"empty commit returns N/A", "", "main", "2026-05-06T18:30:00Z", "N/A"},
		{"populated returns formatted", "abc1234", "main", "2026-05-06T18:30:00Z", "main@abc1234, 2026-05-06T18:30:00Z"},
		{"empty branch still formats", "abc1234", "", "2026-05-06T18:30:00Z", "@abc1234, 2026-05-06T18:30:00Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMetadata(t, "", tc.commit, tc.branch, "", tc.date, func() {
				if got := BuildString(); got != tc.want {
					t.Errorf("BuildString() = %q, want %q", got, tc.want)
				}
			})
		})
	}
}

func TestPrintVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		branch  string
		date    string
		// Each entry is a substring that must appear in the output.
		mustContain []string
	}{
		{
			name:        "all empty",
			version:     "",
			commit:      "",
			branch:      "",
			date:        "",
			mustContain: []string{"cascade version: N/A", "Build: N/A", "Go version: " + runtime.Version()},
		},
		{
			name:    "all populated",
			version: "0.1.0",
			commit:  "abc1234",
			branch:  "main",
			date:    "2026-05-06T18:30:00Z",
			mustContain: []string{
				"cascade version: 0.1.0",
				"Build: main@abc1234, 2026-05-06T18:30:00Z",
				"Go version: " + runtime.Version(),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMetadata(t, tc.version, tc.commit, tc.branch, "", tc.date, func() {
				var buf bytes.Buffer
				PrintVersions(&buf)
				out := buf.String()
				for _, want := range tc.mustContain {
					if !strings.Contains(out, want) {
						t.Errorf("PrintVersions output missing %q\nfull output:\n%s", want, out)
					}
				}
			})
		})
	}
}
