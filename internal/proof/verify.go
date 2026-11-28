package proof

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
)

// VerifyOptions configures Verify.
type VerifyOptions struct {
	// Dir is any directory inside the repository.
	Dir string
	// Commit is the commit being released. Empty means HEAD.
	Commit string
	// Remote is fetched for its proofs first. Empty fetches nothing.
	Remote string
	// AllowedSigners is the path of the allowed signers file.
	AllowedSigners string
	// Allow are further paths a version commit may change: what a pipeline's
	// before-publish step adds to it.
	Allow []string
	// TempDir holds the signature file. Empty means os.TempDir().
	TempDir string
}

// Verified is a commit whose landing proof holds.
type Verified struct {
	// Commit has the proof; Released is the commit asked about, which may sit
	// above Commit on version commits.
	Commit, Released string
	Proof            Proof
}

// maxVersionCommits bounds the walk down through version commits.
const maxVersionCommits = 20

// ErrNoProof is returned when the commit being released has no proof.
var ErrNoProof = errors.New("no landing proof")

// Verify checks that the commit being released carries a landing proof: a
// note signed by an allowed signer, for exactly that commit's tree. A
// version commit CI made (subject "chore: release ...", one parent, only
// release files changed) needs none: Verify checks the commit under it.
func Verify(opts VerifyOptions) (Verified, error) {
	if opts.Commit == "" {
		opts.Commit = "HEAD"
	}
	if opts.TempDir == "" {
		opts.TempDir = os.TempDir()
	}
	if opts.AllowedSigners == "" {
		return Verified{}, errors.New("no allowed signers: pass --allowed-signers <file> or set RELEASE_DAMNIT_ALLOWED_SIGNERS")
	}
	root, err := git.Toplevel(opts.Dir)
	if err != nil {
		return Verified{}, err
	}
	if opts.Remote != "" {
		if _, err := FetchNotes(root, opts.Remote); err != nil {
			return Verified{}, fmt.Errorf("could not fetch the landing proofs from %s: %w", opts.Remote, err)
		}
	}
	released, err := git.Output(root, "rev-parse", "--verify", "--quiet", opts.Commit+"^{commit}")
	if err != nil {
		return Verified{}, fmt.Errorf("no commit %s", opts.Commit)
	}
	allowed, err := releaseFiles(root, opts.Allow)
	if err != nil {
		return Verified{}, err
	}

	commit := released
	for range maxVersionCommits {
		note, ok := Read(root, commit)
		if ok {
			return check(root, commit, released, note, opts)
		}
		ok, err := isVersionCommit(root, commit, allowed)
		if err != nil {
			return Verified{}, err
		}
		if !ok {
			return Verified{}, fmt.Errorf("%w on %s (%s): it was pushed without release-damnit land, or its proof was not pushed. Land it with release-damnit land, which runs the check and signs the proof", ErrNoProof, short(commit), subject(root, commit))
		}
		commit, err = git.Output(root, "rev-parse", commit+"^")
		if err != nil {
			return Verified{}, err
		}
	}
	return Verified{}, fmt.Errorf("%w: more than %d version commits sit on %s with no landed commit under them", ErrNoProof, maxVersionCommits, short(released))
}

// ErrWrongTree is returned when a proof is for another tree than its commit's.
var ErrWrongTree = errors.New("the landing proof is for a different tree")

func check(root, commit, released, note string, opts VerifyOptions) (Verified, error) {
	p, err := Parse(note)
	if err != nil {
		return Verified{}, fmt.Errorf("the landing proof on %s is unreadable: %w", short(commit), err)
	}
	tree, err := git.Output(root, "rev-parse", commit+"^{tree}")
	if err != nil {
		return Verified{}, err
	}
	if p.Statement.Tree != tree {
		return Verified{}, fmt.Errorf("%w: the proof on %s says the check passed on tree %s, but the commit's tree is %s. What landed is not what was checked", ErrWrongTree, short(commit), short(p.Statement.Tree), short(tree))
	}
	want, err := CheckAt(root, commit)
	if err != nil {
		return Verified{}, err
	}
	if p.Statement.Check != want {
		return Verified{}, fmt.Errorf("the landing proof on %s ran %q, but the repository's check at that commit is %q", short(commit), p.Statement.Check, want)
	}
	if err := Check(p, opts.AllowedSigners, opts.TempDir); err != nil {
		return Verified{}, fmt.Errorf("the landing proof on %s does not hold: %w", short(commit), err)
	}
	return Verified{Commit: commit, Released: released, Proof: p}, nil
}

// isVersionCommit reports whether commit is a version commit CI made: one
// parent, the subject publish writes, and only release files changed.
func isVersionCommit(root, commit string, allowed map[string]bool) (bool, error) {
	if !strings.HasPrefix(subject(root, commit), "chore: release ") {
		return false, nil
	}
	parents, err := git.Output(root, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return false, err
	}
	if len(strings.Fields(parents)) != 2 {
		return false, nil
	}
	changed, err := git.OutputNUL(root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", commit)
	if err != nil {
		return false, err
	}
	for _, f := range changed {
		if !allowed[f] {
			return false, nil
		}
	}
	return true, nil
}

// releaseFiles is every file a version commit may change, from the
// repository's release config, plus allow (relative to the repository root).
func releaseFiles(root string, allow []string) (map[string]bool, error) {
	files := map[string]bool{config.ManifestFile: true}
	if _, err := os.Stat(filepath.Join(root, config.ConfigFile)); err == nil {
		cfg, err := config.Load(root)
		if err != nil {
			return nil, err
		}
		files = cfg.ReleaseFiles()
	}
	for _, a := range allow {
		files[strings.TrimPrefix(filepath.ToSlash(filepath.Clean(a)), "./")] = true
	}
	return files, nil
}

// CheckAt is the check command the repository names at rev, or "" when its
// release config there names none.
func CheckAt(dir, rev string) (string, error) {
	path := rev + ":" + config.ConfigFile
	if _, err := git.Output(dir, "cat-file", "-e", path); err != nil {
		return "", nil
	}
	data, err := git.Output(dir, "cat-file", "blob", path)
	if err != nil {
		return "", err
	}
	return config.ParseCheck([]byte(data))
}

func subject(root, commit string) string {
	s, _ := git.Output(root, "log", "-1", "--format=%s", commit)
	return s
}

func short(sha string) string { return sha[:min(len(sha), 9)] }
