package release

import (
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/version"
)

func TestBuildGitHubRelease(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		BumpType:   version.Minor,
		OldVersion: "1.0.0",
		NewVersion: "1.1.0",
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "add new feature"},
		},
	}

	ghRelease := BuildGitHubRelease(rel, "https://github.com/owner/repo")

	if ghRelease.TagName != "service-a-v1.1.0" {
		t.Errorf("expected tag service-a-v1.1.0, got %s", ghRelease.TagName)
	}

	if ghRelease.Title != "service-a v1.1.0" {
		t.Errorf("expected title 'service-a v1.1.0', got %s", ghRelease.Title)
	}

	if !strings.Contains(ghRelease.Notes, "add new feature") {
		t.Error("release notes should contain commit description")
	}

	if ghRelease.PackageInfo != rel {
		t.Error("PackageInfo should reference original release")
	}
}

func TestBuildReleaseNotes_FeaturesAndFixes(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		OldVersion: "1.0.0",
		NewVersion: "1.1.0",
		Commits: []*git.Commit{
			{SHA: "aaa1111111111", ShortSHA: "aaa1111", Type: "feat", Description: "add calendar sync"},
			{SHA: "bbb2222222222", ShortSHA: "bbb2222", Type: "fix", Description: "fix null pointer"},
			{SHA: "ccc3333333333", ShortSHA: "ccc3333", Type: "feat", Description: "add notifications"},
			{SHA: "ddd4444444444", ShortSHA: "ddd4444", Type: "chore", Description: "update deps"},
		},
	}

	notes := BuildReleaseNotes(rel, "https://github.com/owner/repo")

	// Check header
	if !strings.Contains(notes, "## service-a v1.1.0") {
		t.Error("notes should have version header")
	}

	// Check features section
	if !strings.Contains(notes, "### Features") {
		t.Error("notes should have Features section")
	}
	if !strings.Contains(notes, "add calendar sync") {
		t.Error("notes should contain first feature")
	}
	if !strings.Contains(notes, "add notifications") {
		t.Error("notes should contain second feature")
	}

	// Check fixes section
	if !strings.Contains(notes, "### Bug Fixes") {
		t.Error("notes should have Bug Fixes section")
	}
	if !strings.Contains(notes, "fix null pointer") {
		t.Error("notes should contain fix")
	}

	// Chore commits should NOT be in notes
	if strings.Contains(notes, "update deps") {
		t.Error("notes should not contain chore commits")
	}

	// Check compare link
	if !strings.Contains(notes, "Full Changelog") {
		t.Error("notes should have Full Changelog link")
	}
	if !strings.Contains(notes, "service-a-v1.0.0...service-a-v1.1.0") {
		t.Error("notes should have correct compare URL")
	}
}

func TestBuildReleaseNotes_WithCommitLinks(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		NewVersion: "1.0.0",
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "add feature"},
		},
	}

	notes := BuildReleaseNotes(rel, "https://github.com/owner/repo")

	// Should have markdown link to commit
	if !strings.Contains(notes, "[abc1234](https://github.com/owner/repo/commit/abc1234567890)") {
		t.Error("notes should have commit link")
	}
}

func TestBuildReleaseNotes_NoRepoURL(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		NewVersion: "1.0.0",
		Commits: []*git.Commit{
			{SHA: "abc1234567890", ShortSHA: "abc1234", Type: "feat", Description: "add feature"},
		},
	}

	notes := BuildReleaseNotes(rel, "")

	// Should have short SHA without link
	if !strings.Contains(notes, "(abc1234)") {
		t.Error("notes should have short SHA")
	}
	// Should NOT have markdown link
	if strings.Contains(notes, "[abc1234]") {
		t.Error("notes should not have commit link without repo URL")
	}
}

func TestBuildReleaseNotes_PerformanceImprovements(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		NewVersion: "1.0.1",
		Commits: []*git.Commit{
			{SHA: "aaa1111111111", ShortSHA: "aaa1111", Type: "perf", Description: "optimize query"},
		},
	}

	notes := BuildReleaseNotes(rel, "")

	if !strings.Contains(notes, "### Performance Improvements") {
		t.Error("notes should have Performance Improvements section")
	}
	if !strings.Contains(notes, "optimize query") {
		t.Error("notes should contain perf commit")
	}
}

func TestBuildReleaseNotes_EmptyCommits(t *testing.T) {
	rel := &PackageRelease{
		Package: &config.Package{
			Path:      "workloads/service-a",
			Component: "service-a",
		},
		NewVersion: "1.0.0",
		Commits:    []*git.Commit{},
	}

	notes := BuildReleaseNotes(rel, "https://github.com/owner/repo")

	// Should still have header
	if !strings.Contains(notes, "## service-a v1.0.0") {
		t.Error("notes should have version header even with no commits")
	}

	// Should NOT have sections
	if strings.Contains(notes, "### Features") {
		t.Error("notes should not have Features section with no commits")
	}
}

func TestFilterCommitsByType(t *testing.T) {
	commits := []*git.Commit{
		{Type: "feat", Description: "feature 1"},
		{Type: "fix", Description: "fix 1"},
		{Type: "feat", Description: "feature 2"},
		{Type: "chore", Description: "chore 1"},
		{Type: "fix", Description: "fix 2"},
	}

	feats := filterCommitsByType(commits, "feat")
	if len(feats) != 2 {
		t.Errorf("expected 2 feat commits, got %d", len(feats))
	}

	fixes := filterCommitsByType(commits, "fix")
	if len(fixes) != 2 {
		t.Errorf("expected 2 fix commits, got %d", len(fixes))
	}

	chores := filterCommitsByType(commits, "chore")
	if len(chores) != 1 {
		t.Errorf("expected 1 chore commit, got %d", len(chores))
	}

	empty := filterCommitsByType(commits, "perf")
	if len(empty) != 0 {
		t.Errorf("expected 0 perf commits, got %d", len(empty))
	}
}

func TestFormatCommitLink(t *testing.T) {
	c := &git.Commit{
		SHA:      "abc1234567890",
		ShortSHA: "abc1234",
	}

	// With repo URL
	link := formatCommitLink(c, "https://github.com/owner/repo")
	expected := "[abc1234](https://github.com/owner/repo/commit/abc1234567890)"
	if link != expected {
		t.Errorf("expected %s, got %s", expected, link)
	}

	// Without repo URL
	link = formatCommitLink(c, "")
	if link != "abc1234" {
		t.Errorf("expected abc1234, got %s", link)
	}
}
