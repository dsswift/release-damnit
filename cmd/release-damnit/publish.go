package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/publish"
	"github.com/dsswift/release-damnit/internal/release"
)

// pathList is a repeatable string flag.
type pathList []string

func (l *pathList) String() string { return strings.Join(*l, " ") }

func (l *pathList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func runPublish(args []string) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	reportFile := fs.String("report-file", "", "Release report JSON written by release-damnit --report-file (required)")
	remote := fs.String("remote", "origin", "Remote to push to")
	branch := fs.String("branch", "main", "Branch the version commit goes on")
	provider := fs.String("provider", "auto", "Release provider: auto, github, or gitlab")
	gitlabToken := fs.String("gitlab-token", os.Getenv("GITLAB_RELEASE_TOKEN"), "GitLab project access token (or GITLAB_RELEASE_TOKEN)")
	attempts := fs.Int("attempts", 5, "Pushes to try when the branch keeps moving")
	var add pathList
	fs.Var(&add, "add", "Another changed file to put in the version commit (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit publish --report-file PATH [options]

Run after release-damnit --report-file PATH (without --create-releases) has
written the new versions. Commits exactly the files that run changed, plus
every --add path, as "chore: release ... [skip ci]"; tags that commit; pushes
the commit and then the tags, rebasing and retagging when the branch moved;
then creates the provider releases on those tags.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *reportFile == "" || fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, publish.Prefix+err.Error())
		return 1
	}
	repoURL := git.WebURL(wd, *remote)

	outcome, err := publish.Run(publish.Options{
		Dir:        wd,
		ReportFile: *reportFile,
		Remote:     *remote,
		Branch:     *branch,
		Attempts:   *attempts,
		Add:        add,
		RepoURL:    repoURL,
		Out:        os.Stderr,
		CreateReleases: func(result *release.AnalysisResult) error {
			return createProviderReleases(result, *provider, repoURL, wd, *gitlabToken)
		},
	})
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" && (err == nil || outcome.Released) {
		if outErr := publish.WriteGitHubOutput(path, outcome); outErr != nil {
			fmt.Fprintf(os.Stderr, "%swarning: could not write GITHUB_OUTPUT: %v\n", publish.Prefix, outErr)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, publish.Prefix+err.Error())
		return 1
	}
	if outcome.Released {
		fmt.Printf("Published %s at %s\n", outcome.Tags[0].Name, outcome.Commit[:min(len(outcome.Commit), 9)])
	}
	return 0
}

// createProviderReleases makes a release per pushed tag on GitHub or GitLab.
func createProviderReleases(result *release.AnalysisResult, provider, repoURL, repoPath, gitlabToken string) error {
	if release.DetectProvider(provider, repoURL) == "gitlab" {
		created, err := release.CreateGitLabReleases(result, &release.GitLabReleaseOptions{RepoPath: repoPath, Token: gitlabToken, UseJobToken: gitlabToken == ""})
		for _, rel := range created {
			fmt.Printf("  Created release %s\n", rel.TagName)
		}
		return err
	}
	created, err := release.CreateGitHubReleases(result, &release.GitHubReleaseOptions{RepoPath: repoPath, ExistingTags: true})
	for _, rel := range created {
		fmt.Printf("  Created release %s\n", rel.TagName)
	}
	return err
}
