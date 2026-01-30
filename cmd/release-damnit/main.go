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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spraguehouse/release-damnit/internal/git"
	"github.com/spraguehouse/release-damnit/internal/release"
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
	printAnalysis(result)

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
			for _, rel := range result.Releases {
				if err := createGitHubRelease(repoPath, rel); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to create release for %s: %v\n", rel.Package.Component, err)
				} else {
					fmt.Printf("  Created release %s-v%s\n", rel.Package.Component, rel.NewVersion)
				}
			}
		}
	}

	// Output for GitHub Actions
	if os.Getenv("GITHUB_OUTPUT") != "" {
		writeGitHubOutput(result)
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
  release-damnit --create-releases`)
}

func printAnalysis(result *release.AnalysisResult) {
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

func createGitHubRelease(repoPath string, rel *release.PackageRelease) error {
	tagName := fmt.Sprintf("%s-v%s", rel.Package.Component, rel.NewVersion)
	title := fmt.Sprintf("%s v%s", rel.Package.Component, rel.NewVersion)

	// Build release notes from commits
	var notes strings.Builder
	notes.WriteString(fmt.Sprintf("## %s\n\n", title))

	features := filterCommitsByType(rel.Commits, "feat")
	fixes := filterCommitsByType(rel.Commits, "fix")

	if len(features) > 0 {
		notes.WriteString("### Features\n\n")
		for _, c := range features {
			notes.WriteString(fmt.Sprintf("* %s (%s)\n", c.Description, c.ShortSHA))
		}
		notes.WriteString("\n")
	}

	if len(fixes) > 0 {
		notes.WriteString("### Bug Fixes\n\n")
		for _, c := range fixes {
			notes.WriteString(fmt.Sprintf("* %s (%s)\n", c.Description, c.ShortSHA))
		}
	}

	// Create release using gh CLI
	cmd := exec.Command("gh", "release", "create", tagName,
		"--title", title,
		"--notes", notes.String(),
		"--target", "HEAD")
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func filterCommitsByType(commits []*git.Commit, commitType string) []*git.Commit {
	var result []*git.Commit
	for _, c := range commits {
		if c.Type == commitType {
			result = append(result, c)
		}
	}
	return result
}

func writeGitHubOutput(result *release.AnalysisResult) {
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

	// releases_created
	if len(result.Releases) > 0 {
		fmt.Fprintln(f, "releases_created=true")
	} else {
		fmt.Fprintln(f, "releases_created=false")
	}

	// Per-component outputs
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
