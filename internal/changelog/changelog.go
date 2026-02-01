// Package changelog handles generating and updating CHANGELOG.md files.
// It generates entries in the conventional-changelog format used by Release Please.
package changelog

import (
	"fmt"
	"strings"
	"time"

	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/pkg/contracts"
)

// Entry represents a changelog entry for a single version.
type Entry struct {
	Version     string
	Date        time.Time
	CompareURL  string // URL to compare with previous version
	Commits     []*git.Commit
	Component   string
	RepoURL     string
	PrevVersion string
}

// Generate creates a changelog entry string from the given commits.
// Format matches Release Please's conventional-changelog output.
func Generate(entry *Entry) string {
	contracts.RequireNotNil(entry, "entry")
	contracts.RequireNotEmpty(entry.Version, "version")
	contracts.Require(len(entry.Commits) > 0, "commits cannot be empty")

	var sb strings.Builder

	// Header with version, compare link, and date
	dateStr := entry.Date.Format("2006-01-02")

	if entry.CompareURL != "" {
		sb.WriteString(fmt.Sprintf("## [%s](%s) (%s)\n\n", entry.Version, entry.CompareURL, dateStr))
	} else {
		sb.WriteString(fmt.Sprintf("## [%s] (%s)\n\n", entry.Version, dateStr))
	}

	// Group commits by type
	features := filterCommitsByType(entry.Commits, "feat")
	fixes := filterCommitsByType(entry.Commits, "fix")
	perfs := filterCommitsByType(entry.Commits, "perf")
	breaking := filterBreakingChanges(entry.Commits)

	// Breaking changes section (if any)
	if len(breaking) > 0 {
		sb.WriteString("### ⚠ BREAKING CHANGES\n\n")
		for _, c := range breaking {
			sb.WriteString(formatCommitLine(c, entry.RepoURL))
		}
		sb.WriteString("\n")
	}

	// Features section
	if len(features) > 0 {
		sb.WriteString("### Features\n\n")
		for _, c := range features {
			sb.WriteString(formatCommitLine(c, entry.RepoURL))
		}
		sb.WriteString("\n")
	}

	// Bug Fixes section
	if len(fixes) > 0 {
		sb.WriteString("### Bug Fixes\n\n")
		for _, c := range fixes {
			sb.WriteString(formatCommitLine(c, entry.RepoURL))
		}
		sb.WriteString("\n")
	}

	// Performance Improvements section
	if len(perfs) > 0 {
		sb.WriteString("### Performance Improvements\n\n")
		for _, c := range perfs {
			sb.WriteString(formatCommitLine(c, entry.RepoURL))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Prepend adds a new entry to the top of an existing changelog.
// It preserves any content after the first "## " header.
func Prepend(existingChangelog, newEntry string) string {
	// Find where to insert (after the title, before first version entry)
	lines := strings.Split(existingChangelog, "\n")
	var headerLines []string
	var restLines []string
	foundHeader := false

	for i, line := range lines {
		// Look for first version header (## [x.x.x] or ## x.x.x)
		if strings.HasPrefix(line, "## ") && (strings.Contains(line, "[") || strings.Contains(line, "(")) {
			foundHeader = true
			restLines = lines[i:]
			break
		}
		headerLines = append(headerLines, line)
	}

	if !foundHeader {
		// No existing version entries, just append
		return existingChangelog + "\n" + newEntry
	}

	// Rebuild: header + new entry + existing entries
	result := strings.Join(headerLines, "\n")
	if !strings.HasSuffix(result, "\n\n") {
		if strings.HasSuffix(result, "\n") {
			result += "\n"
		} else {
			result += "\n\n"
		}
	}
	result += newEntry
	result += strings.Join(restLines, "\n")

	return result
}

// BuildCompareURL creates a GitHub compare URL between two versions.
func BuildCompareURL(repoURL, component, prevVersion, newVersion string) string {
	if repoURL == "" || prevVersion == "" {
		return ""
	}

	// Ensure repoURL doesn't end with /
	repoURL = strings.TrimSuffix(repoURL, "/")

	// Tag format: component-vX.Y.Z
	prevTag := fmt.Sprintf("%s-v%s", component, prevVersion)
	newTag := fmt.Sprintf("%s-v%s", component, newVersion)

	return fmt.Sprintf("%s/compare/%s...%s", repoURL, prevTag, newTag)
}

// filterCommitsByType returns commits matching the given type.
func filterCommitsByType(commits []*git.Commit, commitType string) []*git.Commit {
	var result []*git.Commit
	for _, c := range commits {
		if c.Type == commitType {
			result = append(result, c)
		}
	}
	return result
}

// filterBreakingChanges returns commits that are breaking changes.
func filterBreakingChanges(commits []*git.Commit) []*git.Commit {
	var result []*git.Commit
	for _, c := range commits {
		if c.IsBreaking {
			result = append(result, c)
		}
	}
	return result
}

// formatCommitLine formats a single commit as a changelog bullet point.
func formatCommitLine(commit *git.Commit, repoURL string) string {
	desc := commit.Description
	if commit.Scope != "" {
		desc = fmt.Sprintf("**%s:** %s", commit.Scope, desc)
	}

	if repoURL != "" {
		commitURL := fmt.Sprintf("%s/commit/%s", strings.TrimSuffix(repoURL, "/"), commit.SHA)
		return fmt.Sprintf("* %s ([%s](%s))\n", desc, commit.ShortSHA, commitURL)
	}

	return fmt.Sprintf("* %s (%s)\n", desc, commit.ShortSHA)
}

// InitialChangelog returns the template for a new CHANGELOG.md file.
func InitialChangelog() string {
	return `# Changelog

All notable changes to this project will be documented in this file.

`
}
