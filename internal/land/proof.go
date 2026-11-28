package land

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/proof"
)

// ErrCheckFailed is returned when the repository's check fails. Nothing was
// pushed.
var ErrCheckFailed = errors.New("the check failed, so nothing was pushed")

// prove attaches a landing proof to commit: it runs the repository's check
// in the first of dirs whose checkout is exactly commit's tree, signs the
// result, and writes the note. A tree an earlier attempt already checked
// keeps its proof, which then moves to the new commit. It reports false, and
// attaches nothing, when the repository names no check.
func (l *lander) prove(commit string, dirs ...string) (bool, error) {
	check, err := proof.CheckAt(l.root, commit)
	if err != nil {
		return false, err
	}
	if check == "" {
		l.logf("warning: release-please-config.json names no \"check\", so this landing carries no proof; a pipeline that runs release-damnit verify will refuse it")
		return false, nil
	}
	tree, err := l.git("rev-parse", commit+"^{tree}")
	if err != nil {
		return false, err
	}

	p, ok := l.proofs[tree]
	if ok && p.Statement.Check == check {
		l.logf("the check already passed on tree %s; its proof moves to %s", short(tree), short(commit))
	} else if l.Proven {
		return false, fmt.Errorf("no proof for tree %s, and a proven landing never runs the check", short(tree))
	} else {
		if l.signer == nil {
			signer, err := proof.GitSigner(l.root, l.TempDir)
			if err != nil {
				return false, err
			}
			l.signer = &signer
		}
		dir, err := checkoutOf(tree, dirs)
		if err != nil {
			return false, err
		}
		l.logf("running the check in %s: %s", dir, check)
		if err := runCheck(dir, check, l); err != nil {
			return false, fmt.Errorf("%w: %s: %v", ErrCheckFailed, check, err)
		}
		p, err = l.signer.Sign(proof.Statement{Tree: tree, Check: check, Result: proof.ResultPass, Time: time.Now()})
		if err != nil {
			return false, err
		}
		l.proofs[tree] = p
		l.logf("the check passed on tree %s; signed its proof as %s", short(tree), p.Statement.Signer)
	}
	if err := proof.Attach(l.root, commit, p, l.TempDir); err != nil {
		return false, err
	}
	return true, nil
}

// checkoutOf returns the first dir whose HEAD has tree and whose tracked
// files are unchanged, so the check sees exactly what lands.
func checkoutOf(tree string, dirs []string) (string, error) {
	for _, dir := range dirs {
		head, err := git.Output(dir, "rev-parse", "HEAD^{tree}")
		if err != nil || head != tree {
			continue
		}
		if dirty, err := isDirty(dir); err == nil && !dirty {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no checkout holds exactly tree %s to run the check in", short(tree))
}

// runCheck runs check with sh in dir, its output going to land's output.
// Git's own variables are dropped, so a check that runs git acts on dir.
func runCheck(dir, check string, l *lander) error {
	cmd := exec.Command("sh", "-c", check)
	cmd.Dir = dir
	cmd.Stdout = l.Out
	cmd.Stderr = l.Out
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); name != "GIT_DIR" && name != "GIT_WORK_TREE" && name != "GIT_INDEX_FILE" {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	return cmd.Run()
}
