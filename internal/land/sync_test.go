package land

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syncDir(t *testing.T, dir string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := Sync(SyncOptions{Dir: dir, Out: &out})
	return out.String(), err
}

func read(t *testing.T, dir, path string) string {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// push commits in b and pushes it, so a's main falls behind.
func push(t *testing.T, w world, path, content, message string) string {
	t.Helper()
	sha := commit(t, w.b, path, content, message)
	run(t, w.b, "push", "-q", "origin", "main")
	return sha
}

func TestSync_FastForwardsACleanMain(t *testing.T) {
	w := newWorld(t)
	tip := push(t, w, "b.txt", "b\n", "feat: b")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != tip {
		t.Errorf("HEAD = %s, want %s", head, tip)
	}
	out, _ = syncDir(t, w.a)
	if !strings.Contains(out, "matches") {
		t.Errorf("second sync:\n%s", out)
	}
}

func TestSync_KeepsChangesToFilesNothingIncomingTouches(t *testing.T) {
	w := newWorld(t)
	tip := push(t, w, "b.txt", "b\n", "feat: b")
	write(t, w.a, "README.md", "half done\n")
	write(t, w.a, "scratch.txt", "untracked\n")
	commit(t, w.a, "staged.txt", "s\n", "chore: base")
	run(t, w.a, "reset", "-q", "--soft", "HEAD~1")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != tip {
		t.Errorf("HEAD = %s, want %s:\n%s", head, tip, out)
	}
	if read(t, w.a, "README.md") != "half done\n" || read(t, w.a, "scratch.txt") != "untracked\n" {
		t.Error("an uncommitted change was lost")
	}
	if status := run(t, w.a, "status", "--porcelain"); !strings.Contains(status, "A  staged.txt") {
		t.Errorf("the staged file was unstaged: %q", status)
	}
}

func TestSync_MergesAChangeIntoAFileThatAlsoChangedUpstream(t *testing.T) {
	w := newWorld(t)
	commit(t, w.b, "list.txt", "one\ntwo\nthree\nfour\nfive\n", "feat: list")
	run(t, w.b, "push", "-q", "origin", "main")
	run(t, w.a, "pull", "-q")
	push(t, w, "list.txt", "ONE\ntwo\nthree\nfour\nfive\n", "fix: first line")
	write(t, w.a, "list.txt", "one\ntwo\nthree\nfour\nFIVE\n")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := read(t, w.a, "list.txt"); got != "ONE\ntwo\nthree\nfour\nFIVE\n" {
		t.Errorf("list.txt = %q, want both edits", got)
	}
	if diff := run(t, w.a, "diff"); !strings.Contains(diff, "+FIVE") || strings.Contains(diff, "+ONE") {
		t.Errorf("the uncommitted diff should be only the local edit:\n%s", diff)
	}
}

func TestSync_ACopyOfWhatLandedIsNoLongerAChange(t *testing.T) {
	w := newWorld(t)
	push(t, w, "README.md", "landed\n", "docs: readme")
	push(t, w, "new/file.txt", "new\n", "feat: new file")
	// The same edits, left behind as uncommitted copies.
	write(t, w.a, "README.md", "landed\n")
	write(t, w.a, "new/file.txt", "new\n")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if status := run(t, w.a, "status", "--porcelain"); status != "" {
		t.Errorf("status = %q, want clean", status)
	}
}

func TestSync_LeavesEverythingWhenAChangeConflicts(t *testing.T) {
	w := newWorld(t)
	before := run(t, w.a, "rev-parse", "HEAD")
	push(t, w, "b.txt", "b\n", "feat: b")
	push(t, w, "README.md", "theirs\n", "docs: theirs")
	write(t, w.a, "README.md", "mine\n")
	write(t, w.a, "b.txt", "different\n")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != before {
		t.Errorf("main moved over a conflict")
	}
	if read(t, w.a, "README.md") != "mine\n" || read(t, w.a, "b.txt") != "different\n" {
		t.Error("an uncommitted change was touched")
	}
	if !strings.Contains(out, "README.md") || !strings.Contains(out, "b.txt") {
		t.Errorf("output does not name the files:\n%s", out)
	}
}

func TestSync_LeavesAMainWithLocalCommitsForLand(t *testing.T) {
	w := newWorld(t)
	push(t, w, "b.txt", "b\n", "feat: b")
	mine := commit(t, w.a, "a.txt", "a\n", "feat: a")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != mine || !strings.Contains(out, "land them") {
		t.Errorf("HEAD = %s:\n%s", head, out)
	}
}

func TestSync_MovesAMainThatIsNotCheckedOut(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	tip := push(t, w, "b.txt", "b\n", "feat: b")

	if out, err := syncDir(t, w.a); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := run(t, w.a, "rev-parse", "main"); got != tip {
		t.Errorf("main = %s, want %s", got, tip)
	}
}

func TestSync_WaitsOutARebase(t *testing.T) {
	w := newWorld(t)
	before := run(t, w.a, "rev-parse", "HEAD")
	push(t, w, "b.txt", "b\n", "feat: b")
	write(t, filepath.Join(w.a, ".git"), "index.lock", "")

	out, _ := syncDir(t, w.a)
	if run(t, w.a, "rev-parse", "HEAD") != before || !strings.Contains(out, "under way") {
		t.Errorf("sync moved main during a git command:\n%s", out)
	}
}

func TestSync_ACopyOfAnOlderLandedVersionMergesFromThatVersion(t *testing.T) {
	w := newWorld(t)
	commit(t, w.b, "list.txt", "one\ntwo\nthree\n", "feat: list")
	run(t, w.b, "push", "-q", "origin", "main")
	run(t, w.a, "pull", "-q")
	push(t, w, "list.txt", "one\nTWO\nthree\n", "fix: two")
	push(t, w, "list.txt", "one\nTWO!\nthree\nfour\n", "fix: two again")
	// Left behind: a copy of the middle version, plus an edit of its own.
	write(t, w.a, "list.txt", "zero\none\nTWO\nthree\n")

	out, err := syncDir(t, w.a)
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := read(t, w.a, "list.txt"); got != "zero\none\nTWO!\nthree\nfour\n" {
		t.Errorf("list.txt = %q, want the newest version with the local line", got)
	}
}
