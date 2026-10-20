package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// GitLabReleaseOptions configures GitLab Releases API calls. A project access
// token uses PRIVATE-TOKEN. A CI job token uses JOB-TOKEN.
type GitLabReleaseOptions struct {
	RepoPath    string
	Token       string
	UseJobToken bool
	DryRun      bool
	APIURL      string
	ProjectID   string
	HTTPClient  *http.Client
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
	return &GitLabRelease{TagName: rel.Package.TagName(rel.NewVersion), Title: fmt.Sprintf("%s v%s", rel.Package.Component, rel.NewVersion), Notes: BuildReleaseNotes(rel, repoURL), PackageInfo: rel}
}

func CreateGitLabReleases(result *AnalysisResult, opts *GitLabReleaseOptions) ([]*GitLabRelease, error) {
	if opts == nil {
		opts = &GitLabReleaseOptions{}
	}
	api, project := opts.APIURL, opts.ProjectID
	if api == "" {
		api = os.Getenv("CI_API_V4_URL")
	}
	if project == "" {
		project = os.Getenv("CI_PROJECT_ID")
	}
	if api == "" || project == "" {
		return nil, fmt.Errorf("GitLab release creation requires CI_API_V4_URL and CI_PROJECT_ID")
	}
	if opts.Token == "" {
		opts.Token = os.Getenv("CI_JOB_TOKEN")
		opts.UseJobToken = true
	}
	if opts.Token == "" {
		return nil, fmt.Errorf("GitLab release creation requires GITLAB_RELEASE_TOKEN or CI_JOB_TOKEN")
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	releases := make([]*GitLabRelease, 0, len(result.Releases))
	for _, item := range result.Releases {
		rel := BuildGitLabRelease(item, result.RepoURL)
		rel.TargetSHA = result.MergeInfo.HeadSHA
		if !opts.DryRun {
			if err := executeGitLabRelease(client, api, project, opts.Token, opts.UseJobToken, rel); err != nil {
				return releases, err
			}
		}
		releases = append(releases, rel)
	}
	return releases, nil
}

func gitLabReleaseEndpoint(api, project string) string {
	return strings.TrimRight(api, "/") + "/projects/" + url.PathEscape(project) + "/releases"
}

func executeGitLabRelease(client *http.Client, api, project, token string, useJobToken bool, rel *GitLabRelease) error {
	body, err := json.Marshal(map[string]string{"name": rel.Title, "tag_name": rel.TagName, "description": rel.Notes, "ref": rel.TargetSHA})
	if err != nil {
		return fmt.Errorf("marshal GitLab release %s: %w", rel.TagName, err)
	}
	req, err := http.NewRequest(http.MethodPost, gitLabReleaseEndpoint(api, project), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build GitLab release request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if useJobToken {
		req.Header.Set("JOB-TOKEN", token)
	} else {
		req.Header.Set("PRIVATE-TOKEN", token)
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("create GitLab release %s: %w", rel.TagName, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("create GitLab release %s: API returned %s", rel.TagName, response.Status)
	}
	return nil
}
