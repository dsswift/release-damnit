package proof

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicKey_ReadsAPublicFileAndDerivesFromAPrivateOne(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(strings.Fields(string(pub))[:2], " ")
	for _, path := range []string{key, key + ".pub"} {
		got, err := publicKey(path)
		if err != nil || got != want {
			t.Errorf("publicKey(%s) = %q, %v; want %q", filepath.Base(path), got, err, want)
		}
	}
}

func TestAttest_NeedsANamedCheck(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=user@example.com", "-c", "user.name=Test", "commit", "-q", "--allow-empty", "-m", "chore: seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := Attest(dir, "HEAD", t.TempDir()); !errors.Is(err, ErrNoCheck) {
		t.Errorf("err = %v, want ErrNoCheck", err)
	}
}
