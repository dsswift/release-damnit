package commit

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/commitcheck"
	"github.com/dsswift/release-damnit/internal/config"
)

// twoPackages is a release config with packages api and web and a repo scope.
const twoPackages = `{
  "scopes": ["repo"],
  "packages": {
    "apps/api": {"component": "api"},
    "apps/web": {"component": "web"}
  }
}`

const twoManifest = `{"apps/api": "1.0.0", "apps/web": "1.0.0"}`

// stub returns its replies in order and records the prompts it got.
type stub struct {
	replies []Reply
	prompts []string
}

func (s *stub) Plan(_ context.Context, prompt string) (Reply, error) {
	s.prompts = append(s.prompts, prompt)
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}

func gitT(t *testing.T, dir string, args ...string) string {
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
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repo with the two-package config and one commit holding
// files in both packages.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "--initial-branch=main")
	gitT(t, dir, "config", "user.email", "user@example.com")
	gitT(t, dir, "config", "user.name", "Test")
	gitT(t, dir, "config", "core.excludesFile", os.DevNull)
	write(t, dir, config.ConfigFile, twoPackages)
	write(t, dir, config.ManifestFile, twoManifest)
	write(t, dir, "apps/api/server.go", "package api\n")
	write(t, dir, "apps/api/old.go", "package api\n// old\n")
	write(t, dir, "apps/web/index.ts", "export {}\n")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "chore(repo): seed")
	return dir
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, config.ConfigFile, twoPackages)
	write(t, dir, config.ManifestFile, twoManifest)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestParseStatus(t *testing.T) {
	out := " M a.go\x00M  b.go\x00R  new.go\x00old.go\x00?? n.txt\x00 D gone.go\x00!! ignored\x00"

	all, err := parseStatus(out, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Path: "a.go"}, {Path: "b.go"}, {Path: "new.go", OldPath: "old.go"}, {Path: "n.txt", Untracked: true}, {Path: "gone.go"}}
	if !reflect.DeepEqual(all, want) {
		t.Errorf("all = %+v, want %+v", all, want)
	}

	staged, err := parseStatus(out, true)
	if err != nil {
		t.Fatal(err)
	}
	want = []Change{{Path: "b.go"}, {Path: "new.go", OldPath: "old.go"}}
	if !reflect.DeepEqual(staged, want) {
		t.Errorf("staged = %+v, want %+v", staged, want)
	}

	if _, err := parseStatus("UU c.go\x00", false); err == nil {
		t.Error("a conflicted file must be an error")
	}
}

func TestSecretsAreSkipped(t *testing.T) {
	for _, p := range []string{"appsettings.json", "src/local.settings.json", "docker-compose.yml", ".env", "svc/.env.local", "credentials.json"} {
		if !IsSecret(p) {
			t.Errorf("%s must be secret", p)
		}
	}
	for _, p := range []string{"appsettings.Development.json", "docker-compose.yaml.md", "env", "my.env", "README.md"} {
		if IsSecret(p) {
			t.Errorf("%s must not be secret", p)
		}
	}

	keep, secret := splitSecrets([]Change{{Path: "a.go"}, {Path: "cfg/.env"}, {Path: "x.json", OldPath: "credentials.json"}})
	if len(keep) != 1 || keep[0].Path != "a.go" || len(secret) != 2 {
		t.Errorf("keep = %+v, secret = %+v", keep, secret)
	}
}

func TestPartition(t *testing.T) {
	cfg := loadConfig(t)
	p, err := partition(cfg, []Change{
		{Path: "apps/web/a.ts"},
		{Path: "README.md"},
		{Path: "apps/api/new.go", OldPath: "apps/api/old.go"},
		{Path: "apps/web/b.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Packages) != 2 || p.Packages[0].Package.Scope() != "api" || p.Packages[1].Package.Scope() != "web" {
		t.Fatalf("packages = %+v", p.Packages)
	}
	if len(p.Packages[1].Changes) != 2 || len(p.Loose) != 1 || p.Loose[0].Path != "README.md" {
		t.Errorf("partition = %+v", p)
	}

	if _, err := partition(cfg, []Change{{Path: "apps/web/x.go", OldPath: "apps/api/x.go"}}); err == nil {
		t.Error("a rename across packages must be an error")
	}

	none, err := partition(nil, []Change{{Path: "apps/api/x.go"}})
	if err != nil || len(none.Packages) != 0 || len(none.Loose) != 1 {
		t.Errorf("no config: %+v, %v", none, err)
	}
}

func testInput(t *testing.T) input {
	cfg := loadConfig(t)
	changes := []Change{{Path: "apps/api/a.go"}, {Path: "apps/web/b.ts"}, {Path: "README.md"}}
	parts, err := partition(cfg, changes)
	if err != nil {
		t.Fatal(err)
	}
	return input{rules: commitcheck.RulesFor(cfg), parts: parts, diffs: map[string]string{}}
}

func TestCheckReply(t *testing.T) {
	in := testInput(t)

	good := Reply{Commits: []PlannedCommit{
		{Message: "feat(api): add a", Files: []string{"apps/api/a.go", "README.md"}},
		{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
	}}
	if problems := in.check(good); len(problems) != 0 {
		t.Fatalf("good plan rejected: %v", problems)
	}

	cases := map[string]struct {
		reply Reply
		want  string
	}{
		"missing file": {Reply{Commits: []PlannedCommit{
			{Message: "feat(api): add a", Files: []string{"apps/api/a.go"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
		}}, `"README.md" is in no commit`},
		"duplicate": {Reply{Commits: []PlannedCommit{
			{Message: "feat(api): add a", Files: []string{"apps/api/a.go", "README.md"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts", "README.md"}},
		}}, "is in commit 1 and commit 2"},
		"unknown": {Reply{Commits: []PlannedCommit{
			{Message: "feat(api): add a", Files: []string{"apps/api/a.go", "README.md", "nope.go"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
		}}, `"nope.go", which is not a pending file`},
		"mixed packages": {Reply{Commits: []PlannedCommit{
			{Message: "feat(api): add a", Files: []string{"apps/api/a.go", "apps/web/b.ts", "README.md"}},
		}}, "mixes packages api, web"},
		"wrong scope": {Reply{Commits: []PlannedCommit{
			{Message: "feat(web): add a", Files: []string{"apps/api/a.go", "README.md"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
		}}, `scope "web" does not match`},
		"bad type and case": {Reply{Commits: []PlannedCommit{
			{Message: "refactor(api): Add a.", Files: []string{"apps/api/a.go", "README.md"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
		}}, `type "refactor" is not allowed`},
		"body": {Reply{Commits: []PlannedCommit{
			{Message: "feat(api): add a\n\nmore", Files: []string{"apps/api/a.go", "README.md"}},
			{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
		}}, "one header line"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			problems := strings.Join(in.check(tc.reply), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Errorf("problems %q do not contain %q", problems, tc.want)
			}
		})
	}
}

func TestCheckReplyNoConfigDropsScope(t *testing.T) {
	in := input{rules: commitcheck.RulesFor(nil), parts: Partition{Loose: []Change{{Path: "a.go"}}}}
	if p := in.check(Reply{Commits: []PlannedCommit{{Message: "feat: add a", Files: []string{"a.go"}}}}); len(p) != 0 {
		t.Errorf("no-scope commit rejected: %v", p)
	}
	if p := in.check(Reply{Commits: []PlannedCommit{{Message: "feat(x): add a", Files: []string{"a.go"}}}}); len(p) == 0 {
		t.Error("a scope in a repo with no packages must be rejected")
	}
}

func TestPlanRetriesOnceWithProblems(t *testing.T) {
	in := testInput(t)
	bad := Reply{Commits: []PlannedCommit{{Message: "feat(api): add a", Files: []string{"apps/api/a.go"}}}}
	good := Reply{Commits: []PlannedCommit{
		{Message: "feat(api): add a", Files: []string{"apps/api/a.go", "README.md"}},
		{Message: "fix(web): fix b", Files: []string{"apps/web/b.ts"}},
	}}

	s := &stub{replies: []Reply{bad, good}}
	reply, err := plan(context.Background(), s, in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reply, good) || len(s.prompts) != 2 {
		t.Fatalf("reply = %+v after %d calls", reply, len(s.prompts))
	}
	if !strings.Contains(s.prompts[1], `"apps/web/b.ts" is in no commit`) {
		t.Errorf("the retry prompt must carry the problems:\n%s", s.prompts[1])
	}

	s = &stub{replies: []Reply{bad, bad}}
	if _, err := plan(context.Background(), s, in); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("two bad plans must fail clearly, got %v", err)
	}
}

func TestPromptCarriesRules(t *testing.T) {
	in := testInput(t)
	p := in.prompt()
	for _, want := range []string{"feat, fix, chore, docs, feat!", "at most 50 characters", `Package api: its commits use the scope "api"`, "one of these scopes: repo", "- README.md"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

func files(t *testing.T, dir, rev string) []string {
	out := gitT(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "--no-renames", rev)
	return strings.Fields(out)
}

func TestRunCommitsEachGroupAlone(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "apps/api/server.go", "package api\n// changed\n")
	write(t, dir, "apps/api/handler.go", "package api\n")
	write(t, dir, "apps/web/index.ts", "export const x = 1\n")
	if err := os.Remove(filepath.Join(dir, "apps/api/old.go")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "apps/api/.env", "TOKEN=x\n")
	write(t, dir, "README.md", "readme\n")

	s := &stub{replies: []Reply{{Commits: []PlannedCommit{
		{Message: "feat(api): add the handler", Files: []string{"apps/api/handler.go", "apps/api/server.go", "apps/api/old.go"}},
		{Message: "fix(web): export x", Files: []string{"apps/web/index.ts", "README.md"}},
	}}}}
	var out bytes.Buffer
	if err := Run(context.Background(), Options{Dir: dir, Yes: true, Planner: s, Out: &out}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}

	if strings.Contains(s.prompts[0], "TOKEN") || strings.Contains(s.prompts[0], ".env") {
		t.Error("the secret file reached the prompt")
	}
	if !strings.Contains(out.String(), "apps/api/.env") {
		t.Errorf("the skipped secret is not listed:\n%s", out.String())
	}
	if got := gitT(t, dir, "log", "--format=%s", "-2"); got != "fix(web): export x\nfeat(api): add the handler" {
		t.Errorf("log = %q", got)
	}
	if got, want := files(t, dir, "HEAD~1"), []string{"apps/api/handler.go", "apps/api/old.go", "apps/api/server.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("api commit files = %v, want %v", got, want)
	}
	if got, want := files(t, dir, "HEAD"), []string{"README.md", "apps/web/index.ts"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web commit files = %v, want %v", got, want)
	}
	if got := gitT(t, dir, "status", "--porcelain", "--untracked-files=all"); got != "?? apps/api/.env" {
		t.Errorf("status after = %q", got)
	}
}

func TestRunStagedCommitsTheIndex(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "apps/api/server.go", "package api\n// staged\n")
	gitT(t, dir, "add", "apps/api/server.go")
	gitT(t, dir, "mv", "apps/api/old.go", "apps/api/renamed.go")
	write(t, dir, "apps/api/server.go", "package api\n// staged\n// not staged\n")
	write(t, dir, "apps/web/index.ts", "export const unstaged = 1\n")

	s := &stub{replies: []Reply{{Commits: []PlannedCommit{
		{Message: "fix(api): rename old", Files: []string{"apps/api/renamed.go", "apps/api/server.go"}},
	}}}}
	var out bytes.Buffer
	if err := Run(context.Background(), Options{Dir: dir, Staged: true, Yes: true, Planner: s, Out: &out}); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}

	if got := gitT(t, dir, "show", "HEAD:apps/api/server.go"); got != "package api\n// staged" {
		t.Errorf("committed server.go = %q", got)
	}
	if got, want := files(t, dir, "HEAD"), []string{"apps/api/old.go", "apps/api/renamed.go", "apps/api/server.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("commit files = %v, want %v", got, want)
	}
	if got := gitT(t, dir, "status", "--porcelain"); got != "M apps/api/server.go\n M apps/web/index.ts" {
		t.Errorf("status after = %q", got)
	}
}

func TestRunDryRunCommitsNothing(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "apps/web/index.ts", "export const x = 1\n")
	head := gitT(t, dir, "rev-parse", "HEAD")

	s := &stub{replies: []Reply{{Commits: []PlannedCommit{{Message: "fix(web): export x", Files: []string{"apps/web/index.ts"}}}}}}
	var out bytes.Buffer
	if err := Run(context.Background(), Options{Dir: dir, DryRun: true, Planner: s, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if gitT(t, dir, "rev-parse", "HEAD") != head || !strings.Contains(out.String(), "1. fix(web): export x") {
		t.Errorf("dry run changed HEAD or hid the plan:\n%s", out.String())
	}
}

func TestRunStopsOnAFailedCommit(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "apps/api/server.go", "package api\n// changed\n")
	write(t, dir, "apps/web/index.ts", "export const x = 1\n")
	hook := filepath.Join(dir, ".git", "hooks", "commit-msg")
	write(t, dir, ".git/hooks/commit-msg", "#!/bin/sh\ngrep -q web \"$1\" && exit 1\nexit 0\n")
	if err := os.Chmod(hook, 0o755); err != nil {
		t.Fatal(err)
	}

	s := &stub{replies: []Reply{{Commits: []PlannedCommit{
		{Message: "fix(api): change server", Files: []string{"apps/api/server.go"}},
		{Message: "fix(web): export x", Files: []string{"apps/web/index.ts"}},
	}}}}
	var out bytes.Buffer
	err := Run(context.Background(), Options{Dir: dir, Yes: true, Planner: s, Out: &out})
	if err == nil {
		t.Fatal("a rejected commit must fail the run")
	}
	if !strings.Contains(out.String(), "committed 1 of 2 before the failure") {
		t.Errorf("output does not report progress:\n%s", out.String())
	}
	if got := gitT(t, dir, "log", "-1", "--format=%s"); got != "fix(api): change server" {
		t.Errorf("HEAD = %q", got)
	}
}
