package release

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spraguehouse/release-damnit/internal/git"
)

// GitHubReleaseOptions configures GitHub release creation.
type GitHubReleaseOptions struct {
	// RepoPath is the path to the git repository.
	RepoPath string

	// DryRun if true, don't actually create releases.
	DryRun bool

	// Verbose if true, print release notes before creating.
	Verbose bool
}

// GitHubRelease represents a GitHub release to be created.
type GitHubRelease struct {
	TagName     string
	Title       string
	Notes       string
	TargetSHA   string
	PackageInfo *PackageRelease
}

// CreateGitHubReleases creates GitHub releases for all packages in the result.
func CreateGitHubReleases(result *AnalysisResult, opts *GitHubReleaseOptions) ([]*GitHubRelease, error) {
	if opts == nil {
		opts = &GitHubReleaseOptions{}
	}

	var releases []*GitHubRelease

	for _, rel := range result.Releases {
		ghRelease := BuildGitHubRelease(rel, result.RepoURL)
		ghRelease.TargetSHA = result.MergeInfo.HeadSHA

		if opts.DryRun {
			releases = append(releases, ghRelease)
			continue
		}

		if err := executeGitHubRelease(opts.RepoPath, ghRelease); err != nil {
			return releases, fmt.Errorf("failed to create release for %s: %w", rel.Package.Component, err)
		}

		releases = append(releases, ghRelease)
	}

	return releases, nil
}

// BuildGitHubRelease constructs a GitHubRelease from a PackageRelease.
func BuildGitHubRelease(rel *PackageRelease, repoURL string) *GitHubRelease {
	tagName := fmt.Sprintf("%s-v%s", rel.Package.Component, rel.NewVersion)
	title := fmt.Sprintf("%s v%s", rel.Package.Component, rel.NewVersion)
	notes := BuildReleaseNotes(rel, repoURL)

	return &GitHubRelease{
		TagName:     tagName,
		Title:       title,
		Notes:       notes,
		PackageInfo: rel,
	}
}

// BuildReleaseNotes generates release notes from commits.
func BuildReleaseNotes(rel *PackageRelease, repoURL string) string {
	var notes strings.Builder

	notes.WriteString(fmt.Sprintf("## %s v%s\n\n", rel.Package.Component, rel.NewVersion))

	features := filterCommitsByType(rel.Commits, "feat")
	fixes := filterCommitsByType(rel.Commits, "fix")
	perfs := filterCommitsByType(rel.Commits, "perf")

	if len(features) > 0 {
		notes.WriteString("### Features\n\n")
		for _, c := range features {
			commitLink := formatCommitLink(c, repoURL)
			notes.WriteString(fmt.Sprintf("* %s (%s)\n", c.Description, commitLink))
		}
		notes.WriteString("\n")
	}

	if len(fixes) > 0 {
		notes.WriteString("### Bug Fixes\n\n")
		for _, c := range fixes {
			commitLink := formatCommitLink(c, repoURL)
			notes.WriteString(fmt.Sprintf("* %s (%s)\n", c.Description, commitLink))
		}
		notes.WriteString("\n")
	}

	if len(perfs) > 0 {
		notes.WriteString("### Performance Improvements\n\n")
		for _, c := range perfs {
			commitLink := formatCommitLink(c, repoURL)
			notes.WriteString(fmt.Sprintf("* %s (%s)\n", c.Description, commitLink))
		}
		notes.WriteString("\n")
	}

	// Add compare link if we have a repo URL and old version
	if repoURL != "" && rel.OldVersion != "" {
		oldTag := fmt.Sprintf("%s-v%s", rel.Package.Component, rel.OldVersion)
		newTag := fmt.Sprintf("%s-v%s", rel.Package.Component, rel.NewVersion)
		compareURL := fmt.Sprintf("%s/compare/%s...%s", repoURL, oldTag, newTag)
		notes.WriteString(fmt.Sprintf("**Full Changelog**: %s\n", compareURL))
	}

	return notes.String()
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

func formatCommitLink(c *git.Commit, repoURL string) string {
	if repoURL == "" {
		return c.ShortSHA
	}
	return fmt.Sprintf("[%s](%s/commit/%s)", c.ShortSHA, repoURL, c.SHA)
}

// executeGitHubRelease creates a release using the gh CLI.
func executeGitHubRelease(repoPath string, ghRelease *GitHubRelease) error {
	args := []string{
		"release", "create", ghRelease.TagName,
		"--title", ghRelease.Title,
		"--notes", ghRelease.Notes,
	}

	if ghRelease.TargetSHA != "" {
		args = append(args, "--target", ghRelease.TargetSHA)
	}

	cmd := exec.Command("gh", args...)
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// CheckGHCLI verifies that the gh CLI is installed and authenticated.
func CheckGHCLI() error {
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh CLI not authenticated - run 'gh auth login' first")
	}
	return nil
}

// DeleteGitHubRelease deletes a release (useful for testing).
func DeleteGitHubRelease(repoPath, tagName string) error {
	cmd := exec.Command("gh", "release", "delete", tagName, "--yes", "--cleanup-tag")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	return cmd.Run()
}
