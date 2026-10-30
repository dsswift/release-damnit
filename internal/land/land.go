// Package land puts finished work on the remote main branch and retries when
// main moves underneath it (a CI version commit, another person's landing).
//
// On main, the local commits are rebased onto the remote main and pushed. A
// branch lands as one --no-ff merge built in a temporary worktree, so the
// user's own working trees are never checked out or reset. A main checkout
// with uncommitted changes, often another person's work in progress, lands
// its commits the branch way and then fast-forwards, carrying those changes
// forward without losing them (sync.go).
package land

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dsswift/release-damnit/internal/commitcheck"
	"github.com/dsswift/release-damnit/internal/git"
)

// Options configures a landing.
type Options struct {
	// Dir is any directory inside the repository.
	Dir string

	// Branch is the branch to land. Empty lands the current branch, or the
	// local main commits when main is checked out.
	Branch string

	// Remote and Main name the remote and its main branch.
	Remote string
	Main   string

	// Attempts is how many times to try before giving up on a moving main.
	Attempts int

	// TempDir holds the temporary merge worktree. Empty means os.TempDir().
	TempDir string

	// Out receives progress lines and git's own output (hooks, server messages).
	Out io.Writer
}

// Prefix starts every line land prints.
const Prefix = "release-damnit land: "

// ErrCommitRules is returned when a commit to land breaks the commit rules.
// The failures are already printed.
var ErrCommitRules = errors.New("fix the commits above, then land again")

type lander struct {
	Options
	prefix string // starts every line; Prefix when empty
	root   string
	remote string // refs/remotes/<remote>/<main>
}

// Run lands per opts.
func Run(opts Options) error {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Main == "" {
		opts.Main = "main"
	}
	if opts.Attempts < 1 {
		opts.Attempts = 1
	}
	if opts.TempDir == "" {
		opts.TempDir = os.TempDir()
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	root, err := git.Toplevel(opts.Dir)
	if err != nil {
		return err
	}
	l := &lander{Options: opts, root: root, remote: "refs/remotes/" + opts.Remote + "/" + opts.Main}

	dirty, err := isDirty(root)
	if err != nil {
		return err
	}
	current, _ := git.Output(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch := opts.Branch
	if branch == "" {
		switch current {
		case "":
			return errors.New("HEAD is detached; check out a branch or name the branch to land")
		case opts.Main:
			if dirty {
				// A rebase in place would stop on those changes. The merge is
				// built aside, and main then fast-forwards to it with the
				// changes carried along.
				l.logf("on %s with uncommitted changes: landing its local commits as a merge built aside", opts.Main)
				return l.landBranch(opts.Main)
			}
			l.logf("on %s: landing its local commits", opts.Main)
			return l.landMain()
		}
		branch = current
	}
	// Tracked edits on the branch being landed would not ride along, and they
	// usually mean work the user thinks is landing. Untracked files are fine.
	if dirty && branch == current {
		return errors.New("the working tree has uncommitted changes to tracked files; commit or stash them first")
	}
	if branch == opts.Main {
		return fmt.Errorf("%s cannot be landed on itself; run land with no branch while on %s to push its local commits", opts.Main, opts.Main)
	}
	if _, err := git.Output(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("no local branch named %s", branch)
	}
	l.logf("landing branch %s on %s/%s as a merge", branch, opts.Remote, opts.Main)
	return l.landBranch(branch)
}

func (l *lander) logf(format string, args ...any) {
	prefix := l.prefix
	if prefix == "" {
		prefix = Prefix
	}
	fmt.Fprintf(l.Out, prefix+format+"\n", args...)
}

func (l *lander) git(args ...string) (string, error) { return git.Output(l.root, args...) }

// fetch updates the remote-tracking main and returns its SHA.
func (l *lander) fetch(attempt int) (string, error) {
	l.logf("attempt %d/%d: fetching %s/%s", attempt, l.Attempts, l.Remote, l.Main)
	if err := git.Stream(l.root, l.Out, "fetch", "--quiet", l.Remote, "+refs/heads/"+l.Main+":"+l.remote); err != nil {
		return "", err
	}
	return l.git("rev-parse", l.remote)
}

// check runs the commit rules over revRange.
func (l *lander) check(revRange string) error {
	results, err := commitcheck.CheckRange(l.root, revRange)
	if err != nil {
		return err
	}
	if failed := commitcheck.Failures(results); len(failed) > 0 {
		commitcheck.WriteFailures(l.Out, failed)
		return ErrCommitRules
	}
	l.logf("%d commit(s) pass the commit rules", len(results))
	return nil
}

// push pushes dir's HEAD to the remote main. A rejected push reports whether
// the remote main moved from base, which is the race worth retrying.
func (l *lander) push(dir, base string, attempt int) (pushed bool, err error) {
	if err := git.Stream(dir, l.Out, "push", l.Remote, "HEAD:refs/heads/"+l.Main); err == nil {
		return true, nil
	}
	now, err := l.git("ls-remote", l.Remote, "refs/heads/"+l.Main)
	if err != nil {
		return false, err
	}
	now, _, _ = strings.Cut(now, "\t")
	if now == "" || now == base {
		return false, fmt.Errorf("push refused and %s/%s did not move; read the hook/server output above", l.Remote, l.Main)
	}
	if attempt >= l.Attempts {
		return false, fmt.Errorf("%s/%s kept moving; gave up after %d attempts", l.Remote, l.Main, attempt)
	}
	l.logf("%s/%s moved (now %s), retrying (attempt %d/%d)", l.Remote, l.Main, short(now), attempt+1, l.Attempts)
	return false, nil
}

func (l *lander) landMain() error {
	for attempt := 1; ; attempt++ {
		base, err := l.fetch(attempt)
		if err != nil {
			return err
		}
		count, err := l.count(base + "..HEAD")
		if err != nil {
			return err
		}
		if count == 0 {
			l.logf("nothing to land: %s has no commits beyond %s/%s", l.Main, l.Remote, l.Main)
			return nil
		}

		if l.isAncestor(base, "HEAD") {
			l.logf("local %s is on top of %s/%s; %d commit(s) to land", l.Main, l.Remote, l.Main, count)
		} else {
			l.logf("%s/%s has commits local %s lacks; rebasing %d local commit(s) onto it", l.Remote, l.Main, l.Main, count)
			if err := l.rebase(base); err != nil {
				return err
			}
		}

		if err := l.check(base + "..HEAD"); err != nil {
			return err
		}
		pushed, err := l.push(l.root, base, attempt)
		if err != nil {
			return err
		}
		if pushed {
			head, _ := l.git("rev-parse", "--short", "HEAD")
			l.logf("landed %d commit(s) on %s/%s at %s", count, l.Remote, l.Main, head)
			return nil
		}
	}
}

func (l *lander) rebase(onto string) error {
	if _, err := l.git("rebase", "--quiet", onto); err != nil {
		files, _ := git.OutputNUL(l.root, "diff", "--name-only", "--diff-filter=U", "-z")
		if _, abortErr := l.git("rebase", "--abort"); abortErr != nil {
			return fmt.Errorf("rebase failed and could not be aborted: %v (after: %w)", abortErr, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("rebase onto %s/%s failed; the rebase was aborted: %w", l.Remote, l.Main, err)
		}
		return fmt.Errorf("rebasing onto %s/%s conflicts in %s; the rebase was aborted. Run git rebase %s/%s, resolve, and land again",
			l.Remote, l.Main, strings.Join(files, ", "), l.Remote, l.Main)
	}
	return nil
}

func (l *lander) landBranch(branch string) error {
	for attempt := 1; ; attempt++ {
		base, err := l.fetch(attempt)
		if err != nil {
			return err
		}
		if err := l.checkLocalMain(branch, base); err != nil {
			return err
		}

		count, err := l.count(base + ".." + branch)
		if err != nil {
			return err
		}
		if count == 0 {
			l.logf("nothing to land: %s has no commits beyond %s/%s", branch, l.Remote, l.Main)
			return nil
		}
		mergeBase, err := l.git("merge-base", base, branch)
		if err != nil {
			return err
		}
		if err := l.check(mergeBase + ".." + branch); err != nil {
			return err
		}

		merged, pushed, err := l.mergeAndPush(branch, base, attempt)
		if err != nil {
			return err
		}
		if pushed {
			l.logf("landed %s on %s/%s at %s", branch, l.Remote, l.Main, short(merged))
			l.updateLocalMain(merged)
			return nil
		}
	}
}

// checkLocalMain refuses when local main holds work that is neither on the
// remote nor a copy of a commit on branch: moving main would drop it. A main
// checked out in a worktree only ever fast-forwards, which cannot drop it, so
// that main is left for its own landing instead.
func (l *lander) checkLocalMain(branch, base string) error {
	if branch == l.Main {
		return nil
	}
	localMain, err := l.git("rev-parse", "--verify", "--quiet", "refs/heads/"+l.Main)
	if err != nil {
		l.logf("no local %s; nothing to protect", l.Main)
		return nil
	}
	if _, checkedOut, err := l.worktreeOf("refs/heads/" + l.Main); err == nil && checkedOut {
		return nil
	}
	stray, err := l.git("rev-list", "--cherry-pick", "--right-only", "--no-merges", branch+"..."+localMain, "^"+base)
	if err != nil {
		return err
	}
	if stray != "" {
		shas := strings.Fields(stray)
		for i := range shas {
			shas[i] = short(shas[i])
		}
		if len(shas) > 3 {
			shas = append(shas[:3], "...")
		}
		return fmt.Errorf("local %s has commits that are not on %s and not on %s; move them to a branch first: %s",
			l.Main, l.Remote, branch, strings.Join(shas, " "))
	}
	switch {
	case localMain == base:
		l.logf("local %s matches %s/%s", l.Main, l.Remote, l.Main)
	case l.isAncestor(localMain, base):
		l.logf("local %s is behind %s/%s; it moves to the landed merge afterwards", l.Main, l.Remote, l.Main)
	default:
		l.logf("local %s holds only copies of %s's commits; they are replaced by the landed merge", l.Main, branch)
	}
	return nil
}

// mergeAndPush builds the --no-ff merge of branch onto base in a temporary
// worktree and pushes it. The worktree is removed however this returns.
func (l *lander) mergeAndPush(branch, base string, attempt int) (merged string, pushed bool, err error) {
	dir, err := os.MkdirTemp(l.TempDir, "release-damnit-land-")
	if err != nil {
		return "", false, err
	}
	defer l.removeWorktree(dir)

	if _, err := l.git("worktree", "add", "--quiet", "--detach", dir, base); err != nil {
		return "", false, err
	}
	if _, err := git.Output(dir, "merge", "--quiet", "--no-ff", "--no-edit", branch); err != nil {
		files, _ := git.OutputNUL(dir, "diff", "--name-only", "--diff-filter=U", "-z")
		_, _ = git.Output(dir, "merge", "--abort")
		if len(files) == 0 {
			return "", false, fmt.Errorf("merging %s onto %s/%s failed: %w", branch, l.Remote, l.Main, err)
		}
		return "", false, fmt.Errorf("%s does not merge cleanly into %s/%s; conflicts in %s. Rebase %s onto %s/%s, resolve, and land again",
			branch, l.Remote, l.Main, strings.Join(files, ", "), branch, l.Remote, l.Main)
	}
	merged, err = git.Output(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", false, err
	}
	l.logf("merged %s onto %s/%s as %s; pushing", branch, l.Remote, l.Main, short(merged))
	pushed, err = l.push(dir, base, attempt)
	return merged, pushed, err
}

func (l *lander) removeWorktree(dir string) {
	if _, err := l.git("worktree", "remove", "--force", dir); err != nil {
		l.logf("could not remove the temporary worktree %s: %v", dir, err)
	}
	_ = os.RemoveAll(dir)
	_, _ = l.git("worktree", "prune")
}

// updateLocalMain moves local main to the landed commit without losing any
// work: a ref update when main is not checked out, and otherwise a
// fast-forward in the worktree that has it, carrying its uncommitted changes
// forward (fastForward). When those changes would conflict, it leaves a note.
func (l *lander) updateLocalMain(landed string) {
	path, checkedOut, err := l.worktreeOf("refs/heads/" + l.Main)
	if err != nil {
		l.logf("could not list worktrees, so local %s was left as it is: %v", l.Main, err)
		return
	}
	if !checkedOut {
		old, _ := l.git("rev-parse", "--verify", "--quiet", "refs/heads/"+l.Main)
		if _, err := l.git("update-ref", "-m", "release-damnit land", "refs/heads/"+l.Main, landed, old); err != nil {
			l.logf("could not move local %s to %s: %v", l.Main, short(landed), err)
			return
		}
		l.logf("moved local %s to %s", l.Main, short(landed))
		return
	}

	head, err := git.Output(path, "rev-parse", "HEAD")
	if err != nil {
		l.logf("note: could not read %s at %s, so it was left as it is: %v", l.Main, path, err)
		return
	}
	if !l.isAncestor(head, landed) {
		n, _ := l.count(landed + ".." + head)
		l.logf("note: left %s at %s as it is: it has %d commit(s) that are not on %s/%s; land them", l.Main, path, n, l.Remote, l.Main)
		return
	}
	if err := l.fastForward(path, head, landed); err != nil {
		l.logf("note: left %s at %s as it is: %v. Later: release-damnit sync", l.Main, path, err)
		return
	}
	l.logf("fast-forwarded %s at %s to %s", l.Main, path, short(landed))
}

// worktreeOf returns the worktree that has ref checked out, if any.
func (l *lander) worktreeOf(ref string) (string, bool, error) {
	out, err := l.git("worktree", "list", "--porcelain")
	if err != nil {
		return "", false, err
	}
	var path string
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			path = p
		}
		if line == "branch "+ref {
			return path, true, nil
		}
	}
	return "", false, nil
}

func (l *lander) count(revRange string) (int, error) {
	out, err := l.git("rev-list", "--count", revRange)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

func (l *lander) isAncestor(ancestor, of string) bool {
	_, err := l.git("merge-base", "--is-ancestor", ancestor, of)
	return err == nil
}

func isDirty(dir string) (bool, error) {
	out, err := git.Output(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func short(sha string) string { return sha[:min(len(sha), 9)] }
