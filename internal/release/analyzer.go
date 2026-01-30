// Package release contains the core release logic that ties together
// config, git, version, and changelog packages.
package release

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spraguehouse/release-damnit/internal/changelog"
	"github.com/spraguehouse/release-damnit/internal/config"
	"github.com/spraguehouse/release-damnit/internal/git"
	"github.com/spraguehouse/release-damnit/internal/version"
	"github.com/spraguehouse/release-damnit/pkg/contracts"
)

// PackageRelease represents the release information for a single package.
type PackageRelease struct {
	Package     *config.Package
	BumpType    version.BumpType
	OldVersion  string
	NewVersion  string
	Commits     []*git.Commit
	SkipReason  string // Set if this package is being skipped (e.g., linked to another)
}

// AnalysisResult contains the result of analyzing commits for releases.
type AnalysisResult struct {
	// MergeInfo contains information about the merge commit (if applicable).
	MergeInfo *git.MergeInfo

	// Commits is the list of commits that were analyzed.
	Commits []*git.Commit

	// Releases is the list of packages that will be released.
	Releases []*PackageRelease

	// Config is the loaded configuration.
	Config *config.Config

	// RepoURL is the GitHub repository URL (for changelog links).
	RepoURL string
}

// Options configures the release analysis.
type Options struct {
	// RepoPath is the path to the git repository root.
	RepoPath string

	// DryRun if true, don't make any changes.
	DryRun bool

	// RepoURL is the GitHub repository URL (e.g., "https://github.com/owner/repo").
	RepoURL string

	// TreatPreMajorAsMinor if true, feat bumps patch for 0.x versions.
	TreatPreMajorAsMinor bool
}

// Analyze analyzes HEAD for releasable changes.
func Analyze(opts *Options) (*AnalysisResult, error) {
	contracts.RequireNotNil(opts, "opts")
	contracts.RequireNotEmpty(opts.RepoPath, "RepoPath")

	// Load config
	cfg, err := config.Load(opts.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Analyze HEAD
	mergeInfo, err := git.AnalyzeHead(opts.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze HEAD: %w", err)
	}

	// Get commits to analyze
	var commits []*git.Commit
	if mergeInfo.IsMerge {
		// Get commits from merge base to merge head (second parent)
		commits, err = git.GetCommitsInRange(opts.RepoPath, mergeInfo.MergeBase, mergeInfo.MergeHead)
		if err != nil {
			return nil, fmt.Errorf("failed to get merge commits: %w", err)
		}
	} else {
		// Fall back to HEAD~1..HEAD for non-merge commits
		// This may fail if there's only one commit in the repo
		commits, err = git.GetCommitsInRange(opts.RepoPath, "HEAD~1", "HEAD")
		if err != nil {
			// If HEAD~1 doesn't exist (single commit repo), return empty commits
			commits = nil
		}
	}

	// Map commits to packages
	packageCommits := make(map[string][]*git.Commit)
	for _, commit := range commits {
		for _, file := range commit.Files {
			pkg := cfg.FindPackageForPath(file)
			if pkg != nil {
				packageCommits[pkg.Path] = append(packageCommits[pkg.Path], commit)
			}
		}
	}

	// Calculate bumps per package
	releases := calculateReleases(cfg, packageCommits, opts.TreatPreMajorAsMinor)

	result := &AnalysisResult{
		MergeInfo: mergeInfo,
		Commits:   commits,
		Releases:  releases,
		Config:    cfg,
		RepoURL:   opts.RepoURL,
	}

	return result, nil
}

// calculateReleases determines which packages need releases and their version bumps.
func calculateReleases(cfg *config.Config, packageCommits map[string][]*git.Commit, treatPreMajorAsMinor bool) []*PackageRelease {
	var releases []*PackageRelease
	processedLinkedGroups := make(map[string]bool)

	// Process packages in deterministic order
	for _, pkg := range cfg.PackagesSortedByPath() {
		commits := packageCommits[pkg.Path]
		if len(commits) == 0 {
			continue
		}

		// Calculate bump type from commits
		var maxBump version.BumpType
		for _, commit := range commits {
			if commit.IsBreaking {
				maxBump = version.Major
				break // Can't go higher
			}
			bump := version.CommitTypeToBump(commit.Type)
			maxBump = version.MaxBump(maxBump, bump)
		}

		if maxBump == version.None {
			continue // No releasable commits
		}

		// Handle linked versions
		if pkg.LinkedGroup != "" {
			if processedLinkedGroups[pkg.LinkedGroup] {
				// Already processed this group
				continue
			}
			processedLinkedGroups[pkg.LinkedGroup] = true

			// Get all packages in the group and find max bump
			linkedPackages := cfg.GetLinkedPackages(pkg)
			for _, linkedPkg := range linkedPackages {
				linkedCommits := packageCommits[linkedPkg.Path]
				for _, commit := range linkedCommits {
					if commit.IsBreaking {
						maxBump = version.Major
						break
					}
					bump := version.CommitTypeToBump(commit.Type)
					maxBump = version.MaxBump(maxBump, bump)
				}
			}

			// Create releases for all linked packages
			for _, linkedPkg := range linkedPackages {
				release := createRelease(linkedPkg, packageCommits[linkedPkg.Path], maxBump, treatPreMajorAsMinor)
				releases = append(releases, release)
			}
		} else {
			// Not linked - create single release
			release := createRelease(pkg, commits, maxBump, treatPreMajorAsMinor)
			releases = append(releases, release)
		}
	}

	// Sort releases by path for deterministic output
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Package.Path < releases[j].Package.Path
	})

	return releases
}

// createRelease creates a PackageRelease for a package.
func createRelease(pkg *config.Package, commits []*git.Commit, bumpType version.BumpType, treatPreMajorAsMinor bool) *PackageRelease {
	oldVersion := pkg.CurrentVersion
	if oldVersion == "" {
		oldVersion = "0.0.0"
	}

	v, err := version.Parse(oldVersion)
	if err != nil {
		// Invalid version - start from 0.1.0
		v = &version.Version{Major: 0, Minor: 1, Patch: 0}
		oldVersion = "0.1.0"
	}

	newVersion := v.Bump(bumpType, treatPreMajorAsMinor)

	// Deduplicate commits (a commit might touch multiple files in the package)
	seen := make(map[string]bool)
	var uniqueCommits []*git.Commit
	for _, c := range commits {
		if !seen[c.SHA] {
			seen[c.SHA] = true
			uniqueCommits = append(uniqueCommits, c)
		}
	}

	return &PackageRelease{
		Package:    pkg,
		BumpType:   bumpType,
		OldVersion: oldVersion,
		NewVersion: newVersion.String(),
		Commits:    uniqueCommits,
	}
}

// Apply writes the version updates, changelogs, and manifest updates.
func Apply(result *AnalysisResult, dryRun bool) error {
	contracts.RequireNotNil(result, "result")

	if len(result.Releases) == 0 {
		return nil
	}

	// Update manifest with new versions
	manifestUpdates := make(map[string]string)
	for _, rel := range result.Releases {
		manifestUpdates[rel.Package.Path] = rel.NewVersion
	}

	// Update VERSION files and CHANGELOGs
	for _, rel := range result.Releases {
		if dryRun {
			continue
		}

		// Update VERSION file
		versionPath := filepath.Join(result.Config.RepoRoot, rel.Package.Path, "VERSION")
		if err := updateVersionFile(versionPath, rel.NewVersion); err != nil {
			return fmt.Errorf("failed to update VERSION for %s: %w", rel.Package.Component, err)
		}

		// Update CHANGELOG
		changelogPath := filepath.Join(result.Config.RepoRoot, rel.Package.Path, rel.Package.ChangelogPath)
		compareURL := changelog.BuildCompareURL(result.RepoURL, rel.Package.Component, rel.OldVersion, rel.NewVersion)
		if err := updateChangelog(changelogPath, rel, compareURL, result.RepoURL); err != nil {
			return fmt.Errorf("failed to update CHANGELOG for %s: %w", rel.Package.Component, err)
		}
	}

	// Update manifest file
	if !dryRun {
		if err := updateManifest(result.Config.RepoRoot, manifestUpdates); err != nil {
			return fmt.Errorf("failed to update manifest: %w", err)
		}
	}

	return nil
}

// updateVersionFile updates a VERSION file with the new version.
func updateVersionFile(path, newVersion string) error {
	// Read existing content to preserve format
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := version.FormatVersionFile(newVersion, string(existing))

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// updateChangelog updates a CHANGELOG.md file with a new entry.
func updateChangelog(path string, rel *PackageRelease, compareURL, repoURL string) error {
	// Skip changelog update if there are no commits
	// This can happen for linked packages that weren't directly modified
	if len(rel.Commits) == 0 {
		return nil
	}

	// Read existing content
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			existing = []byte(changelog.InitialChangelog())
		} else {
			return err
		}
	}

	// Generate new entry
	entry := &changelog.Entry{
		Version:     rel.NewVersion,
		Date:        time.Now(),
		CompareURL:  compareURL,
		Commits:     rel.Commits,
		Component:   rel.Package.Component,
		RepoURL:     repoURL,
		PrevVersion: rel.OldVersion,
	}

	newEntry := changelog.Generate(entry)
	updated := changelog.Prepend(string(existing), newEntry)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(updated), 0644)
}

// updateManifest updates release-please-manifest.json with new versions.
func updateManifest(repoRoot string, updates map[string]string) error {
	manifestPath := filepath.Join(repoRoot, "release-please-manifest.json")

	// Read existing manifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	// Parse as generic JSON (preserves order)
	// For simplicity, we'll use string manipulation to update values
	content := string(data)

	for path, version := range updates {
		// Find and replace the version for this path
		// Format: "path": "version"
		oldPattern := fmt.Sprintf(`"%s": "[^"]*"`, path)
		newValue := fmt.Sprintf(`"%s": "%s"`, path, version)

		// Simple replacement (works for well-formatted JSON)
		content = replaceJSONValue(content, path, version)
		_ = oldPattern
		_ = newValue
	}

	return os.WriteFile(manifestPath, []byte(content), 0644)
}

// replaceJSONValue replaces a value in a JSON object.
// This is a simple string-based approach that works for our use case.
func replaceJSONValue(json, key, newValue string) string {
	// Find the key
	keyStr := fmt.Sprintf(`"%s":`, key)
	keyIdx := 0
	for {
		idx := indexOf(json[keyIdx:], keyStr)
		if idx == -1 {
			break
		}
		idx += keyIdx

		// Find the value (skip whitespace, find opening quote)
		valueStart := idx + len(keyStr)
		for valueStart < len(json) && (json[valueStart] == ' ' || json[valueStart] == '\t') {
			valueStart++
		}

		if valueStart >= len(json) || json[valueStart] != '"' {
			keyIdx = idx + 1
			continue
		}

		// Find closing quote
		valueEnd := valueStart + 1
		for valueEnd < len(json) && json[valueEnd] != '"' {
			if json[valueEnd] == '\\' {
				valueEnd++ // Skip escaped character
			}
			valueEnd++
		}

		if valueEnd >= len(json) {
			break
		}

		// Replace the value
		return json[:valueStart+1] + newValue + json[valueEnd:]
	}

	return json
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
