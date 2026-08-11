package release

import (
	"github.com/dsswift/release-damnit/internal/config"
	"testing"
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
