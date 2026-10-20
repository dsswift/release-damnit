package commitcheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
)

// LoadRules returns the rules for the repo containing dir: from its
// release-please config when it has one, else the defaults.
func LoadRules(dir string) (Rules, string, error) {
	root, err := git.Toplevel(dir)
	if err != nil {
		return Rules{}, "", err
	}
	if _, err := os.Stat(filepath.Join(root, config.ConfigFile)); errors.Is(err, os.ErrNotExist) {
		return RulesFor(nil), root, nil
	}
	cfg, err := config.Load(root)
	if err != nil {
		return Rules{}, "", err
	}
	return RulesFor(cfg), root, nil
}

// CheckStaged checks the message in msgFile against the staged changes, as a
// commit-msg hook sees them. Deletions and both sides of a rename count.
func CheckStaged(dir, msgFile string) (Result, error) {
	rules, root, err := LoadRules(dir)
	if err != nil {
		return Result{}, err
	}
	message, err := os.ReadFile(msgFile)
	if err != nil {
		return Result{}, fmt.Errorf("read the commit message: %w", err)
	}
	files, err := git.OutputNUL(root, "diff", "--cached", "--name-only", "--no-renames", "-z")
	if err != nil {
		return Result{}, err
	}
	return Check(rules, string(message), files), nil
}

// CommitResult is the check of one commit in a range.
type CommitResult struct {
	SHA string
	Result
}

// Short returns the abbreviated SHA.
func (c CommitResult) Short() string { return c.SHA[:min(len(c.SHA), 9)] }

// CheckRange checks every non-merge commit in revRange, oldest first.
func CheckRange(dir, revRange string) ([]CommitResult, error) {
	rules, root, err := LoadRules(dir)
	if err != nil {
		return nil, err
	}
	out, err := git.Output(root, "rev-list", "--no-merges", "--topo-order", "--reverse", revRange, "--")
	if err != nil {
		return nil, fmt.Errorf("list commits in %s: %w", revRange, err)
	}
	var results []CommitResult
	for _, sha := range strings.Fields(out) {
		message, err := git.Output(root, "log", "-1", "--format=%B", sha)
		if err != nil {
			return nil, err
		}
		files, err := git.OutputNUL(root, "diff-tree", "--no-commit-id", "--name-only", "-r", "--root", "--no-renames", "-z", sha)
		if err != nil {
			return nil, err
		}
		results = append(results, CommitResult{SHA: sha, Result: Check(rules, message, files)})
	}
	return results, nil
}

// Failures returns the commits in results that broke a rule.
func Failures(results []CommitResult) []CommitResult {
	var failed []CommitResult
	for _, r := range results {
		if !r.OK() {
			failed = append(failed, r)
		}
	}
	return failed
}
