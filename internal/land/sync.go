package land

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dsswift/release-damnit/internal/git"
)

// SyncOptions configures a sync.
type SyncOptions struct {
	// Dir is any directory inside the repository.
	Dir string

	// Remote names the remote. Main names its main branch; empty means the
	// branch the remote's HEAD points at, or main.
	Remote string
	Main   string

	// Out receives progress lines.
	Out io.Writer
}

// SyncPrefix starts every line sync prints.
const SyncPrefix = "release-damnit sync: "

// Sync fetches the remote main and brings local main up to it. Where main is
// checked out, uncommitted work is carried forward (see fastForward).
func Sync(opts SyncOptions) error {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	root, err := git.Toplevel(opts.Dir)
	if err != nil {
		return err
	}
	if opts.Main == "" {
		opts.Main = "main"
		if head, err := git.Output(root, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+opts.Remote+"/HEAD"); err == nil {
			opts.Main = strings.TrimPrefix(head, opts.Remote+"/")
		}
	}
	l := &lander{
		Options: Options{Remote: opts.Remote, Main: opts.Main, Attempts: 1, Out: opts.Out},
		prefix:  SyncPrefix + filepath.Base(root) + ": ",
		root:    root,
		remote:  "refs/remotes/" + opts.Remote + "/" + opts.Main,
	}
	if err := git.Stream(root, l.Out, "fetch", "--quiet", l.Remote, "+refs/heads/"+l.Main+":"+l.remote); err != nil {
		return err
	}
	target, err := l.git("rev-parse", l.remote)
	if err != nil {
		return err
	}
	local, err := l.git("rev-parse", "--verify", "--quiet", "refs/heads/"+l.Main)
	if err != nil {
		l.logf("no local %s; nothing to sync", l.Main)
		return nil
	}
	switch {
	case local == target:
		l.logf("local %s matches %s/%s", l.Main, l.Remote, l.Main)
		return nil
	case l.isAncestor(target, local):
		n, _ := l.count(target + ".." + local)
		l.logf("local %s is %d commit(s) ahead of %s/%s; land them", l.Main, n, l.Remote, l.Main)
		return nil
	case !l.isAncestor(local, target):
		n, _ := l.count(target + ".." + local)
		l.logf("local %s has %d commit(s) that are not on %s/%s; land them, then sync", l.Main, n, l.Remote, l.Main)
		return nil
	}
	l.updateLocalMain(target)
	return nil
}

// carried is one uncommitted file that the incoming commits also change.
type carried struct {
	path   string
	before []byte // what the work tree held; nil when the file was absent
	mode   fs.FileMode
	after  []byte // what it holds once main has moved; nil when it goes away
}

// fastForward moves the main checkout at dir from head to target and keeps
// every uncommitted change. Changes to files the incoming commits leave
// alone stay as they are. A file both sides changed is merged three ways
// (the version it started from, the work tree, target); a copy of a
// version that landed merges to target.
// When any such file would conflict, is staged, or an operation such as a
// rebase is under way, nothing is touched and the reason is returned.
func (l *lander) fastForward(dir, head, target string) error {
	if op := inProgress(dir); op != "" {
		return fmt.Errorf("a %s is under way there", op)
	}
	incoming, err := git.OutputNUL(dir, "diff", "--no-renames", "--name-only", "-z", head, target)
	if err != nil {
		return err
	}
	tracked, err := git.OutputNUL(dir, "diff", "--no-renames", "--name-only", "-z", "HEAD")
	if err != nil {
		return err
	}
	staged, err := git.OutputNUL(dir, "diff", "--cached", "--no-renames", "--name-only", "-z")
	if err != nil {
		return err
	}
	untracked, err := git.OutputNUL(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}

	var plan []carried
	var refused []string
	for _, path := range incoming {
		isTracked, isUntracked := slices.Contains(tracked, path), slices.Contains(untracked, path)
		if !isTracked && !isUntracked {
			continue
		}
		if slices.Contains(staged, path) {
			refused = append(refused, path+" (staged)")
			continue
		}
		c, ok, err := mergeOne(dir, path, head, target)
		if err != nil {
			return err
		}
		if !ok {
			refused = append(refused, path)
			continue
		}
		plan = append(plan, c)
	}
	if len(refused) > 0 {
		return fmt.Errorf("uncommitted changes would conflict with the incoming commits in %s", strings.Join(refused, ", "))
	}

	// Put each carried file back as git expects it, move main, then write the
	// merged result. Any failure puts the files back as they were.
	restore := func() {
		for _, c := range plan {
			writeOrRemove(filepath.Join(dir, c.path), c.before, c.mode)
		}
	}
	for _, c := range plan {
		full := filepath.Join(dir, c.path)
		if c.before == nil {
			continue
		}
		if _, err := git.Output(dir, "cat-file", "-e", head+":"+c.path); err == nil {
			if _, err := git.Output(dir, "checkout", head, "--", c.path); err != nil {
				restore()
				return err
			}
		} else if err := os.Remove(full); err != nil {
			restore()
			return err
		}
	}
	if _, err := git.Output(dir, "merge", "--quiet", "--ff-only", target); err != nil {
		restore()
		return err
	}
	for _, c := range plan {
		full := filepath.Join(dir, c.path)
		mode := c.mode
		if info, err := os.Stat(full); err == nil {
			mode = info.Mode().Perm()
		}
		writeOrRemove(full, c.after, mode)
	}
	if len(plan) > 0 {
		l.logf("carried %d uncommitted file(s) forward over the incoming commits", len(plan))
	}
	return nil
}

// mergeOne works out what path holds after the move, or reports false when
// the uncommitted change cannot be carried without a conflict.
func mergeOne(dir, path, head, target string) (carried, bool, error) {
	c := carried{path: path, mode: 0o644}
	full := filepath.Join(dir, path)
	if info, err := os.Lstat(full); err == nil {
		if !info.Mode().IsRegular() {
			return c, false, nil
		}
		c.mode = info.Mode().Perm()
		if c.before, err = os.ReadFile(full); err != nil {
			return c, false, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return c, false, err
	}
	theirs, theirsOK := blob(dir, target, path)
	if c.before == nil { // deleted here
		return c, !theirsOK, nil
	}
	if !theirsOK { // deleted upstream, changed here
		return c, false, nil
	}
	base, baseOK, err := startingPoint(dir, path, head, target, c.before)
	if err != nil {
		return c, false, err
	}
	if !baseOK { // added on both sides
		c.after = theirs
		return c, bytes.Equal(c.before, theirs), nil
	}
	merged, clean, err := mergeFile(c.before, base, theirs)
	if err != nil {
		return c, false, err
	}
	c.after = merged
	return c, clean, nil
}

// startingPoint returns the version of path the work tree's copy started
// from. A checkout left behind holds copies of files written for commits
// that landed later; against head those read as edits that undo nothing and
// redo what landed. So it is the newest incoming version of path whose
// change from head the copy already holds, or head's own when none is.
func startingPoint(dir, path, head, target string, local []byte) ([]byte, bool, error) {
	base, baseOK := blob(dir, head, path)
	revs, err := git.OutputNUL(dir, "log", "-z", "--format=%H", head+".."+target, "--", path)
	if err != nil {
		return nil, false, err
	}
	for _, rev := range revs {
		version, ok := blob(dir, rev, path)
		if !ok {
			continue
		}
		if !baseOK {
			if bytes.Equal(local, version) {
				return version, true, nil
			}
			continue
		}
		merged, clean, err := mergeFile(local, base, version)
		if err != nil {
			return nil, false, err
		}
		if clean && bytes.Equal(merged, local) {
			return version, true, nil
		}
	}
	return base, baseOK, nil
}

func blob(dir, rev, path string) ([]byte, bool) {
	cmd := exec.Command("git", "cat-file", "blob", rev+":"+path)
	cmd.Dir = dir
	out, err := cmd.Output()
	return out, err == nil
}

// mergeFile merges the change from base to ours onto theirs with git
// merge-file, and reports whether it merged without a conflict.
func mergeFile(ours, base, theirs []byte) ([]byte, bool, error) {
	tmp, err := os.MkdirTemp("", "release-damnit-sync-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(tmp)
	names := []string{"ours", "base", "theirs"}
	for i, content := range [][]byte{ours, base, theirs} {
		if err := os.WriteFile(filepath.Join(tmp, names[i]), content, 0o600); err != nil {
			return nil, false, err
		}
	}
	cmd := exec.Command("git", "merge-file", "-p", "--quiet", "ours", "base", "theirs")
	cmd.Dir = tmp
	out, err := cmd.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out, true, nil
	case errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() < 128:
		return nil, false, nil // that many conflicts
	default:
		return nil, false, fmt.Errorf("git merge-file: %w", err)
	}
}

func writeOrRemove(full string, content []byte, mode fs.FileMode) {
	if content == nil {
		_ = os.Remove(full)
		return
	}
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	_ = os.WriteFile(full, content, mode)
	_ = os.Chmod(full, mode)
}

// inProgress names a git operation under way in dir's worktree, if any.
func inProgress(dir string) string {
	for _, op := range []struct{ path, name string }{
		{"index.lock", "commit or other git command"},
		{"MERGE_HEAD", "merge"},
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
		{"BISECT_LOG", "bisect"},
	} {
		p, err := git.Output(dir, "rev-parse", "--git-path", op.path)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if _, err := os.Stat(p); err == nil {
			return op.name
		}
	}
	return ""
}
