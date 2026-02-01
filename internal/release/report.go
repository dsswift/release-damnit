// Package release contains the core release logic that ties together
// config, git, version, and changelog packages.
package release

import (
	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
)

// ReleaseReport is the comprehensive JSON output for downstream workflows.
// It enables simple component checks (contains in components array) and
// detailed access to version info, commits, and release URLs.
type ReleaseReport struct {
	// Releases contains details for each released package.
	Releases []ComponentRelease `json:"releases"`

	// Components is a flat list of component names that were released.
	// Enables simple checks: contains(fromJSON(outputs.release_report).components, 'jarvis')
	Components []string `json:"components"`

	// Summary provides aggregate statistics about the release.
	Summary ReleaseSummary `json:"summary"`
}

// ComponentRelease contains release information for a single component.
type ComponentRelease struct {
	// Component is the package name (e.g., "jarvis").
	Component string `json:"component"`

	// Path is the relative path from repo root (e.g., "workloads/jarvis").
	Path string `json:"path"`

	// OldVersion is the previous version.
	OldVersion string `json:"old_version"`

	// NewVersion is the new version being released.
	NewVersion string `json:"new_version"`

	// BumpType is "major", "minor", or "patch".
	BumpType string `json:"bump_type"`

	// TagName is the git tag (e.g., "jarvis-v0.1.120").
	TagName string `json:"tag_name"`

	// ReleaseURL is the GitHub release URL (if created).
	ReleaseURL string `json:"release_url,omitempty"`

	// LinkedBump is true if this release was bumped due to linked-versions.
	LinkedBump bool `json:"linked_bump"`

	// Commits contains the commits that triggered this release.
	Commits []CommitInfo `json:"commits"`
}

// CommitInfo contains commit details for the release report.
type CommitInfo struct {
	// SHA is the full commit hash.
	SHA string `json:"sha"`

	// Type is the conventional commit type (feat, fix, etc.).
	Type string `json:"type"`

	// Scope is the commit scope (optional).
	Scope string `json:"scope,omitempty"`

	// Description is the commit message subject.
	Description string `json:"description"`

	// Breaking indicates if this is a breaking change.
	Breaking bool `json:"breaking"`
}

// ReleaseSummary provides aggregate statistics.
type ReleaseSummary struct {
	// TotalReleases is the number of packages released.
	TotalReleases int `json:"total_releases"`

	// TotalCommits is the total number of commits analyzed.
	TotalCommits int `json:"total_commits"`

	// ByBumpType breaks down releases by bump type.
	ByBumpType BumpTypeCounts `json:"by_bump_type"`
}

// BumpTypeCounts counts releases by bump type.
type BumpTypeCounts struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

// AnalysisInput is the JSON output showing what data was used for release decisions.
// This enables debugging, auditing, and verification of the release process.
type AnalysisInput struct {
	// Git contains information about the analyzed commit(s).
	Git GitInfo `json:"git"`

	// CommitsAnalyzed lists all commits that were considered.
	CommitsAnalyzed []AnalyzedCommit `json:"commits_analyzed"`

	// Config summarizes the release configuration used.
	Config ConfigSummary `json:"config"`
}

// GitInfo contains git state information.
type GitInfo struct {
	// HeadSHA is the SHA of HEAD.
	HeadSHA string `json:"head_sha"`

	// IsMergeCommit indicates if HEAD is a merge commit.
	IsMergeCommit bool `json:"is_merge_commit"`

	// MergeBase is the common ancestor (for merge commits).
	MergeBase string `json:"merge_base,omitempty"`

	// MergeHead is the tip of the merged branch (for merge commits).
	MergeHead string `json:"merge_head,omitempty"`

	// Branch is the current branch name (if available).
	Branch string `json:"branch,omitempty"`
}

// AnalyzedCommit contains details about a commit that was analyzed.
type AnalyzedCommit struct {
	// SHA is the full commit hash.
	SHA string `json:"sha"`

	// Message is the full commit message subject.
	Message string `json:"message"`

	// Type is the conventional commit type.
	Type string `json:"type,omitempty"`

	// Scope is the commit scope.
	Scope string `json:"scope,omitempty"`

	// Breaking indicates if this is a breaking change.
	Breaking bool `json:"breaking"`

	// FilesChanged lists files modified by this commit.
	FilesChanged []string `json:"files_changed"`

	// PackagesMatched lists package components this commit affects.
	PackagesMatched []string `json:"packages_matched"`
}

// ConfigSummary summarizes the release configuration.
type ConfigSummary struct {
	// Packages maps path to component name.
	Packages map[string]string `json:"packages"`

	// LinkedGroups maps group name to component names.
	LinkedGroups map[string][]string `json:"linked_groups"`
}

// BuildReleaseReport creates a ReleaseReport from an AnalysisResult.
func BuildReleaseReport(result *AnalysisResult, repoURL string) *ReleaseReport {
	report := &ReleaseReport{
		Releases:   make([]ComponentRelease, 0, len(result.Releases)),
		Components: make([]string, 0, len(result.Releases)),
		Summary: ReleaseSummary{
			TotalReleases: len(result.Releases),
			TotalCommits:  len(result.Commits),
		},
	}

	for _, rel := range result.Releases {
		compRelease := ComponentRelease{
			Component:  rel.Package.Component,
			Path:       rel.Package.Path,
			OldVersion: rel.OldVersion,
			NewVersion: rel.NewVersion,
			BumpType:   rel.BumpType.String(),
			TagName:    buildTagName(rel.Package.Component, rel.NewVersion),
			LinkedBump: len(rel.Commits) == 0 && rel.Package.LinkedGroup != "",
			Commits:    make([]CommitInfo, 0, len(rel.Commits)),
		}

		// Build release URL if repo URL is available
		if repoURL != "" {
			compRelease.ReleaseURL = buildReleaseURL(repoURL, compRelease.TagName)
		}

		// Convert commits
		for _, c := range rel.Commits {
			compRelease.Commits = append(compRelease.Commits, CommitInfo{
				SHA:         c.SHA,
				Type:        c.Type,
				Scope:       c.Scope,
				Description: c.Description,
				Breaking:    c.IsBreaking,
			})
		}

		report.Releases = append(report.Releases, compRelease)
		report.Components = append(report.Components, rel.Package.Component)

		// Update bump type counts
		switch rel.BumpType.String() {
		case "major":
			report.Summary.ByBumpType.Major++
		case "minor":
			report.Summary.ByBumpType.Minor++
		case "patch":
			report.Summary.ByBumpType.Patch++
		}
	}

	return report
}

// BuildAnalysisInput creates an AnalysisInput from an AnalysisResult.
func BuildAnalysisInput(result *AnalysisResult) *AnalysisInput {
	input := &AnalysisInput{
		Git: GitInfo{
			HeadSHA:       result.MergeInfo.HeadSHA,
			IsMergeCommit: result.MergeInfo.IsMerge,
		},
		CommitsAnalyzed: make([]AnalyzedCommit, 0, len(result.Commits)),
		Config: ConfigSummary{
			Packages:     make(map[string]string),
			LinkedGroups: make(map[string][]string),
		},
	}

	// Set merge info if applicable
	if result.MergeInfo.IsMerge {
		input.Git.MergeBase = result.MergeInfo.MergeBase
		input.Git.MergeHead = result.MergeInfo.MergeHead
	}

	// Build package to component map for matching
	pathToComponent := make(map[string]string)
	for _, pkg := range result.Config.Packages {
		pathToComponent[pkg.Path] = pkg.Component
		input.Config.Packages[pkg.Path] = pkg.Component
	}

	// Copy linked groups
	for groupName, components := range result.Config.LinkedGroups {
		input.Config.LinkedGroups[groupName] = components
	}

	// Convert commits with package matching
	for _, c := range result.Commits {
		analyzed := AnalyzedCommit{
			SHA:             c.SHA,
			Message:         buildCommitMessage(c),
			Type:            c.Type,
			Scope:           c.Scope,
			Breaking:        c.IsBreaking,
			FilesChanged:    c.Files,
			PackagesMatched: findMatchingPackages(c.Files, result.Config),
		}
		input.CommitsAnalyzed = append(input.CommitsAnalyzed, analyzed)
	}

	return input
}

// buildTagName creates a tag name from component and version.
func buildTagName(component, version string) string {
	return component + "-v" + version
}

// buildReleaseURL creates a GitHub release URL.
func buildReleaseURL(repoURL, tagName string) string {
	return repoURL + "/releases/tag/" + tagName
}

// buildCommitMessage reconstructs the commit message from parsed parts.
func buildCommitMessage(c *git.Commit) string {
	if c.Type == "" {
		return c.Description
	}
	if c.Scope != "" {
		prefix := c.Type + "(" + c.Scope + ")"
		if c.IsBreaking {
			prefix += "!"
		}
		return prefix + ": " + c.Description
	}
	prefix := c.Type
	if c.IsBreaking {
		prefix += "!"
	}
	return prefix + ": " + c.Description
}

// findMatchingPackages returns component names for packages that match the given files.
func findMatchingPackages(files []string, cfg *config.Config) []string {
	seen := make(map[string]bool)
	var result []string

	for _, file := range files {
		pkg := cfg.FindPackageForPath(file)
		if pkg != nil && !seen[pkg.Component] {
			seen[pkg.Component] = true
			result = append(result, pkg.Component)
		}
	}

	return result
}
