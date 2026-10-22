package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binary is the release-damnit binary built once for the CLI tests.
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "release-damnit-cli-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "release-damnit")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// cli runs the binary in dir and returns its combined output and exit code.
func cli(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return string(out), 0
}

func gitRun(t *testing.T, dir string, args ...string) string {
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

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "--initial-branch=main")
	gitRun(t, dir, "config", "user.email", "user@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	return dir
}

func TestCLI_CheckCommit(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "release-please-config.json", `{"packages": {"a": {"component": "a"}, "b": {"component": "b"}}}`)
	write(t, dir, "release-please-manifest.json", `{}`)
	write(t, dir, "a/x", "1\n")
	write(t, dir, "b/y", "1\n")
	gitRun(t, dir, "add", "-A")

	write(t, dir, ".git/MSG", "feat(a): add x\n")
	out, code := cli(t, dir, "check-commit", ".git/MSG")
	if code != 1 || !strings.Contains(out, "release-damnit check-commit: this commit changes 2 packages; split into one commit per package:") || !strings.Contains(out, "    b: b/y") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}

	gitRun(t, dir, "reset", "-q", "b/y")
	if out, code := cli(t, dir, "check-commit", ".git/MSG"); code != 0 || out != "" {
		t.Errorf("good commit: exit %d, output %q", code, out)
	}

	if out, code := cli(t, t.TempDir(), "check-commit", "MSG"); code != 1 || !strings.Contains(out, "not inside a git repository") {
		t.Errorf("outside a repo: exit %d, output %q", code, out)
	}
	if _, code := cli(t, dir, "check-commit"); code != 1 {
		t.Errorf("missing msgfile: exit %d", code)
	}
}

func TestCLI_CheckCommits(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "x", "1\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "chore: initial")
	base := gitRun(t, dir, "rev-parse", "HEAD")
	write(t, dir, "x", "2\n")
	gitRun(t, dir, "commit", "-qam", "feat: Bad one.")
	write(t, dir, "x", "3\n")
	gitRun(t, dir, "commit", "-qam", "fix: fine")
	bad := gitRun(t, dir, "rev-parse", "--short=9", "HEAD~1")

	out, code := cli(t, dir, "check-commits", base+"..HEAD")
	if code != 1 || !strings.Contains(out, "release-damnit check-commit: "+bad+" feat: Bad one.") || !strings.Contains(out, "1 commit(s) break the commit rules") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
	if out, code := cli(t, dir, "check-commits", "HEAD~1..HEAD"); code != 0 {
		t.Errorf("clean range: exit %d, output %s", code, out)
	}
}

func TestCLI_UnknownFirstArgumentRunsTheDefaultCommand(t *testing.T) {
	out, code := cli(t, t.TempDir(), "--version")
	// A test binary carries no ldflags, so the version comes from build info.
	if code != 0 || !strings.HasPrefix(out, "release-damnit ") || strings.Contains(out, "dev (unknown)") {
		t.Errorf("--version: exit %d, output %q", code, out)
	}
	out, code = cli(t, t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "check-commit <msgfile>") || !strings.Contains(out, "--create-releases") {
		t.Errorf("--help: exit %d, output %q", code, out)
	}
}

func TestCLI_Land(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	gitRun(t, root, "init", "-q", "--bare", "--initial-branch=main", remote)
	clone := filepath.Join(root, "clone")
	gitRun(t, root, "clone", "-q", remote, clone)
	gitRun(t, clone, "config", "user.email", "user@example.com")
	gitRun(t, clone, "config", "user.name", "Test")
	write(t, clone, "a.txt", "a\n")
	gitRun(t, clone, "add", "-A")
	gitRun(t, clone, "commit", "-qm", "feat: add a")
	gitRun(t, clone, "push", "-q", "origin", "main")
	write(t, clone, "b.txt", "b\n")
	gitRun(t, clone, "add", "-A")
	gitRun(t, clone, "commit", "-qm", "fix: add b")

	out, code := cli(t, clone, "land", "--attempts", "2")
	if code != 0 || !strings.Contains(out, "release-damnit land: attempt 1/2: fetching origin/main") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
	if gitRun(t, remote, "rev-parse", "main") != gitRun(t, clone, "rev-parse", "HEAD") {
		t.Error("not landed")
	}
	if _, code := cli(t, clone, "land", "-h"); code != 0 {
		t.Errorf("-h exit %d", code)
	}
	if out, code := cli(t, clone, "land", "main"); code != 1 || !strings.Contains(out, "release-damnit land: main cannot be landed on itself") {
		t.Errorf("exit %d, output %s", code, out)
	}
}

// TestCLI_HookBlocksABadCommit installs the hook and commits through git, so
// the whole path from git to the check runs.
func TestCLI_HookBlocksABadCommit(t *testing.T) {
	dir := newRepo(t)
	if out, code := cli(t, dir, "hooks"); code != 0 || !strings.Contains(out, "installed the commit-msg hook") {
		t.Fatalf("hooks: exit %d, %s", code, out)
	}
	write(t, dir, "x.txt", "x\n")
	gitRun(t, dir, "add", "-A")

	commit := func(message string) (string, error) {
		cmd := exec.Command("git", "commit", "-q", "-m", message)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := commit("Added x."); err == nil || !strings.Contains(out, "release-damnit check-commit:") {
		t.Errorf("bad message committed: %v %s", err, out)
	}
	if out, err := commit("feat: add x"); err != nil {
		t.Errorf("good message refused: %v %s", err, out)
	}
	if out, code := cli(t, dir, "hooks"); code != 0 || !strings.Contains(out, "already in place") {
		t.Errorf("second run: exit %d, %s", code, out)
	}
}

func TestCLI_PublishNeedsAReport(t *testing.T) {
	if out, code := cli(t, t.TempDir(), "publish"); code != 2 || !strings.Contains(out, "--report-file") {
		t.Errorf("exit %d, output %s", code, out)
	}
}

func TestCLI_Init(t *testing.T) {
	dir := newRepo(t)
	out, code := cli(t, dir, "init", "--component", "tool", "--version", "1.2.3")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	for _, want := range []string{"release-damnit init: tool at 1.2.3", "wrote .github/workflows/release.yml", "installed the commit-msg hook", ".github/workflows/build.yml"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if out, code := cli(t, dir, "init"); code != 1 || !strings.Contains(out, "files already exist") {
		t.Errorf("second init: exit %d\n%s", code, out)
	}
	if out, code := cli(t, dir, "init", "--forge", "bitbucket"); code != 1 || !strings.Contains(out, "use github or gitlab") {
		t.Errorf("unknown forge: exit %d\n%s", code, out)
	}
	lab := newRepo(t)
	out, code = cli(t, lab, "init", "--forge", "gitlab", "--version", "0.1.0")
	if code != 0 {
		t.Fatalf("gitlab: exit %d:\n%s", code, out)
	}
	for _, want := range []string{"wrote .gitlab-ci.yml", "wrote .gitlab/build.yml", "GITLAB_RELEASE_TOKEN", "installed the commit-msg hook"} {
		if !strings.Contains(out, want) {
			t.Errorf("gitlab output lacks %q:\n%s", want, out)
		}
	}
	if out, code := cli(t, dir, "init", "--extra-file", "pom.xml"); code != 1 || !strings.Contains(out, "--extra-file") {
		t.Errorf("bad extra file: exit %d\n%s", code, out)
	}
}

func TestCLI_Commit(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "a\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-qm", "feat: add a")

	if out, code := cli(t, dir, "commit", "--yes"); code != 0 || !strings.Contains(out, "release-damnit commit: nothing to commit") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
	if out, code := cli(t, dir, "commit", "-h"); code != 0 || !strings.Contains(out, "-staged") {
		t.Errorf("-h exit %d, output:\n%s", code, out)
	}
	if _, code := cli(t, dir, "commit", "extra"); code != 2 {
		t.Errorf("an argument must exit 2, got %d", code)
	}
}
