// release-damnit is a drop-in replacement for Release Please that correctly
// traverses merge commits.
//
// Usage:
//
//	release-damnit [options]
//
// Options:
//
//	--dry-run          Show what would be done without making changes
//	--create-releases  Create GitHub releases (requires gh CLI)
//	--repo-url URL     GitHub repository URL (auto-detected if not provided)
//	--help             Show this help
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dsswift/release-damnit/internal/release"
)

var (
	version = "dev"
	gitSha  = "unknown"
)

func main() {
	// Define flags
	dryRun := flag.Bool("dry-run", false, "Show what would be done without making changes")
	createReleases := flag.Bool("create-releases", false, "Create GitHub releases")
	repoURL := flag.String("repo-url", "", "GitHub repository URL (auto-detected if not provided)")
	verbose := flag.Bool("verbose", false, "Show detailed analysis output")
	showVersion := flag.Bool("version", false, "Show version information")
	help := flag.Bool("help", false, "Show help")

	flag.Parse()

	if *showVersion {
		fmt.Printf("release-damnit %s (%s)\n", version, gitSha)
		os.Exit(0)
	}

	if *help {
		printHelp()
		os.Exit(0)
	}

	// Get repository path
	repoPath, err := os.Getwd()
	if err != nil {
		fatal("Failed to get current directory: %v", err)
	}

	// Auto-detect repo URL if not provided
	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	// Run analysis
	opts := &release.Options{
		RepoPath:             repoPath,
		DryRun:               *dryRun,
		RepoURL:              *repoURL,
		TreatPreMajorAsMinor: true, // Default behavior for pre-1.0 packages
	}

	result, err := release.Analyze(opts)
	if err != nil {
		fatal("Analysis failed: %v", err)
	}

	// Print analysis results
	printAnalysis(result, *verbose)

	// Output for GitHub Actions (always output, even with no releases)
	// This ensures downstream jobs can safely call fromJSON on release_report
	if os.Getenv("GITHUB_OUTPUT") != "" {
		writeGitHubOutput(result, *repoURL)
	}

	if len(result.Releases) == 0 {
		fmt.Println("\nNo releasable changes.")
		os.Exit(0)
	}

	// Apply changes
	if *dryRun {
		fmt.Println("\n--dry-run specified, no changes made.")
	} else {
		fmt.Println("\nApplying changes...")
		if err := release.Apply(result, false); err != nil {
			fatal("Failed to apply changes: %v", err)
		}

		// Print what was updated
		for _, rel := range result.Releases {
			fmt.Printf("  Updated %s: %s → %s\n", rel.Package.Component, rel.OldVersion, rel.NewVersion)
		}

		// Create GitHub releases if requested
		if *createReleases {
			fmt.Println("\nCreating GitHub releases...")
			ghOpts := &release.GitHubReleaseOptions{
				RepoPath: repoPath,
				DryRun:   false,
			}
			ghReleases, err := release.CreateGitHubReleases(result, ghOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			for _, ghRel := range ghReleases {
				fmt.Printf("  Created release %s\n", ghRel.TagName)
			}
		}
	}

}

func printHelp() {
	fmt.Println(`release-damnit - Drop-in replacement for Release Please with correct merge traversal

Usage:
  release-damnit [options]

Options:
  --dry-run          Show what would be done without making changes
  --create-releases  Create GitHub releases (requires gh CLI)
  --repo-url URL     GitHub repository URL (auto-detected if not provided)
  --verbose          Show detailed analysis output (unmatched directories, commit details)
  --version          Show version information
  --help             Show this help

Environment Variables:
  GITHUB_OUTPUT      Path to GitHub Actions output file (set automatically in Actions)

Examples:
  # See what would be released
  release-damnit --dry-run

  # Update versions and changelogs
  release-damnit

  # Also create GitHub releases
  release-damnit --create-releases

  # Debug: show why commits weren't matched to packages
  release-damnit --dry-run --verbose`)
}

func printAnalysis(result *release.AnalysisResult, verbose bool) {
	if result.MergeInfo.IsMerge {
		fmt.Printf("Analyzing merge commit %s...\n", result.MergeInfo.HeadSHA[:7])
		fmt.Printf("Merge range: %s..%s (%d commits)\n",
			result.MergeInfo.MergeBase[:7],
			result.MergeInfo.MergeHead[:7],
			len(result.Commits))
	} else {
		fmt.Printf("Analyzing commit %s...\n", result.MergeInfo.HeadSHA[:7])
		fmt.Printf("Commits: %d\n", len(result.Commits))
	}

	// Always show summary line when there are unmatched commits
	if result.Stats != nil && result.Stats.TotalCommits > 0 {
		if result.Stats.UnmatchedCommits > 0 {
			fmt.Printf("  → %d matched packages, %d unmatched commits\n",
				result.Stats.MatchedCommits, result.Stats.UnmatchedCommits)
		}
	}

	// Verbose: show orphaned directories
	if verbose && result.Stats != nil && len(result.Stats.OrphanedDirs) > 0 {
		fmt.Println("\nUnmatched directories (consider adding to config):")
		for _, dir := range result.Stats.OrphanedDirs {
			fmt.Printf("  %s/\n", dir)
		}
	}

	// Verbose: show per-commit breakdown
	if verbose && len(result.Commits) > 0 {
		fmt.Println("\nCommit details:")
		for _, c := range result.Commits {
			// Format commit type and scope
			var typeStr string
			if c.Type != "" {
				if c.Scope != "" {
					typeStr = fmt.Sprintf("%s(%s)", c.Type, c.Scope)
				} else {
					typeStr = c.Type
				}
			} else {
				typeStr = "non-conventional"
			}
			fmt.Printf("  %s %s: %s\n", c.ShortSHA, typeStr, c.Description)

			// Show file-to-package mappings
			for _, file := range c.Files {
				pkg := result.Config.FindPackageForPath(file)
				if pkg != nil {
					fmt.Printf("         %s → %s\n", file, pkg.Component)
				} else {
					fmt.Printf("         %s → (no package)\n", file)
				}
			}
		}
	}

	if len(result.Releases) > 0 {
		fmt.Println("\nPackage analysis:")
		for _, rel := range result.Releases {
			commitCount := len(rel.Commits)
			fmt.Printf("  %-20s → %s (%s) [%d commit(s)]\n",
				rel.Package.Component,
				rel.BumpType,
				rel.NewVersion,
				commitCount)
		}
	}
}

func detectRepoURL(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	url := strings.TrimSpace(string(output))

	// Convert SSH URL to HTTPS
	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
	}

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	return url
}

func writeGitHubOutput(result *release.AnalysisResult, repoURL string) {
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return
	}

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to open GITHUB_OUTPUT: %v\n", err)
		return
	}
	defer f.Close()

	// releases_created (simple boolean for quick checks)
	if len(result.Releases) > 0 {
		fmt.Fprintln(f, "releases_created=true")
	} else {
		fmt.Fprintln(f, "releases_created=false")
	}

	// Build and output release_report JSON
	releaseReport := release.BuildReleaseReport(result, repoURL)
	releaseReportJSON, err := json.Marshal(releaseReport)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to marshal release_report: %v\n", err)
	} else {
		fmt.Fprintf(f, "release_report=%s\n", string(releaseReportJSON))
	}

	// Build and output analysis_input JSON
	analysisInput := release.BuildAnalysisInput(result)
	analysisInputJSON, err := json.Marshal(analysisInput)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to marshal analysis_input: %v\n", err)
	} else {
		fmt.Fprintf(f, "analysis_input=%s\n", string(analysisInputJSON))
	}

	// Per-component outputs (backward compatibility)
	for _, rel := range result.Releases {
		component := rel.Package.Component
		fmt.Fprintf(f, "%s--release_created=true\n", component)
		fmt.Fprintf(f, "%s--version=%s\n", component, rel.NewVersion)
		fmt.Fprintf(f, "%s--tag_name=%s-v%s\n", component, component, rel.NewVersion)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
