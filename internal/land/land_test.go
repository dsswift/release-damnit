package land

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, path, content, message string) string {
	t.Helper()
	write(t, dir, path, content)
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", message)
	return run(t, dir, "rev-parse", "HEAD")
}

// world is a bare remote with one commit on main and two clones of it.
type world struct {
	remote, a, b string
}

func newWorld(t *testing.T) world {
	t.Helper()
	root := t.TempDir()
	w := world{remote: filepath.Join(root, "remote.git"), a: filepath.Join(root, "a"), b: filepath.Join(root, "b")}
	run(t, root, "init", "-q", "--bare", "--initial-branch=main", w.remote)
	seed := filepath.Join(root, "seed")
	run(t, root, "clone", "-q", w.remote, seed)
	configure(t, seed)
	commit(t, seed, "README.md", "seed\n", "chore: seed")
	run(t, seed, "push", "-q", "origin", "HEAD:refs/heads/main")
	for _, dir := range []string{w.a, w.b} {
		run(t, root, "clone", "-q", w.remote, dir)
		configure(t, dir)
	}
	return w
}

func configure(t *testing.T, dir string) {
	run(t, dir, "config", "user.email", "user@example.com")
	run(t, dir, "config", "user.name", "Test")
}

func (w world) remoteMain(t *testing.T) string {
	return run(t, w.remote, "rev-parse", "refs/heads/main")
}

func land(t *testing.T, dir string, opts Options) (string, error) {
	t.Helper()
	var out bytes.Buffer
	opts.Dir = dir
	opts.Out = &out
	if opts.Attempts == 0 {
		opts.Attempts = 5
	}
	if opts.TempDir == "" {
		opts.TempDir = t.TempDir()
	}
	err := Run(opts)
	return out.String(), err
}

// raceOnce installs a pre-push hook in dir that, on the first push only,
// pushes a commit from other first, so dir's push is rejected.
func raceOnce(t *testing.T, dir, other string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "raced")
	hookDir := run(t, dir, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	write(t, hookDir, "pre-push", `#!/bin/sh
if [ ! -f "`+marker+`" ]; then
  touch "`+marker+`"
  unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
  git -C "`+other+`" push -q origin HEAD:refs/heads/main >&2
fi
exit 0
`)
}

func assertNoTempWorktrees(t *testing.T, dir, tempDir string, want int) {
	t.Helper()
	list := run(t, dir, "worktree", "list", "--porcelain")
	if got := strings.Count(list, "worktree "); got != want {
		t.Errorf("worktrees = %d, want %d:\n%s", got, want, list)
	}
	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 0 {
		t.Errorf("temp dir still holds %v", entries)
	}
}

func TestLand_OnMainRebasesOntoRemote(t *testing.T) {
	w := newWorld(t)
	other := commit(t, w.b, "b.txt", "b\n", "feat: from b")
	run(t, w.b, "push", "-q", "origin", "main")
	commit(t, w.a, "a.txt", "a\n", "feat: from a")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	head := run(t, w.a, "rev-parse", "HEAD")
	if w.remoteMain(t) != head {
		t.Errorf("remote main %s, local HEAD %s", w.remoteMain(t), head)
	}
	if parent := run(t, w.a, "rev-parse", "HEAD^"); parent != other {
		t.Errorf("landed commit's parent = %s, want b's commit %s (rebased, linear)", parent, other)
	}
	if !strings.Contains(out, "rebasing 1 local commit(s)") {
		t.Errorf("output does not say it rebased:\n%s", out)
	}
}

func TestLand_OnMainNothingToLand(t *testing.T) {
	w := newWorld(t)
	before := w.remoteMain(t)
	out, err := land(t, w.a, Options{})
	if err != nil || !strings.Contains(out, "nothing to land") || w.remoteMain(t) != before {
		t.Errorf("err %v, output:\n%s", err, out)
	}
}

func TestLand_OnMainRetriesWhenTheRemoteMoves(t *testing.T) {
	w := newWorld(t)
	raced := commit(t, w.b, "b.txt", "b\n", "fix: lands during the push")
	mine := commit(t, w.a, "a.txt", "a\n", "feat: mine")
	raceOnce(t, w.a, w.b)

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if !strings.Contains(out, "retrying (attempt 2/5)") {
		t.Errorf("no retry in output:\n%s", out)
	}
	if run(t, w.a, "rev-parse", "HEAD^") != raced {
		t.Errorf("not rebased onto the racing commit")
	}
	if subject := run(t, w.remote, "log", "-1", "--format=%s", "main"); subject != "feat: mine" || w.remoteMain(t) == mine {
		t.Errorf("remote main = %s %q, want the rebased copy of mine", w.remoteMain(t), subject)
	}
}

func TestLand_GivesUpAfterTheLastAttempt(t *testing.T) {
	w := newWorld(t)
	commit(t, w.b, "b.txt", "b\n", "fix: racer")
	commit(t, w.a, "a.txt", "a\n", "feat: mine")
	raceOnce(t, w.a, w.b)

	out, err := land(t, w.a, Options{Attempts: 1})
	if err == nil || !strings.Contains(err.Error(), "gave up after 1 attempts") {
		t.Errorf("err = %v\n%s", err, out)
	}
}

func TestLand_PushRefusedWithoutARace(t *testing.T) {
	w := newWorld(t)
	commit(t, w.a, "a.txt", "a\n", "feat: mine")
	hookDir := run(t, w.a, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	write(t, hookDir, "pre-push", "#!/bin/sh\necho 'hook says no' >&2\nexit 1\n")
	before := w.remoteMain(t)

	out, err := land(t, w.a, Options{})
	if err == nil || !strings.Contains(err.Error(), "push refused") || !strings.Contains(out, "hook says no") {
		t.Errorf("err = %v\n%s", err, out)
	}
	if w.remoteMain(t) != before {
		t.Error("remote moved")
	}
}

func TestLand_RefusesADirtyTreeOnTheBranchItLands(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	commit(t, w.a, "a.txt", "a\n", "feat: mine")
	write(t, w.a, "a.txt", "edited\n")
	if _, err := land(t, w.a, Options{}); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("err = %v", err)
	}

	// Untracked files are fine.
	run(t, w.a, "checkout", "-q", "a.txt")
	write(t, w.a, "scratch.txt", "untracked\n")
	if out, err := land(t, w.a, Options{}); err != nil {
		t.Errorf("untracked file blocked the landing: %v\n%s", err, out)
	}
}

func TestLand_OnADirtyMainLandsAsAMergeAndTouchesNothing(t *testing.T) {
	w := newWorld(t)
	other := commit(t, w.b, "b.txt", "b\n", "feat: from b")
	run(t, w.b, "push", "-q", "origin", "main")
	mine := commit(t, w.a, "a.txt", "a\n", "feat: from a")
	// Someone else's work in progress, in the same checkout.
	write(t, w.a, "README.md", "half done\n")
	tempDir := t.TempDir()

	out, err := land(t, w.a, Options{TempDir: tempDir})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	merged := w.remoteMain(t)
	if parents := run(t, w.a, "log", "-1", "--format=%P", merged); parents != other+" "+mine {
		t.Errorf("remote main's parents = %s, want b's commit then a's", parents)
	}
	if head := run(t, w.a, "rev-parse", "main"); head != mine {
		t.Errorf("local main moved to %s; it must stay at %s under uncommitted changes", head, mine)
	}
	if got, _ := os.ReadFile(filepath.Join(w.a, "README.md")); string(got) != "half done\n" {
		t.Errorf("README.md = %q; the uncommitted change was touched", got)
	}
	if _, err := os.Stat(filepath.Join(w.a, "b.txt")); err == nil {
		t.Error("b.txt appeared: the checkout's files moved")
	}
	if status := run(t, w.a, "status", "--porcelain", "--untracked-files=no"); status != "M README.md" {
		t.Errorf("status = %q, want only the uncommitted change", status)
	}
	if !strings.Contains(out, "stay as they are") || !strings.Contains(out, "note: main is checked out at") {
		t.Errorf("output does not explain itself:\n%s", out)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 1)

	// The next landing from the same checkout finds nothing left to land.
	out, err = land(t, w.a, Options{TempDir: tempDir})
	if err != nil || !strings.Contains(out, "nothing to land") {
		t.Errorf("second land: %v\n%s", err, out)
	}
}

func TestLand_ABranchLandsFromADirtyMainCheckout(t *testing.T) {
	w := newWorld(t)
	feature := filepath.Join(t.TempDir(), "feature")
	run(t, w.a, "worktree", "add", "-q", "-b", "feature", feature, "main")
	configure(t, feature)
	commit(t, feature, "f.txt", "1\n", "feat: one")
	write(t, w.a, "README.md", "half done\n")

	out, err := land(t, w.a, Options{Branch: "feature"})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(w.a, "README.md")); string(got) != "half done\n" {
		t.Errorf("README.md = %q; the uncommitted change was touched", got)
	}
	if run(t, w.a, "rev-parse", "main") == w.remoteMain(t) {
		t.Error("local main moved under uncommitted changes")
	}
}

func TestLand_RefusesCommitsThatBreakTheRules(t *testing.T) {
	w := newWorld(t)
	commit(t, w.a, "a.txt", "a\n", "Added a thing.")
	before := w.remoteMain(t)
	out, err := land(t, w.a, Options{})
	if err != ErrCommitRules || !strings.Contains(out, "Added a thing.") {
		t.Errorf("err = %v\n%s", err, out)
	}
	if w.remoteMain(t) != before {
		t.Error("a bad commit was pushed")
	}
}

func TestLand_RebaseConflictAborts(t *testing.T) {
	w := newWorld(t)
	commit(t, w.b, "README.md", "theirs\n", "docs: theirs")
	run(t, w.b, "push", "-q", "origin", "main")
	mine := commit(t, w.a, "README.md", "mine\n", "docs: mine")

	_, err := land(t, w.a, Options{})
	if err == nil || !strings.Contains(err.Error(), "conflicts in README.md") {
		t.Fatalf("err = %v", err)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != mine {
		t.Errorf("HEAD = %s after an aborted rebase, want %s", head, mine)
	}
	if _, err := os.Stat(filepath.Join(w.a, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Error("rebase left in progress")
	}
}

func TestLand_BranchLandsAsMergeAndMovesLocalMain(t *testing.T) {
	w := newWorld(t)
	theirs := commit(t, w.b, "b.txt", "b\n", "fix: theirs")
	run(t, w.b, "push", "-q", "origin", "main")

	run(t, w.a, "checkout", "-q", "-b", "feature")
	commit(t, w.a, "f1.txt", "1\n", "feat: one")
	tip := commit(t, w.a, "f2.txt", "2\n", "feat: two")
	tempDir := t.TempDir()

	out, err := land(t, w.a, Options{TempDir: tempDir})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	merge := w.remoteMain(t)
	if parents := run(t, w.remote, "log", "-1", "--format=%P", merge); parents != theirs+" "+tip {
		t.Errorf("merge parents = %s, want %s %s", parents, theirs, tip)
	}
	if local := run(t, w.a, "rev-parse", "refs/heads/main"); local != merge {
		t.Errorf("local main = %s, want the landed merge %s", local, merge)
	}
	if current := run(t, w.a, "branch", "--show-current"); current != "feature" {
		t.Errorf("current branch = %s; land must not check anything out", current)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 1)
}

func TestLand_BranchFastForwardsACleanCheckedOutMain(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	commit(t, w.a, "f.txt", "1\n", "feat: one")
	run(t, w.a, "checkout", "-q", "main")

	out, err := land(t, w.a, Options{Branch: "feature"})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if head := run(t, w.a, "rev-parse", "HEAD"); head != w.remoteMain(t) {
		t.Errorf("checked-out main HEAD = %s, want %s", head, w.remoteMain(t))
	}
	if !strings.Contains(out, "fast-forwarded main") {
		t.Errorf("output:\n%s", out)
	}
	// The branch sat directly on main, so only --no-ff makes this a merge.
	if parents := strings.Fields(run(t, w.remote, "log", "-1", "--format=%P", "main")); len(parents) != 2 {
		t.Errorf("landed commit has parents %v, want a two-parent merge", parents)
	}
}

func TestLand_BranchLeavesADirtyMainWorktreeAlone(t *testing.T) {
	w := newWorld(t)
	mainTree := filepath.Join(t.TempDir(), "main-tree")
	run(t, w.a, "checkout", "-q", "-b", "feature")
	run(t, w.a, "worktree", "add", "-q", mainTree, "main")
	write(t, mainTree, "README.md", "local edit\n")
	before := run(t, mainTree, "rev-parse", "HEAD")
	commit(t, w.a, "f.txt", "1\n", "feat: one")
	tempDir := t.TempDir()

	out, err := land(t, w.a, Options{TempDir: tempDir})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if run(t, mainTree, "rev-parse", "HEAD") != before {
		t.Error("the dirty main worktree moved")
	}
	if !strings.Contains(out, "note: main is checked out at") {
		t.Errorf("no note in output:\n%s", out)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 2)
}

func TestLand_BranchRefusesStrayLocalMainCommits(t *testing.T) {
	w := newWorld(t)
	commit(t, w.a, "stray.txt", "x\n", "feat: stray on main")
	run(t, w.a, "checkout", "-q", "-b", "feature", "origin/main")
	commit(t, w.a, "f.txt", "1\n", "feat: one")
	before := w.remoteMain(t)

	_, err := land(t, w.a, Options{})
	if err == nil || !strings.Contains(err.Error(), "local main has commits that are not on origin and not on feature") {
		t.Errorf("err = %v", err)
	}
	if w.remoteMain(t) != before {
		t.Error("remote moved")
	}
}

func TestLand_BranchAcceptsCopiesOfItsCommitsOnLocalMain(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	tip := commit(t, w.a, "f.txt", "1\n", "feat: one")
	run(t, w.a, "checkout", "-q", "main")
	// -x changes the message, so the copy is a different commit with the same patch.
	run(t, w.a, "cherry-pick", "-x", tip)
	if run(t, w.a, "rev-parse", "main") == tip {
		t.Fatal("the copy is the same commit; the test would prove nothing")
	}
	run(t, w.a, "checkout", "-q", "feature")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if !strings.Contains(out, "holds only copies of feature's commits") {
		t.Errorf("output:\n%s", out)
	}
	if run(t, w.a, "rev-parse", "main") != w.remoteMain(t) {
		t.Error("local main was not replaced by the merge")
	}
}

func TestLand_BranchRetriesWhenTheRemoteMoves(t *testing.T) {
	w := newWorld(t)
	raced := commit(t, w.b, "b.txt", "b\n", "fix: racer")
	run(t, w.a, "checkout", "-q", "-b", "feature")
	tip := commit(t, w.a, "f.txt", "1\n", "feat: one")
	raceOnce(t, w.a, w.b)
	tempDir := t.TempDir()

	out, err := land(t, w.a, Options{TempDir: tempDir})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if !strings.Contains(out, "retrying (attempt 2/5)") {
		t.Errorf("no retry:\n%s", out)
	}
	if parents := run(t, w.remote, "log", "-1", "--format=%P", "main"); parents != raced+" "+tip {
		t.Errorf("merge parents = %s, want %s %s", parents, raced, tip)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 1)
}

func TestLand_BranchConflictCleansUpTheTempWorktree(t *testing.T) {
	w := newWorld(t)
	commit(t, w.b, "README.md", "theirs\n", "docs: theirs")
	run(t, w.b, "push", "-q", "origin", "main")
	run(t, w.a, "checkout", "-q", "-b", "feature")
	commit(t, w.a, "README.md", "mine\n", "docs: mine")
	tempDir := t.TempDir()

	_, err := land(t, w.a, Options{TempDir: tempDir})
	if err == nil || !strings.Contains(err.Error(), "does not merge cleanly") || !strings.Contains(err.Error(), "README.md") {
		t.Errorf("err = %v", err)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 1)
}

func TestLand_BranchPushRefusalCleansUpTheTempWorktree(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	commit(t, w.a, "f.txt", "1\n", "feat: one")
	hookDir := run(t, w.a, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	write(t, hookDir, "pre-push", "#!/bin/sh\nexit 1\n")
	tempDir := t.TempDir()

	if _, err := land(t, w.a, Options{TempDir: tempDir}); err == nil || !strings.Contains(err.Error(), "push refused") {
		t.Errorf("err = %v", err)
	}
	assertNoTempWorktrees(t, w.a, tempDir, 1)
}

func TestLand_BranchWithoutLocalMain(t *testing.T) {
	w := newWorld(t)
	run(t, w.a, "checkout", "-q", "-b", "feature")
	run(t, w.a, "branch", "-D", "main")
	commit(t, w.a, "f.txt", "1\n", "feat: one")

	out, err := land(t, w.a, Options{})
	if err != nil {
		t.Fatalf("land: %v\n%s", err, out)
	}
	if run(t, w.a, "rev-parse", "refs/heads/main") != w.remoteMain(t) {
		t.Error("local main was not created at the landed merge")
	}
}

func TestLand_RefusesMainAsTheBranch(t *testing.T) {
	w := newWorld(t)
	if _, err := land(t, w.a, Options{Branch: "main"}); err == nil || !strings.Contains(err.Error(), "cannot be landed on itself") {
		t.Errorf("err = %v", err)
	}
	if _, err := land(t, w.a, Options{Branch: "nope"}); err == nil || !strings.Contains(err.Error(), "no local branch named nope") {
		t.Errorf("err = %v", err)
	}
}
