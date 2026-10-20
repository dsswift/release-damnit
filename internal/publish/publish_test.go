package publish

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/release"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, path, content, message string) string {
	t.Helper()
	write(t, dir, path, content)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", message)
	return run(t, dir, "rev-parse", "HEAD")
}

// ci is a bare remote plus a clone standing in for the CI checkout, and a
// second clone for racing pushes.
type ci struct {
	remote, checkout, other string
}

func newCI(t *testing.T, configJSON, manifestJSON string, files map[string]string) ci {
	t.Helper()
	root := t.TempDir()
	c := ci{remote: filepath.Join(root, "remote.git"), checkout: filepath.Join(root, "checkout"), other: filepath.Join(root, "other")}
	run(t, root, "init", "-q", "--bare", "--initial-branch=main", c.remote)
	run(t, root, "clone", "-q", c.remote, c.checkout)
	configure(t, c.checkout)
	write(t, c.checkout, "release-please-config.json", configJSON)
	write(t, c.checkout, "release-please-manifest.json", manifestJSON)
	for path, content := range files {
		write(t, c.checkout, path, content)
	}
	run(t, c.checkout, "add", "-A")
	run(t, c.checkout, "commit", "-q", "-m", "chore: initial")
	run(t, c.checkout, "push", "-q", "origin", "HEAD:refs/heads/main")
	run(t, root, "clone", "-q", c.remote, c.other)
	configure(t, c.other)
	return c
}

func configure(t *testing.T, dir string) {
	run(t, dir, "config", "user.email", "user@example.com")
	run(t, dir, "config", "user.name", "Test")
}

// analyze runs the release analysis and Apply in the checkout and writes the
// report, as the default command does with --report-file.
func analyze(t *testing.T, dir, since string) string {
	t.Helper()
	result, err := release.Analyze(&release.Options{RepoPath: dir, Since: since, TreatPreMajorAsMinor: true})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	data, err := json.Marshal(release.BuildReleaseReport(result, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

type fakeReleases struct {
	target string
	tags   []string
}

func (f *fakeReleases) create(result *release.AnalysisResult) error {
	f.target = result.MergeInfo.HeadSHA
	for _, rel := range result.Releases {
		f.tags = append(f.tags, rel.Package.TagName(rel.NewVersion))
	}
	return nil
}

func publish(t *testing.T, dir, report string, add ...string) (Outcome, *fakeReleases, string, error) {
	t.Helper()
	fake := &fakeReleases{}
	var out bytes.Buffer
	outcome, err := Run(Options{Dir: dir, ReportFile: report, Attempts: 5, Add: add, Out: &out, CreateReleases: fake.create})
	return outcome, fake, out.String(), err
}

const singleRepo = `{"include-component-in-tag": false, "packages": {".": {"component": "tool"}}}`

func singleRepoFiles() map[string]string {
	return map[string]string{"VERSION": "1.0.0\n", "CHANGELOG.md": "# Changelog\n", "main.go": "package main\n"}
}

func TestPublish_TagsTheVersionCommit(t *testing.T) {
	c := newCI(t, singleRepo, `{".": "1.0.0"}`, singleRepoFiles())
	feature := commit(t, c.checkout, "feature.go", "package main\n", "feat: add a feature")
	run(t, c.checkout, "push", "-q", "origin", "main")
	report := analyze(t, c.checkout, "")

	outcome, fake, out, err := publish(t, c.checkout, report)
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}

	remoteMain := run(t, c.remote, "rev-parse", "main")
	if outcome.Commit != remoteMain || !outcome.Released {
		t.Errorf("outcome %+v, remote main %s", outcome, remoteMain)
	}
	if subject := run(t, c.remote, "log", "-1", "--format=%s", "main"); subject != "chore: release v1.1.0 [skip ci]" {
		t.Errorf("version commit subject = %q", subject)
	}
	if parent := run(t, c.remote, "rev-parse", "main^"); parent != feature {
		t.Errorf("version commit parent = %s, want the feature commit %s", parent, feature)
	}
	if tagged := run(t, c.remote, "rev-parse", "v1.1.0^{commit}"); tagged != remoteMain {
		t.Errorf("v1.1.0 points at %s, want the version commit %s (not the pre-bump %s)", tagged, remoteMain, feature)
	}
	if kind := run(t, c.remote, "cat-file", "-t", "refs/tags/v1.1.0"); kind != "tag" {
		t.Errorf("v1.1.0 is a %s, want an annotated tag", kind)
	}
	if v := run(t, c.remote, "show", "v1.1.0:VERSION"); v != "1.1.0" {
		t.Errorf("VERSION at the tag = %q", v)
	}
	files := run(t, c.remote, "show", "--name-only", "--format=", "main")
	if files != "CHANGELOG.md\nVERSION\nrelease-please-manifest.json" {
		t.Errorf("version commit files =\n%s", files)
	}
	if fake.target != remoteMain || strings.Join(fake.tags, " ") != "v1.1.0" {
		t.Errorf("releases created for %v at %s, want v1.1.0 at %s", fake.tags, fake.target, remoteMain)
	}
}

func TestPublish_RetriesAndMovesTheTagsWhenTheBranchMoves(t *testing.T) {
	c := newCI(t, singleRepo, `{".": "1.0.0"}`, singleRepoFiles())
	commit(t, c.checkout, "feature.go", "package main\n", "fix: repair a thing")
	run(t, c.checkout, "push", "-q", "origin", "main")
	run(t, c.other, "pull", "-q", "origin", "main")
	racer := commit(t, c.other, "docs.md", "racing\n", "docs: land while ci runs")
	report := analyze(t, c.checkout, "")

	marker := filepath.Join(t.TempDir(), "raced")
	write(t, c.checkout, ".git/hooks/pre-push", `#!/bin/sh
if [ ! -f "`+marker+`" ]; then
  touch "`+marker+`"
  unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
  git -C "`+c.other+`" push -q origin HEAD:refs/heads/main >&2
fi
exit 0
`)

	outcome, fake, out, err := publish(t, c.checkout, report)
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if !strings.Contains(out, "retrying (attempt 2/5)") || !strings.Contains(out, "moved tags v1.0.1") {
		t.Errorf("output:\n%s", out)
	}
	remoteMain := run(t, c.remote, "rev-parse", "main")
	if parent := run(t, c.remote, "rev-parse", "main^"); parent != racer {
		t.Errorf("version commit parent = %s, want the racing commit %s", parent, racer)
	}
	if tagged := run(t, c.remote, "rev-parse", "v1.0.1^{commit}"); tagged != remoteMain || outcome.Commit != remoteMain || fake.target != remoteMain {
		t.Errorf("tag at %s, outcome %s, release target %s; want all at the rebased commit %s", tagged, outcome.Commit, fake.target, remoteMain)
	}
}

func TestPublish_RebaseConflictPushesNothing(t *testing.T) {
	c := newCI(t, singleRepo, `{".": "1.0.0"}`, singleRepoFiles())
	commit(t, c.checkout, "feature.go", "package main\n", "fix: repair")
	run(t, c.checkout, "push", "-q", "origin", "main")
	run(t, c.other, "pull", "-q", "origin", "main")
	report := analyze(t, c.checkout, "")
	// Another version commit lands first and rewrites the same files.
	commit(t, c.other, "VERSION", "9.9.9\n", "chore: release v9.9.9 [skip ci]")
	run(t, c.other, "push", "-q", "origin", "main")
	before := run(t, c.remote, "rev-parse", "main")

	_, _, out, err := publish(t, c.checkout, report)
	if err == nil || !strings.Contains(err.Error(), "conflicts") || !strings.Contains(err.Error(), "VERSION") {
		t.Errorf("err = %v\n%s", err, out)
	}
	if run(t, c.remote, "rev-parse", "main") != before {
		t.Error("remote moved")
	}
	if tags := run(t, c.remote, "tag"); tags != "" {
		t.Errorf("tags pushed: %s", tags)
	}
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(c.checkout, ".git", state)); !os.IsNotExist(err) {
			t.Errorf("the rebase was left in progress (%s)", state)
		}
	}
}

func TestPublish_AddsRepoStepFilesAndNamesEveryComponent(t *testing.T) {
	c := newCI(t, `{"packages": {"web": {"component": "web", "extra-files": [{"type": "json", "path": "package.json", "jsonpath": "$.version"}]}, "api": {"component": "api"}}}`,
		`{"web": "1.0.0", "api": "0.3.0"}`,
		map[string]string{"web/package.json": "{\n  \"version\": \"1.0.0\"\n}\n", "api/VERSION": "0.3.0\n", "package-lock.json": "{}\n", "untouched.txt": "x\n"})
	base := run(t, c.checkout, "rev-parse", "HEAD")
	commit(t, c.checkout, "web/a.js", "1\n", "feat(web): add a")
	commit(t, c.checkout, "api/b.go", "1\n", "fix(api): fix b")
	run(t, c.checkout, "push", "-q", "origin", "main")
	report := analyze(t, c.checkout, base)

	// A repo step (before-publish) refreshes a lock file; an unrelated edit stays out.
	write(t, c.checkout, "package-lock.json", "{\"lockfileVersion\": 3}\n")
	write(t, c.checkout, "untouched.txt", "local edit\n")

	_, _, out, err := publish(t, c.checkout, report, "package-lock.json")
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if subject := run(t, c.remote, "log", "-1", "--format=%s", "main"); subject != "chore: release api v0.3.1, web v1.1.0 [skip ci]" {
		t.Errorf("subject = %q", subject)
	}
	files := run(t, c.remote, "show", "--name-only", "--format=", "main")
	want := "api/CHANGELOG.md\napi/VERSION\npackage-lock.json\nrelease-please-manifest.json\nweb/CHANGELOG.md\nweb/package.json"
	if files != want {
		t.Errorf("version commit files =\n%s\nwant\n%s", files, want)
	}
	for _, tag := range []string{"api-v0.3.1", "web-v1.1.0"} {
		if run(t, c.remote, "rev-parse", tag+"^{commit}") != run(t, c.remote, "rev-parse", "main") {
			t.Errorf("%s is not on the version commit", tag)
		}
	}
}

func TestPublish_NoReleaseIsANoOp(t *testing.T) {
	c := newCI(t, singleRepo, `{".": "1.0.0"}`, singleRepoFiles())
	commit(t, c.checkout, "notes.md", "x\n", "docs: notes")
	run(t, c.checkout, "push", "-q", "origin", "main")
	before := run(t, c.remote, "rev-parse", "main")
	report := analyze(t, c.checkout, "")

	outcome, fake, out, err := publish(t, c.checkout, report)
	if err != nil || outcome.Released || !strings.Contains(out, "nothing to publish") {
		t.Errorf("outcome %+v, err %v\n%s", outcome, err, out)
	}
	if run(t, c.remote, "rev-parse", "main") != before || fake.target != "" {
		t.Error("something was pushed or released")
	}

	output := filepath.Join(t.TempDir(), "github_output")
	if err := WriteGitHubOutput(output, outcome); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(output); string(data) != "released=false\n" {
		t.Errorf("GITHUB_OUTPUT = %q", data)
	}
}

func TestPublish_RefusesAnExistingTag(t *testing.T) {
	c := newCI(t, singleRepo, `{".": "1.0.0"}`, singleRepoFiles())
	commit(t, c.checkout, "feature.go", "package main\n", "feat: add")
	run(t, c.checkout, "tag", "v1.1.0")
	report := analyze(t, c.checkout, "")
	head := run(t, c.checkout, "rev-parse", "HEAD")

	if _, _, _, err := publish(t, c.checkout, report); err == nil || !strings.Contains(err.Error(), "tag v1.1.0 already exists") {
		t.Errorf("err = %v", err)
	}
	if run(t, c.checkout, "rev-parse", "HEAD") != head {
		t.Error("a version commit was made before the tag check")
	}
}

func TestWriteGitHubOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	err := WriteGitHubOutput(path, Outcome{Released: true, Commit: "abc", Tags: []Tag{
		{Name: "api-v0.3.1", Component: "api", Version: "0.3.1"},
		{Name: "web-v1.1.0", Component: "web", Version: "1.1.0"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "released=true\ntag=api-v0.3.1\nversion=0.3.1\ntags=api-v0.3.1 web-v1.1.0\napi--tag_name=api-v0.3.1\napi--version=0.3.1\nweb--tag_name=web-v1.1.0\nweb--version=1.1.0\n"
	if string(data) != want {
		t.Errorf("got\n%s\nwant\n%s", data, want)
	}
}
