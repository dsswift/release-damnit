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

// maxMachineCommits bounds the walk down through machine commits.
const maxMachineCommits = 50

// ErrNoProof is returned when the commit being released has no proof.
var ErrNoProof = errors.New("no landing proof")

// Verify checks that the commit being released carries a landing proof: a
// note signed by an allowed signer, for exactly that commit's tree. A
// machine commit (one parent, changing only release files and the config's
// machine-paths, such as CI's version commit) needs none: Verify checks the
// commit under it.
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
	owned, err := machineOwned(root, opts.Allow)
	if err != nil {
		return Verified{}, err
	}

	commit := released
	for range maxMachineCommits {
		note, ok := Read(root, commit)
		if ok {
			return check(root, commit, released, note, opts)
		}
		ok, err := isMachineCommit(root, commit, owned)
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
	return Verified{}, fmt.Errorf("%w: more than %d machine commits sit under %s with no landed commit below them", ErrNoProof, maxMachineCommits, short(released))
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

// isMachineCommit reports whether commit is one only machines make: one
// parent, and every file it changes is machine owned.
func isMachineCommit(root, commit string, owned owned) (bool, error) {
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
		if !owned.has(f) {
			return false, nil
		}
	}
	return len(changed) > 0, nil
}

// owned is what machines write: exact files, and folders by prefix.
type owned struct {
	files   map[string]bool
	folders []string
}

func (o owned) has(path string) bool {
	if o.files[path] {
		return true
	}
	for _, f := range o.folders {
		if strings.HasPrefix(path, f) {
			return true
		}
	}
	return false
}

// machineOwned is every path a commit with no proof may change: the release
// files from the repository's release config, its machine-paths, and allow.
// A path ending in "/" is a folder. The config is read from the checkout, and
// a commit that changes it is never machine owned, so no commit under the
// proof can widen the list.
func machineOwned(root string, allow []string) (owned, error) {
	o := owned{files: map[string]bool{config.ManifestFile: true}}
	paths := allow
	if _, err := os.Stat(filepath.Join(root, config.ConfigFile)); err == nil {
		cfg, err := config.Load(root)
		if err != nil {
			return owned{}, err
		}
		o.files = cfg.ReleaseFiles()
		paths = append(append([]string(nil), cfg.MachinePaths...), allow...)
	}
	for _, p := range paths {
		folder := strings.HasSuffix(p, "/")
		clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./")
		if clean == "." || clean == config.ConfigFile {
			return owned{}, fmt.Errorf("machine path %q would let a commit with no proof change what verify trusts", p)
		}
		if folder {
			o.folders = append(o.folders, clean+"/")
		} else {
			o.files[clean] = true
		}
	}
	return o, nil
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
