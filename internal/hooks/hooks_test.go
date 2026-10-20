package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstall_PlainGitHook(t *testing.T) {
	dir := newRepo(t)
	result, err := Install(filepath.Join(dir), false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(dir, ".git", "hooks", "commit-msg")
	if resolved, _ := filepath.EvalSymlinks(result.Path); resolved != mustEval(t, want) || !result.Changed || result.Husky {
		t.Errorf("result = %+v, want a new git hook at %s", result, want)
	}
	if got := read(t, want); got != Script {
		t.Errorf("hook = %q", got)
	}
	if info, _ := os.Stat(want); info.Mode().Perm()&0111 == 0 {
		t.Error("hook is not executable")
	}

	again, err := Install(dir, false)
	if err != nil || again.Changed {
		t.Errorf("second install: %+v, %v; want unchanged", again, err)
	}
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestInstall_ScriptReportsAMissingBinary(t *testing.T) {
	dir := newRepo(t)
	result, err := Install(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", result.Path, "MSG")
	cmd.Env = []string{"PATH=/nonexistent"}
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "release-damnit is not installed: go install github.com/dsswift/release-damnit/cmd/release-damnit@latest") {
		t.Errorf("err %v, output %q", err, out)
	}
}

func TestInstall_RefusesAForeignHookUnlessForced(t *testing.T) {
	dir := newRepo(t)
	path := filepath.Join(dir, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := Install(dir, false)
	if !errors.Is(err, ErrForeignHook) || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want ErrForeignHook naming %s", err, path)
	}
	if read(t, path) != "#!/bin/sh\nexit 0\n" {
		t.Error("foreign hook was changed")
	}
	if _, err := Install(dir, true); err != nil || read(t, path) != Script {
		t.Errorf("--force: %v", err)
	}
}

func TestInstall_ReplacesAnOlderReleaseDamnitHook(t *testing.T) {
	dir := newRepo(t)
	path := filepath.Join(dir, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+marker+"\nold\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if result, err := Install(dir, false); err != nil || !result.Changed || read(t, path) != Script {
		t.Errorf("result %+v, err %v", result, err)
	}
}

func TestInstall_LinkedWorktree(t *testing.T) {
	dir := newRepo(t)
	for _, args := range [][]string{
		{"-c", "user.email=user@example.com", "-c", "user.name=Test", "commit", "-q", "--allow-empty", "-m", "chore: initial"},
		{"worktree", "add", "-q", filepath.Join(dir, "wt")},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	result, err := Install(filepath.Join(dir, "wt"), false)
	if err != nil {
		t.Fatal(err)
	}
	if mustEval(t, result.Path) != mustEval(t, filepath.Join(dir, ".git", "hooks", "commit-msg")) {
		t.Errorf("hook path %s, want the shared hooks directory", result.Path)
	}
}

func TestInstall_Husky(t *testing.T) {
	dir := newRepo(t)
	husky := filepath.Join(dir, ".husky")
	if err := os.Mkdir(husky, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(husky, "commit-msg")
	original := "#!/usr/bin/env sh\n. \"$(dirname -- \"$0\")/_/husky.sh\"\n\nnpx --no -- commitlint --edit $1\necho done\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Install(dir, false)
	if err != nil || !result.Husky || !result.Changed {
		t.Fatalf("result %+v, err %v", result, err)
	}
	want := "#!/usr/bin/env sh\n. \"$(dirname -- \"$0\")/_/husky.sh\"\n\necho done\n" + CheckLine + "\n"
	if got := read(t, path); got != want {
		t.Errorf("husky hook =\n%s\nwant\n%s", got, want)
	}
	if again, err := Install(dir, false); err != nil || again.Changed || read(t, path) != want {
		t.Errorf("second install changed the file: %+v %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "hooks", "commit-msg")); !os.IsNotExist(err) {
		t.Error("a git hook was written in a husky repo")
	}
}

func TestInstall_HuskyWithoutACommitMsgFile(t *testing.T) {
	dir := newRepo(t)
	if err := os.Mkdir(filepath.Join(dir, ".husky"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(dir, false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, ".husky", "commit-msg")); got != CheckLine+"\n" {
		t.Errorf("hook = %q", got)
	}
}
