package land

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/proof"
)

func attest(t *testing.T, dir string) proof.Proof {
	t.Helper()
	p, err := proof.Attest(dir, "HEAD", t.TempDir())
	if err != nil {
		t.Fatalf("attest: %v", err)
	}
	return p
}

func TestProven_LandsTheAttestedTreeAndRunsNoCheck(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)

	out, err := land(t, w.a, Options{Proven: true})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if runs := w.checkRuns(t); len(runs) != 0 {
		t.Errorf("a proven landing ran the check: %v", runs)
	}
	if v := w.verifyRemote(t); v.Commit != w.remoteMain(t) {
		t.Errorf("the proof is on %s, want the landed main %s", v.Commit, w.remoteMain(t))
	}
}

func TestProven_NoProofPushesNothingAndRunsNoCheck(t *testing.T) {
	w := newProvedWorld(t, "")
	before := w.remoteMain(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{Proven: true})
	if err == nil || !strings.Contains(err.Error(), "release-damnit attest") {
		t.Fatalf("err = %v, want it to say to attest\n%s", err, out)
	}
	if w.remoteMain(t) != before || len(w.checkRuns(t)) != 0 {
		t.Errorf("with no proof: main %s (was %s), check runs %v", w.remoteMain(t), before, w.checkRuns(t))
	}
}

func TestProven_StopsWithoutRebasingWhenMainMovedBefore(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.b, "b.txt", "b\n", "feat: from b")
	run(t, w.b, "push", "-q", "origin", "main")
	head := commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)
	moved := w.remoteMain(t)

	out, err := land(t, w.a, Options{Proven: true})
	if !errors.Is(err, ErrMainMoved) {
		t.Fatalf("err = %v, want ErrMainMoved\n%s", err, out)
	}
	if got := run(t, w.a, "rev-parse", "HEAD"); got != head {
		t.Errorf("a proven landing moved HEAD from %s to %s", head, got)
	}
	if w.remoteMain(t) != moved || len(w.checkRuns(t)) != 0 {
		t.Errorf("main moved: main %s (was %s), check runs %v", w.remoteMain(t), moved, w.checkRuns(t))
	}
}

func TestProven_StopsWhenMainMovesDuringThePush(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.b, "b.txt", "b\n", "feat: from b")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)
	raceOnce(t, w.a, w.b)

	out, err := land(t, w.a, Options{Proven: true})
	if !errors.Is(err, ErrMainMoved) {
		t.Fatalf("err = %v, want ErrMainMoved\n%s", err, out)
	}
	if len(w.checkRuns(t)) != 0 || w.remoteNotes(t) != "" {
		t.Errorf("after the race: check runs %v, remote notes %q", w.checkRuns(t), w.remoteNotes(t))
	}
}

func TestProven_ARaceOnTheProofsAloneKeepsTheProof(t *testing.T) {
	w := newProvedWorld(t, "")
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
	attest(t, w.a)

	out, err := land(t, w.a, Options{Proven: true})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if len(w.checkRuns(t)) != 0 {
		t.Errorf("the check ran: %v", w.checkRuns(t))
	}
	w.verifyRemote(t)
}

func TestProven_RefusesAProofOfAnotherTree(t *testing.T) {
	w := newProvedWorld(t, "")
	before := w.remoteMain(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	p := attest(t, w.a)
	commit(t, w.a, "a.txt", "changed after the check\n", "fix: change a")
	if err := proof.Attach(w.a, "HEAD", p, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	out, err := land(t, w.a, Options{Proven: true})
	if !errors.Is(err, proof.ErrWrongTree) {
		t.Fatalf("err = %v, want ErrWrongTree\n%s", err, out)
	}
	if w.remoteMain(t) != before {
		t.Errorf("a proof of another tree pushed main to %s", w.remoteMain(t))
	}
}

func TestProven_RefusesAProofAnotherKeySigned(t *testing.T) {
	w := newProvedWorld(t, "")
	before := w.remoteMain(t)
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)
	other := filepath.Join(t.TempDir(), "other")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", other).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	run(t, w.a, "config", "user.signingkey", other)

	out, err := land(t, w.a, Options{Proven: true})
	if err == nil || !strings.Contains(err.Error(), "does not hold") {
		t.Fatalf("err = %v, want the proof refused\n%s", err, out)
	}
	if w.remoteMain(t) != before {
		t.Errorf("a proof another key signed pushed main to %s", w.remoteMain(t))
	}
}

func TestProven_RefusesABranch(t *testing.T) {
	w := newProvedWorld(t, "")
	run(t, w.a, "checkout", "-q", "-b", "topic")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)

	out, err := land(t, w.a, Options{Proven: true})
	if err == nil || !strings.Contains(err.Error(), "own commits") {
		t.Fatalf("err = %v, want a branch refused\n%s", err, out)
	}
}

func TestProven_DefaultLandIsUnchanged(t *testing.T) {
	w := newProvedWorld(t, "")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")
	attest(t, w.a)

	// Without Proven, land runs the check itself even over an attested HEAD.
	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if runs := w.checkRuns(t); len(runs) != 1 {
		t.Errorf("the check ran %d times, want once", len(runs))
	}
}
