package commitcheck

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/git"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// newMonorepo makes a repo with packages web and api and one commit.
func newMonorepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "--initial-branch=main")
	run(t, dir, "config", "user.email", "user@example.com")
	run(t, dir, "config", "user.name", "Test")
	write(t, dir, "release-please-config.json", `{"scopes": ["repo"], "packages": {"apps/web": {"component": "web"}, "apps/api": {"component": "api"}}}`)
	write(t, dir, "release-please-manifest.json", `{"apps/web": "1.0.0", "apps/api": "1.0.0"}`)
	write(t, dir, "apps/web/a.txt", "a\n")
	write(t, dir, "apps/api/b.txt", "b\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", "chore(repo): initial commit")
	return dir
}

func checkStaged(t *testing.T, dir, message string) Result {
	t.Helper()
	msg := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	write(t, filepath.Dir(msg), "COMMIT_EDITMSG", message)
	result, err := CheckStaged(dir, msg)
	if err != nil {
		t.Fatalf("CheckStaged: %v", err)
	}
	return result
}

func TestCheckStaged_CountsDeletionsAndBothSidesOfRenames(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := newMonorepo(t)

	run(t, dir, "rm", "-q", "apps/api/b.txt")
	if r := checkStaged(t, dir, "fix(web): drop b\n"); r.OK() {
		t.Error("a staged deletion under api passed as a web commit")
	}
	if r := checkStaged(t, dir, "fix(api): drop b\n"); !r.OK() {
		t.Errorf("deletion scoped to its package failed: %q", r.Problems)
	}
	run(t, dir, "reset", "-q", "--hard")

	// A move from api into web touches both packages.
	if err := os.Rename(filepath.Join(dir, "apps/api/b.txt"), filepath.Join(dir, "apps/web/b.txt")); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "-A")
	r := checkStaged(t, dir, "feat(web): take b\n")
	if r.OK() || !strings.Contains(r.Problems[0], "api: apps/api/b.txt") || !strings.Contains(r.Problems[0], "web: apps/web/b.txt") {
		t.Errorf("rename across packages: problems = %q", r.Problems)
	}
}

func TestCheckStaged_SubdirectoryAndNoConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := newMonorepo(t)
	write(t, dir, "apps/web/a.txt", "changed\n")
	run(t, dir, "add", "-A")
	result, err := CheckStaged(filepath.Join(dir, "apps/api"), writeMsg(t, "fix(web): change a\n"))
	if err != nil || !result.OK() {
		t.Errorf("from a subdirectory: %v %q", err, result.Problems)
	}

	plain := t.TempDir()
	run(t, plain, "init", "--initial-branch=main")
	write(t, plain, "x.txt", "x\n")
	run(t, plain, "add", "-A")
	if r, err := CheckStaged(plain, writeMsg(t, "feat(whatever): add x\n")); err != nil || !r.OK() {
		t.Errorf("no config: %v %q", err, r.Problems)
	}
	if r, _ := CheckStaged(plain, writeMsg(t, "feat: Add x\n")); r.OK() {
		t.Error("no config still enforces the header rules")
	}
}

func TestCheckStaged_OutsideARepository(t *testing.T) {
	_, err := CheckStaged(t.TempDir(), writeMsg(t, "feat: x\n"))
	if !errors.Is(err, git.ErrNotRepository) {
		t.Errorf("err = %v, want ErrNotRepository", err)
	}
}

func writeMsg(t *testing.T, message string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "MSG", message)
	return filepath.Join(dir, "MSG")
}

func TestCheckRange(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := newMonorepo(t)
	base := run(t, dir, "rev-parse", "HEAD")

	write(t, dir, "apps/web/a.txt", "1\n")
	run(t, dir, "commit", "-qam", "feat(web): good")
	write(t, dir, "apps/web/a.txt", "2\n")
	write(t, dir, "apps/api/b.txt", "2\n")
	run(t, dir, "commit", "-qam", "fix(web): touches both")
	run(t, dir, "rm", "-q", "apps/api/b.txt")
	run(t, dir, "commit", "-qm", "fix(web): deletes from api")
	run(t, dir, "checkout", "-qb", "side", base)
	write(t, dir, "README.md", "x\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "docs(repo): readme")
	run(t, dir, "checkout", "-q", "main")
	run(t, dir, "merge", "-q", "--no-ff", "--no-edit", "side")

	results, err := CheckRange(dir, base+"..HEAD")
	if err != nil {
		t.Fatalf("CheckRange: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("checked %d commits, want the 4 non-merge commits", len(results))
	}
	failed := Failures(results)
	var headers []string
	for _, f := range failed {
		headers = append(headers, f.Header)
	}
	if strings.Join(headers, "|") != "fix(web): touches both|fix(web): deletes from api" {
		t.Errorf("failures = %v", headers)
	}

	// The root commit's files count too: it adds both packages at once.
	all, err := CheckRange(dir, "HEAD")
	if err != nil || len(all) != 5 {
		t.Fatalf("CheckRange(HEAD) = %d results, %v", len(all), err)
	}
	for _, r := range all {
		if r.Header == "chore(repo): initial commit" && (r.OK() || !strings.Contains(r.Problems[0], "split into one commit per package")) {
			t.Errorf("root commit: problems = %q, want the split problem", r.Problems)
		}
	}
}

func TestLoadRules_ReadsVantageYAML(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := newMonorepo(t)
	if rules, _, err := LoadRules(dir); err != nil || rules.Work.On() {
		t.Fatalf("no vantage.yaml: work = %+v, %v; want off", rules.Work, err)
	}

	write(t, dir, VantageFile, "hub: https://hub.example.com\nworkspace: app\nroadmap: APP   # roadmap items\nwork: APPW\n")
	rules, _, err := LoadRules(filepath.Join(dir, "apps/web"))
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if rules.Work != (Tracking{Kind: "APPW", Roadmap: "APP"}) {
		t.Errorf("work = %+v, want APPW and APP", rules.Work)
	}
	if rules.Config == nil || len(rules.Config.Packages) != 2 {
		t.Error("the release config was not loaded beside vantage.yaml")
	}

	write(t, dir, VantageFile, "work: not a kind\n")
	if _, _, err := LoadRules(dir); err == nil || !strings.Contains(err.Error(), "must be a kind key") {
		t.Errorf("bad kind: err = %v", err)
	}
}

func TestCheckRange_Work(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	dir := newMonorepo(t)
	write(t, dir, VantageFile, "roadmap: APP\nwork: APPW\n")
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-qm", "chore(repo): track work\n\nWork: APP-1")
	base := run(t, dir, "rev-parse", "HEAD")

	write(t, dir, "apps/web/a.txt", "1\n")
	run(t, dir, "commit", "-qam", "feat(web): good\n\nWork: APPW-15")
	write(t, dir, "apps/web/a.txt", "2\n")
	run(t, dir, "commit", "-qam", "fix(web): no trailer")
	run(t, dir, "checkout", "-qb", "side", base)
	write(t, dir, "apps/api/b.txt", "2\n")
	run(t, dir, "commit", "-qam", "fix(api): other kind\n\nWork: WEB-3")
	run(t, dir, "checkout", "-q", "main")
	run(t, dir, "merge", "-q", "--no-ff", "--no-edit", "side")

	results, err := CheckRange(dir, base+"..HEAD")
	if err != nil {
		t.Fatalf("CheckRange: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("checked %d commits, want the 3 non-merge commits", len(results))
	}
	var headers []string
	for _, f := range Failures(results) {
		headers = append(headers, f.Header)
	}
	if strings.Join(headers, "|") != "fix(web): no trailer|fix(api): other kind" {
		t.Errorf("failures = %v", headers)
	}

	write(t, dir, "apps/web/a.txt", "3\n")
	run(t, dir, "add", "-A")
	if r := checkStaged(t, dir, "fix(web): staged\n\nWork: APPW-16\n"); !r.OK() {
		t.Errorf("staged with a trailer: problems = %q", r.Problems)
	}
	if r := checkStaged(t, dir, "fix(web): staged\n"); r.OK() {
		t.Error("staged with no trailer passed")
	}
}
