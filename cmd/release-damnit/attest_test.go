package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLI_AttestThenLandProven is the lander whose check runs elsewhere:
// attest signs HEAD's tree, land --proven pushes it without running the
// check, and a moved main exits 3 with nothing pushed.
func TestCLI_AttestThenLandProven(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	gitRun(t, root, "init", "-q", "--bare", "--initial-branch=main", remote)
	clone := filepath.Join(root, "clone")
	gitRun(t, root, "clone", "-q", remote, clone)
	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	ran := filepath.Join(t.TempDir(), "ran")
	for _, kv := range [][2]string{{"user.email", "user@example.com"}, {"user.name", "Test"}, {"gpg.format", "ssh"}, {"user.signingkey", key}} {
		gitRun(t, clone, "config", kv[0], kv[1])
	}
	write(t, clone, "release-please-config.json", `{"check": "touch `+ran+`", "packages": {".": {"component": "app"}}}`)
	write(t, clone, "release-please-manifest.json", `{".": "1.0.0"}`)
	gitRun(t, clone, "add", "-A")
	gitRun(t, clone, "commit", "-qm", "chore: start")
	gitRun(t, clone, "push", "-q", "origin", "main")

	write(t, clone, "a.txt", "a\n")
	gitRun(t, clone, "add", "-A")
	gitRun(t, clone, "commit", "-qm", "feat: add a")
	if out, code := cli(t, clone, "land", "--proven"); code != 1 || !strings.Contains(out, "attest") {
		t.Errorf("unproven: exit %d, output:\n%s", code, out)
	}
	if out, code := cli(t, clone, "attest"); code != 0 || !strings.Contains(out, "signed as SHA256:") {
		t.Fatalf("attest: exit %d, output:\n%s", code, out)
	}
	if out, code := cli(t, clone, "land", "--proven"); code != 0 {
		t.Fatalf("land --proven: exit %d, output:\n%s", code, out)
	}
	if gitRun(t, remote, "rev-parse", "main") != gitRun(t, clone, "rev-parse", "HEAD") {
		t.Error("not landed")
	}

	other := filepath.Join(root, "other")
	gitRun(t, root, "clone", "-q", remote, other)
	gitRun(t, other, "config", "user.email", "user@example.com")
	gitRun(t, other, "config", "user.name", "Test")
	write(t, other, "c.txt", "c\n")
	gitRun(t, other, "add", "-A")
	gitRun(t, other, "commit", "-qm", "feat: add c")
	gitRun(t, other, "push", "-q", "origin", "main")
	write(t, clone, "b.txt", "b\n")
	gitRun(t, clone, "add", "-A")
	gitRun(t, clone, "commit", "-qm", "fix: add b")
	cli(t, clone, "attest")
	if out, code := cli(t, clone, "land", "--proven"); code != exitMainMoved || !strings.Contains(out, "moved") {
		t.Errorf("main moved: exit %d, output:\n%s", code, out)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("a proven landing ran the check")
	}
}
