package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// GitLabReleaseOptions configures GitLab Releases API calls. Token is a project
// access token (api scope) or CI job token accepted by the target GitLab instance.
type GitLabReleaseOptions struct {
	RepoPath, Token string
	DryRun          bool
}
type GitLabRelease struct {
	TagName, Title, Notes, TargetSHA string
	PackageInfo                      *PackageRelease
}

func DetectProvider(requested, repoURL string) string {
	if requested == "github" || requested == "gitlab" {
		return requested
	}
	if strings.Contains(strings.ToLower(repoURL), "gitlab") || os.Getenv("GITLAB_CI") != "" {
		return "gitlab"
	}
	return "github"
}
func BuildGitLabRelease(rel *PackageRelease, repoURL string) *GitLabRelease {
	return &GitLabRelease{TagName: fmt.Sprintf("%s-v%s", rel.Package.Component, rel.NewVersion), Title: fmt.Sprintf("%s v%s", rel.Package.Component, rel.NewVersion), Notes: BuildReleaseNotes(rel, repoURL), PackageInfo: rel}
}
func CreateGitLabReleases(result *AnalysisResult, opts *GitLabReleaseOptions) ([]*GitLabRelease, error) {
	if opts == nil {
		opts = &GitLabReleaseOptions{}
	}
	api, project := os.Getenv("CI_API_V4_URL"), os.Getenv("CI_PROJECT_ID")
	if api == "" || project == "" {
		return nil, fmt.Errorf("GitLab release creation requires CI_API_V4_URL and CI_PROJECT_ID")
	}
	if opts.Token == "" {
		opts.Token = os.Getenv("CI_JOB_TOKEN")
	}
	if opts.Token == "" {
		return nil, fmt.Errorf("GitLab release creation requires GITLAB_TOKEN or CI_JOB_TOKEN")
	}
	releases := make([]*GitLabRelease, 0, len(result.Releases))
	for _, item := range result.Releases {
		rel := BuildGitLabRelease(item, result.RepoURL)
		rel.TargetSHA = result.MergeInfo.HeadSHA
		if !opts.DryRun {
			if err := executeGitLabRelease(api, project, opts.Token, rel); err != nil {
				return releases, err
			}
		}
		releases = append(releases, rel)
	}
	return releases, nil
}
func executeGitLabRelease(api, project, token string, rel *GitLabRelease) error {
	body, err := json.Marshal(map[string]string{"name": rel.Title, "tag_name": rel.TagName, "description": rel.Notes, "ref": rel.TargetSHA})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(api, "/") + "/projects/" + url.PathEscape(project) + "/releases"
	cmd := exec.Command("curl", "--fail-with-body", "--request", "POST", "--header", "PRIVATE-TOKEN: "+token, "--header", "Content-Type: application/json", "--data-binary", "@-", endpoint)
	cmd.Stdin = bytes.NewReader(body)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create GitLab release %s: %w", rel.TagName, err)
	}
	return nil
}
