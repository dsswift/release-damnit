package release

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
)

func TestDetectProvider(t *testing.T) {
	for _, tc := range []struct{ requested, url, want string }{{"auto", "https://gitlab.example/group/app", "gitlab"}, {"auto", "https://github.com/example/app", "github"}, {"gitlab", "https://github.com/example/app", "gitlab"}} {
		if got := DetectProvider(tc.requested, tc.url); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}
func TestBuildGitLabRelease(t *testing.T) {
	got := BuildGitLabRelease(&PackageRelease{Package: &config.Package{Component: "api"}, NewVersion: "1.2.3"}, "https://gitlab.example/group/app")
	if got.TagName != "api-v1.2.3" || got.Title != "api v1.2.3" {
		t.Fatalf("unexpected release: %#v", got)
	}
}
func gitLabResult() *AnalysisResult {
	return &AnalysisResult{RepoURL: "https://gitlab.example/group/app", MergeInfo: &git.MergeInfo{HeadSHA: "abc123"}, Releases: []*PackageRelease{{Package: &config.Package{Component: "api", Path: "workloads/api"}, NewVersion: "1.2.3"}}}
}
func TestGitLabReleaseProjectTokenRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fapp/releases" {
			t.Errorf("path %q", r.URL.EscapedPath())
		}
		if r.Header.Get("PRIVATE-TOKEN") != "project-token" || r.Header.Get("JOB-TOKEN") != "" {
			t.Errorf("headers %#v", r.Header)
		}
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["tag_name"] != "api-v1.2.3" || payload["ref"] != "abc123" || payload["name"] != "api v1.2.3" {
			t.Errorf("payload %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	_, err := CreateGitLabReleases(gitLabResult(), &GitLabReleaseOptions{APIURL: server.URL + "/api/v4", ProjectID: "group/app", Token: "project-token"})
	if err != nil {
		t.Fatal(err)
	}
}
func TestGitLabReleaseNumericProjectAndJobToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects/123/releases" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.Header.Get("JOB-TOKEN") != "job-token" || r.Header.Get("PRIVATE-TOKEN") != "" {
			t.Errorf("headers %#v", r.Header)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	_, err := CreateGitLabReleases(gitLabResult(), &GitLabReleaseOptions{APIURL: server.URL + "/api", ProjectID: "123", Token: "job-token", UseJobToken: true})
	if err != nil {
		t.Fatal(err)
	}
}
func TestGitLabReleaseAPIErrorIsFatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	if _, err := CreateGitLabReleases(gitLabResult(), &GitLabReleaseOptions{APIURL: server.URL, ProjectID: "1", Token: "bad"}); err == nil {
		t.Fatal("expected API error")
	}
}
func TestGitLabReleaseDryRunDoesNotCallAPI(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusCreated) }))
	defer server.Close()
	releases, err := CreateGitLabReleases(gitLabResult(), &GitLabReleaseOptions{APIURL: server.URL, ProjectID: "1", Token: "token", DryRun: true})
	if err != nil || called || len(releases) != 1 {
		t.Fatalf("releases=%d called=%t err=%v", len(releases), called, err)
	}
}
