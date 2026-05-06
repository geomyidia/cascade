package project

import (
	"bytes"
	"runtime"
	"runtime/debug"
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

func TestLoadDefaults(t *testing.T) {
	embedded := strings.TrimSpace(versionFile)
	if embedded == "" {
		t.Fatalf("embedded VERSION file is empty; this should not happen in a checked-out repo")
	}

	tests := []struct {
		name              string
		startVersion      string
		startCommit       string
		startBranch       string
		startDate         string
		wantVersionPrefix string // prefix-match (Version may be the embedded value)
		wantCommitEmpty   bool   // whether GitCommit should remain empty after loadDefaults
		wantDateEmpty     bool
	}{
		{
			name:              "embed-fills-empty-Version",
			startVersion:      "",
			startCommit:       "abcdef0", // non-empty so ReadBuildInfo doesn't run
			startBranch:       "",
			startDate:         "2026-01-01T00:00:00Z", // non-empty so ReadBuildInfo doesn't run
			wantVersionPrefix: embedded,
			wantCommitEmpty:   false,
			wantDateEmpty:     false,
		},
		{
			name:              "ldflags-Version-takes-precedence",
			startVersion:      "9.9.9-fromldflags",
			startCommit:       "abcdef0",
			startBranch:       "",
			startDate:         "2026-01-01T00:00:00Z",
			wantVersionPrefix: "9.9.9-fromldflags",
			wantCommitEmpty:   false,
			wantDateEmpty:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMetadata(t, tc.startVersion, tc.startCommit, tc.startBranch, "", tc.startDate, func() {
				loadDefaults()
				if Version != tc.wantVersionPrefix {
					t.Errorf("Version = %q, want %q", Version, tc.wantVersionPrefix)
				}
				if tc.wantCommitEmpty && GitCommit != "" {
					t.Errorf("GitCommit = %q, want empty", GitCommit)
				}
				if tc.wantDateEmpty && BuildDate != "" {
					t.Errorf("BuildDate = %q, want empty", BuildDate)
				}
			})
		})
	}
}

func TestApplyBuildInfo(t *testing.T) {
	tests := []struct {
		name        string
		startCommit string
		startDate   string
		mainVersion string
		settings    []debug.BuildSetting
		wantCommit  string
		wantDate    string
	}{
		{
			name:        "long revision truncated to 7 chars",
			startCommit: "",
			startDate:   "",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0123456789"},
				{Key: "vcs.modified", Value: "false"},
				{Key: "vcs.time", Value: "2026-05-06T18:30:00Z"},
			},
			wantCommit: "abcdef0",
			wantDate:   "2026-05-06T18:30:00Z",
		},
		{
			name:        "modified true appends -dirty",
			startCommit: "",
			startDate:   "",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0123456789"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.time", Value: "2026-05-06T18:30:00Z"},
			},
			wantCommit: "abcdef0-dirty",
			wantDate:   "2026-05-06T18:30:00Z",
		},
		{
			name:        "short revision passes through",
			startCommit: "",
			startDate:   "",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc"},
				{Key: "vcs.modified", Value: "false"},
			},
			wantCommit: "abc",
			wantDate:   "",
		},
		{
			name:        "all settings empty is a no-op",
			startCommit: "",
			startDate:   "",
			settings:    nil,
			wantCommit:  "",
			wantDate:    "",
		},
		{
			name:        "ldflags-set GitCommit is preserved",
			startCommit: "preset-commit",
			startDate:   "",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0123456789"},
				{Key: "vcs.modified", Value: "true"},
			},
			wantCommit: "preset-commit",
			wantDate:   "",
		},
		{
			name:        "ldflags-set BuildDate is preserved",
			startCommit: "",
			startDate:   "preset-date",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef0"},
				{Key: "vcs.time", Value: "2026-05-06T18:30:00Z"},
			},
			wantCommit: "abcdef0",
			wantDate:   "preset-date",
		},
		{
			name:        "pseudo-version Main.Version fills both when vcs.* absent",
			startCommit: "",
			startDate:   "",
			mainVersion: "v0.0.0-20260506200756-4fd94246d2e2",
			settings:    nil,
			wantCommit:  "4fd9424",
			wantDate:    "2026-05-06T20:07:56Z",
		},
		{
			name:        "pseudo-version filling Main.Version with patched-tag prefix",
			startCommit: "",
			startDate:   "",
			mainVersion: "v0.1.1-0.20260506200756-4fd94246d2e2",
			settings:    nil,
			wantCommit:  "4fd9424",
			wantDate:    "2026-05-06T20:07:56Z",
		},
		{
			name:        "real semver Main.Version does not populate fallback",
			startCommit: "",
			startDate:   "",
			mainVersion: "v0.1.0",
			settings:    nil,
			wantCommit:  "",
			wantDate:    "",
		},
		{
			name:        "vcs.* wins over pseudo-version",
			startCommit: "",
			startDate:   "",
			mainVersion: "v0.0.0-20260506200756-4fd94246d2e2",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "deadbeefcafe1234"},
				{Key: "vcs.time", Value: "2025-01-01T00:00:00Z"},
			},
			wantCommit: "deadbee",
			wantDate:   "2025-01-01T00:00:00Z",
		},
		{
			name:        "pseudo-version fills only BuildDate when vcs.revision present",
			startCommit: "",
			startDate:   "",
			mainVersion: "v0.0.0-20260506200756-4fd94246d2e2",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "deadbee"},
			},
			wantCommit: "deadbee",
			wantDate:   "2026-05-06T20:07:56Z",
		},
		{
			name:        "irrelevant settings ignored",
			startCommit: "",
			startDate:   "",
			settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "linux"},
				{Key: "GOARCH", Value: "amd64"},
				{Key: "vcs", Value: "git"},
			},
			wantCommit: "",
			wantDate:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withMetadata(t, "", tc.startCommit, "", "", tc.startDate, func() {
				info := &debug.BuildInfo{Settings: tc.settings}
				info.Main.Version = tc.mainVersion
				applyBuildInfo(info)
				if GitCommit != tc.wantCommit {
					t.Errorf("GitCommit = %q, want %q", GitCommit, tc.wantCommit)
				}
				if BuildDate != tc.wantDate {
					t.Errorf("BuildDate = %q, want %q", BuildDate, tc.wantDate)
				}
			})
		})
	}
}

func TestParsePseudoVersion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCommit string
		wantDate   string
		wantOK     bool
	}{
		{
			name:       "canonical v0.0.0 pseudo-version",
			input:      "v0.0.0-20260506200756-4fd94246d2e2",
			wantCommit: "4fd9424",
			wantDate:   "2026-05-06T20:07:56Z",
			wantOK:     true,
		},
		{
			name:       "pre-release pseudo-version",
			input:      "v0.1.1-0.20260506200756-4fd94246d2e2",
			wantCommit: "4fd9424",
			wantDate:   "2026-05-06T20:07:56Z",
			wantOK:     true,
		},
		{
			name:       "real semver tag is not a pseudo-version",
			input:      "v0.1.0",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "real semver pre-release is not a pseudo-version",
			input:      "v1.2.3-rc1",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "empty string",
			input:      "",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "wrong-length commit suffix",
			input:      "v0.0.0-20260506200756-4fd9424",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "wrong-length timestamp",
			input:      "v0.0.0-2026050620075-4fd94246d2e2",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "non-hex commit",
			input:      "v0.0.0-20260506200756-zzzzzzzzzzzz",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
		{
			name:       "invalid timestamp values",
			input:      "v0.0.0-99999999999999-4fd94246d2e2",
			wantCommit: "",
			wantDate:   "",
			wantOK:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			commit, date, ok := parsePseudoVersion(tc.input)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if commit != tc.wantCommit {
				t.Errorf("commit = %q, want %q", commit, tc.wantCommit)
			}
			if date != tc.wantDate {
				t.Errorf("date = %q, want %q", date, tc.wantDate)
			}
		})
	}
}

// TestReadBuildInfoFallback exercises the wrapper around debug.ReadBuildInfo.
// Drives both ok=true and ok=false branches via the readBuildInfo seam.
func TestReadBuildInfoFallback(t *testing.T) {
	t.Run("ok=false is a no-op", func(t *testing.T) {
		saveFn := readBuildInfo
		t.Cleanup(func() { readBuildInfo = saveFn })
		readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }

		withMetadata(t, "", "", "", "", "", func() {
			readBuildInfoFallback()
			if GitCommit != "" || BuildDate != "" {
				t.Errorf("unexpected mutation: GitCommit=%q BuildDate=%q", GitCommit, BuildDate)
			}
		})
	})

	t.Run("ok=true delegates to applyBuildInfo", func(t *testing.T) {
		saveFn := readBuildInfo
		t.Cleanup(func() { readBuildInfo = saveFn })
		readBuildInfo = func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abcdef0123456"},
					{Key: "vcs.modified", Value: "false"},
					{Key: "vcs.time", Value: "2026-05-06T18:30:00Z"},
				},
			}, true
		}

		withMetadata(t, "", "", "", "", "", func() {
			readBuildInfoFallback()
			if GitCommit != "abcdef0" {
				t.Errorf("GitCommit = %q, want %q", GitCommit, "abcdef0")
			}
			if BuildDate != "2026-05-06T18:30:00Z" {
				t.Errorf("BuildDate = %q, want %q", BuildDate, "2026-05-06T18:30:00Z")
			}
		})
	})
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
