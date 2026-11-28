package land

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/proof"
)

// provedWorld is a world whose release config names a check, and whose
// clones a and b sign with their own SSH keys. The check appends a line to
// runs, so a test can count how often it ran and where.
type provedWorld struct {
	world
	runs    string
	allowed string
}

func newProvedWorld(t *testing.T, check string) provedWorld {
	t.Helper()
	w := provedWorld{world: newWorld(t), runs: filepath.Join(t.TempDir(), "runs")}
	if check == "" {
		check = `pwd >> ` + w.runs
	}
	write(t, w.a, "release-please-config.json", `{"check": "`+strings.ReplaceAll(check, `"`, `\"`)+`", "packages": {".": {"component": "app"}}}`)
	write(t, w.a, "release-please-manifest.json", `{".": "1.0.0"}`)
	commit(t, w.a, "VERSION", "1.0.0\n", "chore: add the release config")
	run(t, w.a, "push", "-q", "origin", "main")
	run(t, w.b, "pull", "-q", "--rebase", "origin", "main")

	var lines []string
	for _, dir := range []string{w.a, w.b} {
		path := filepath.Join(t.TempDir(), "key")
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v\n%s", err, out)
		}
		run(t, dir, "config", "gpg.format", "ssh")
		run(t, dir, "config", "user.signingkey", path)
		pub, err := os.ReadFile(path + ".pub")
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, err := proof.Fingerprint(path)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fingerprint+` namespaces="`+proof.Namespace+`" `+strings.TrimSpace(string(pub)))
	}
	w.allowed = filepath.Join(t.TempDir(), "allowed_signers")
	write(t, filepath.Dir(w.allowed), filepath.Base(w.allowed), strings.Join(lines, "\n")+"\n")
	return w
}

func (w provedWorld) checkRuns(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(w.runs)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

// verifyRemote verifies the remote main from a fresh clone, as a pipeline would.
func (w provedWorld) verifyRemote(t *testing.T) proof.Verified {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "ci")
	run(t, filepath.Dir(clone), "clone", "-q", w.remote, clone)
	v, err := proof.Verify(proof.VerifyOptions{Dir: clone, Remote: "origin", AllowedSigners: w.allowed, TempDir: t.TempDir()})
	if err != nil {
		t.Fatalf("verify the remote main: %v", err)
	}
	return v
}

func (w provedWorld) remoteNotes(t *testing.T) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", w.remote, "rev-parse", "--verify", "--quiet", proof.NotesRef).Output()
	return strings.TrimSpace(string(out))
}

func TestProof_LandSignsTheTreeThatLands(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	v := w.verifyRemote(t)
	if v.Commit != w.remoteMain(t) {
		t.Errorf("the proof is on %s, want the landed main %s", v.Commit, w.remoteMain(t))
	}
	if v.Proof.Statement.Tree != run(t, w.remote, "rev-parse", "main^{tree}") || v.Proof.Statement.Check != "pwd >> "+w.runs {
		t.Errorf("proof says %+v", v.Proof.Statement)
	}
	if runs := w.checkRuns(t); len(runs) != 1 {
		t.Errorf("the check ran %d times, want once: %v", len(runs), runs)
	}
}

func TestProof_AFailedCheckPushesNothing(t *testing.T) {
	w := newProvedWorld(t, "echo the tests failed; exit 3")
	before := w.remoteMain(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("err = %v, want ErrCheckFailed\n%s", err, out)
	}
	if !strings.Contains(out, "the tests failed") {
		t.Errorf("the check's own output did not reach land's:\n%s", out)
	}
	if w.remoteMain(t) != before || w.remoteNotes(t) != "" {
		t.Errorf("a failed check pushed: main %s (was %s), notes %q", w.remoteMain(t), before, w.remoteNotes(t))
	}
}

func TestProof_NoSigningKeyPushesNothingAndRunsNoCheck(t *testing.T) {
	w := newProvedWorld(t, "")
	// An empty value hides any key the machine's own git config names.
	run(t, w.a, "config", "user.signingkey", "")
	before := w.remoteMain(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err == nil || !strings.Contains(err.Error(), "user.signingkey") {
		t.Fatalf("err = %v, want it to name user.signingkey\n%s", err, out)
	}
	if w.remoteMain(t) != before || len(w.checkRuns(t)) != 0 {
		t.Errorf("with no key: main %s (was %s), check runs %v", w.remoteMain(t), before, w.checkRuns(t))
	}
}

func TestProof_RebaseAndRetryRedoesTheCheckOnTheNewTree(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.b, "b.txt", "b\n", "feat: from b")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	raceOnce(t, w.a, w.b) // b's commit lands while a pushes: a rebases onto it

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if runs := w.checkRuns(t); len(runs) != 2 {
		t.Errorf("the check ran %d times, want twice (the first tree, then the rebased one): %v\n%s", len(runs), runs, out)
	}
	v := w.verifyRemote(t)
	if got := run(t, w.remote, "rev-parse", "main^{tree}"); v.Proof.Statement.Tree != got {
		t.Errorf("proof tree %s, landed tree %s", v.Proof.Statement.Tree, got)
	}
}

func TestProof_ARaceOnTheProofsAloneKeepsTheProof(t *testing.T) {
	w := newProvedWorld(t, "")
	// b puts a proof note on the seed commit, so only the notes ref moves
	// while a pushes; main stays where a fetched it.
	marker := filepath.Join(t.TempDir(), "raced")
	hookDir := run(t, w.a, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	write(t, hookDir, "pre-push", `#!/bin/sh
if [ ! -f "`+marker+`" ]; then
  touch "`+marker+`"
  unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
  git -C "`+w.b+`" notes --ref `+proof.NotesRef+` add -f -m other HEAD >&2
  git -C "`+w.b+`" push -q origin `+proof.NotesRef+`:`+proof.NotesRef+` >&2
fi
exit 0
`)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if !strings.Contains(out, "another landing pushed its proof first") || !strings.Contains(out, "its proof moves to") {
		t.Errorf("the retry did not keep the proof:\n%s", out)
	}
	if runs := w.checkRuns(t); len(runs) != 1 {
		t.Errorf("the check ran %d times, want once: the tree did not change", len(runs))
	}
	w.verifyRemote(t)
	if note, err := exec.Command("git", "-C", w.remote, "notes", "--ref", proof.NotesRef, "show", "main^").Output(); err != nil || strings.TrimSpace(string(note)) != "other" {
		t.Errorf("the other landing's note was lost: %q, %v", note, err)
	}
}

func TestProof_ABranchLandsWithAProofOfTheMerge(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.b, "b.txt", "b\n", "feat: from b")
	run(t, w.b, "push", "-q", "origin", "main")
	run(t, w.a, "checkout", "-q", "-b", "topic")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	v := w.verifyRemote(t)
	if parents := strings.Fields(run(t, w.remote, "rev-list", "--parents", "-n", "1", v.Commit)); len(parents) != 3 {
		t.Errorf("the proof is on %s, want the merge commit", v.Commit)
	}
	// main had moved, so the merge's tree is not the branch's: the check ran
	// in the merge's own worktree, not in the branch's checkout.
	runs := w.checkRuns(t)
	if len(runs) != 1 || runs[0] == w.a {
		t.Errorf("check ran in %v, want once in the temporary merge worktree", runs)
	}
}

func TestProof_ABranchUpToDateIsCheckedInItsOwnCheckout(t *testing.T) {
	w := newProvedWorld(t, "")
	run(t, w.a, "checkout", "-q", "-b", "topic")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	w.verifyRemote(t)
	real, _ := filepath.EvalSymlinks(w.a)
	if runs := w.checkRuns(t); len(runs) != 1 || (runs[0] != w.a && runs[0] != real) {
		t.Errorf("check ran in %v, want once in the branch's checkout %s", runs, w.a)
	}
}

func TestProof_NoCheckNamedLandsWithoutAProof(t *testing.T) {
	w := newWorld(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if !strings.Contains(out, `names no "check"`) {
		t.Errorf("land did not warn that the landing carries no proof:\n%s", out)
	}
	if notes, _ := exec.Command("git", "-C", w.remote, "rev-parse", "--verify", "--quiet", proof.NotesRef).Output(); len(notes) != 0 {
		t.Errorf("a landing with no check pushed proofs")
	}
}
