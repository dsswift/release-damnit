package changelog

import (
	"strings"
	"testing"
	"time"

	"github.com/spraguehouse/release-damnit/internal/git"
)

func TestGenerate_BasicEntry(t *testing.T) {
	entry := &Entry{
		Version: "1.2.0",
		Date:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		RepoURL: "https://github.com/owner/repo",
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "add new feature"},
			{SHA: "def5678901234", ShortSHA: "def5678", Type: "fix", Scope: "auth", Description: "fix login bug"},
		},
	}

	result := Generate(entry)

	// Check header
	if !strings.Contains(result, "## [1.2.0]") {
		t.Error("expected version header")
	}
	if !strings.Contains(result, "(2024-01-15)") {
		t.Error("expected date in header")
	}

	// Check sections
	if !strings.Contains(result, "### Features") {
		t.Error("expected Features section")
	}
	if !strings.Contains(result, "### Bug Fixes") {
		t.Error("expected Bug Fixes section")
	}

	// Check commit formatting
	if !strings.Contains(result, "* add new feature") {
		t.Error("expected feature commit")
	}
	if !strings.Contains(result, "**auth:**") {
		t.Error("expected scope formatting")
	}
	if !strings.Contains(result, "[abc1234](https://github.com/owner/repo/commit/abc1234567890)") {
		t.Error("expected commit link")
	}
}

func TestGenerate_BreakingChanges(t *testing.T) {
	entry := &Entry{
		Version: "2.0.0",
		Date:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "breaking change", IsBreaking: true},
		},
	}

	result := Generate(entry)

	if !strings.Contains(result, "### ⚠ BREAKING CHANGES") {
		t.Error("expected BREAKING CHANGES section")
	}
}

func TestGenerate_WithCompareURL(t *testing.T) {
	entry := &Entry{
		Version:    "1.2.0",
		Date:       time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		CompareURL: "https://github.com/owner/repo/compare/v1.1.0...v1.2.0",
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "new feature"},
		},
	}

	result := Generate(entry)

	if !strings.Contains(result, "[1.2.0](https://github.com/owner/repo/compare/v1.1.0...v1.2.0)") {
		t.Error("expected version with compare link")
	}
}

func TestGenerate_PerformanceSection(t *testing.T) {
	entry := &Entry{
		Version: "1.0.1",
		Date:    time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "perf", Description: "optimize query"},
		},
	}

	result := Generate(entry)

	if !strings.Contains(result, "### Performance Improvements") {
		t.Error("expected Performance Improvements section")
	}
}

func TestPrepend_ExistingChangelog(t *testing.T) {
	existing := `# Changelog

All notable changes to this project will be documented in this file.

## [1.0.0] (2024-01-01)

### Features

* initial release
`

	newEntry := `## [1.1.0] (2024-01-15)

### Bug Fixes

* fix a bug
`

	result := Prepend(existing, newEntry)

	// New entry should come before 1.0.0
	idx110 := strings.Index(result, "## [1.1.0]")
	idx100 := strings.Index(result, "## [1.0.0]")

	if idx110 == -1 {
		t.Error("expected new entry in result")
	}
	if idx100 == -1 {
		t.Error("expected old entry in result")
	}
	if idx110 > idx100 {
		t.Error("new entry should come before old entry")
	}
}

func TestPrepend_EmptyChangelog(t *testing.T) {
	existing := `# Changelog

All notable changes to this project will be documented in this file.
`

	newEntry := `## [1.0.0] (2024-01-15)

### Features

* initial release
`

	result := Prepend(existing, newEntry)

	if !strings.Contains(result, "## [1.0.0]") {
		t.Error("expected new entry in result")
	}
	if !strings.Contains(result, "# Changelog") {
		t.Error("expected original header preserved")
	}
}

func TestBuildCompareURL(t *testing.T) {
	tests := []struct {
		name        string
		repoURL     string
		component   string
		prevVersion string
		newVersion  string
		want        string
	}{
		{
			name:        "basic",
			repoURL:     "https://github.com/owner/repo",
			component:   "jarvis",
			prevVersion: "1.0.0",
			newVersion:  "1.1.0",
			want:        "https://github.com/owner/repo/compare/jarvis-v1.0.0...jarvis-v1.1.0",
		},
		{
			name:        "trailing slash",
			repoURL:     "https://github.com/owner/repo/",
			component:   "api",
			prevVersion: "0.1.0",
			newVersion:  "0.2.0",
			want:        "https://github.com/owner/repo/compare/api-v0.1.0...api-v0.2.0",
		},
		{
			name:        "no repo url",
			repoURL:     "",
			component:   "api",
			prevVersion: "0.1.0",
			newVersion:  "0.2.0",
			want:        "",
		},
		{
			name:        "no prev version",
			repoURL:     "https://github.com/owner/repo",
			component:   "api",
			prevVersion: "",
			newVersion:  "1.0.0",
			want:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildCompareURL(tc.repoURL, tc.component, tc.prevVersion, tc.newVersion)
			if got != tc.want {
				t.Errorf("BuildCompareURL() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestInitialChangelog(t *testing.T) {
	result := InitialChangelog()

	if !strings.Contains(result, "# Changelog") {
		t.Error("expected Changelog header")
	}
	if !strings.Contains(result, "All notable changes") {
		t.Error("expected standard description")
	}
}
