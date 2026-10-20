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

func git(t *testing.T, dir string, args ...string) string {
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
	git(t, dir, "init", "-q", "--initial-branch=main")
	git(t, dir, "config", "user.email", "user@example.com")
	git(t, dir, "config", "user.name", "Test")
	return dir
}

func TestCLI_CheckCommit(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "release-please-config.json", `{"packages": {"a": {"component": "a"}, "b": {"component": "b"}}}`)
	write(t, dir, "release-please-manifest.json", `{}`)
	write(t, dir, "a/x", "1\n")
	write(t, dir, "b/y", "1\n")
	git(t, dir, "add", "-A")

	write(t, dir, ".git/MSG", "feat(a): add x\n")
	out, code := cli(t, dir, "check-commit", ".git/MSG")
	if code != 1 || !strings.Contains(out, "release-damnit check-commit: this commit changes 2 packages; split into one commit per package:") || !strings.Contains(out, "    b: b/y") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}

	git(t, dir, "reset", "-q", "b/y")
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
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "chore: initial")
	base := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "x", "2\n")
	git(t, dir, "commit", "-qam", "feat: Bad one.")
	write(t, dir, "x", "3\n")
	git(t, dir, "commit", "-qam", "fix: fine")
	bad := git(t, dir, "rev-parse", "--short=9", "HEAD~1")

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
	if code != 0 || !strings.HasPrefix(out, "release-damnit dev") {
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
	git(t, root, "init", "-q", "--bare", "--initial-branch=main", remote)
	clone := filepath.Join(root, "clone")
	git(t, root, "clone", "-q", remote, clone)
	git(t, clone, "config", "user.email", "user@example.com")
	git(t, clone, "config", "user.name", "Test")
	write(t, clone, "a.txt", "a\n")
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-qm", "feat: add a")
	git(t, clone, "push", "-q", "origin", "main")
	write(t, clone, "b.txt", "b\n")
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-qm", "fix: add b")

	out, code := cli(t, clone, "land", "--attempts", "2")
	if code != 0 || !strings.Contains(out, "release-damnit land: attempt 1/2: fetching origin/main") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
	if git(t, remote, "rev-parse", "main") != git(t, clone, "rev-parse", "HEAD") {
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
	git(t, dir, "add", "-A")

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
