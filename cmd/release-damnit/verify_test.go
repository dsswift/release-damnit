package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dsswift/release-damnit/internal/proof"
)

// TestVerify_ReadsTheAllowedSignersFromTheVariable checks the pipeline's
// form: the allowed signers come in RELEASE_DAMNIT_ALLOWED_SIGNERS, and the
// command fails plainly without them.
func TestVerify_ReadsTheAllowedSignersFromTheVariable(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "--initial-branch=main")
	gitRun(t, dir, "config", "user.email", "user@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	write(t, dir, "release-please-config.json", `{"check": "make check", "packages": {".": {"component": "app"}}}`)
	write(t, dir, "release-please-manifest.json", `{".": "1.0.0"}`)
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "feat: start")

	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	signer := proof.Signer{Key: key, Principal: "user@example.com"}
	p, err := signer.Sign(proof.Statement{Tree: gitRun(t, dir, "rev-parse", "HEAD^{tree}"), Check: "make check", Result: proof.ResultPass, Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := proof.Attach(dir, "HEAD", p, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	verify := func(env ...string) (string, int) {
		cmd := exec.Command(binary, "verify", "--remote", "")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); ok {
			return string(out), exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(out), 0
	}
	if out, code := verify("RELEASE_DAMNIT_ALLOWED_SIGNERS="); code != 1 || !strings.Contains(out, "RELEASE_DAMNIT_ALLOWED_SIGNERS") {
		t.Errorf("with no allowed signers: exit %d, output %q", code, out)
	}
	out, code := verify("RELEASE_DAMNIT_ALLOWED_SIGNERS=user@example.com " + strings.TrimSpace(string(pub)))
	if code != 0 || !strings.Contains(out, "landed by user@example.com") {
		t.Errorf("with the signer allowed: exit %d, output %q", code, out)
	}
}
