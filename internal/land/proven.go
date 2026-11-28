package land

import (
	"errors"
	"fmt"

	"github.com/dsswift/release-damnit/internal/proof"
)

// ErrMainMoved is returned by a proven landing when the remote main has
// commits HEAD lacks. Nothing was pushed. The caller rebases, runs the check
// on the new tree where it runs checks, signs it (proof.Attest), and lands
// again.
var ErrMainMoved = errors.New("the remote main moved, so the proven commit no longer lands as it is")

// landProven lands main's local commits with the proof HEAD carries. It
// runs no check, so it needs a proof made where the check ran: one for
// HEAD's tree, naming the repository's check at HEAD, signed by this
// lander's own key. A proof read before the first fetch is held in memory,
// since each fetch makes the local proofs a copy of the remote's.
func (l *lander) landProven(current string, dirty bool) error {
	if l.Branch != "" || current != l.Main || dirty {
		return fmt.Errorf("a proven landing lands %s's own commits: check out %s with no uncommitted changes and name no branch", l.Main, l.Main)
	}
	if err := l.loadProof(); err != nil {
		return err
	}
	l.logf("on %s: landing its local commits with the proof HEAD carries; the check does not run here", l.Main)
	return l.landMain()
}

func (l *lander) loadProof() error {
	head, err := l.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	check, err := proof.CheckAt(l.root, head)
	if err != nil {
		return err
	}
	if check == "" {
		return nil // prove warns that the landing carries no proof
	}
	note, ok := proof.Read(l.root, head)
	if !ok {
		return fmt.Errorf("HEAD (%s) carries no landing proof; run the check, then sign it with release-damnit attest", short(head))
	}
	p, err := proof.Parse(note)
	if err != nil {
		return fmt.Errorf("the landing proof on HEAD is unreadable: %w", err)
	}
	tree, err := l.git("rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if p.Statement.Tree != tree {
		return fmt.Errorf("%w: the proof on HEAD is for tree %s, but HEAD's tree is %s", proof.ErrWrongTree, short(p.Statement.Tree), short(tree))
	}
	if p.Statement.Check != check {
		return fmt.Errorf("the proof on HEAD says %q passed, but the repository's check is %q", p.Statement.Check, check)
	}
	signer, err := proof.GitSigner(l.root, l.TempDir)
	if err != nil {
		return err
	}
	if err := signer.Holds(p, l.TempDir); err != nil {
		return fmt.Errorf("the proof on HEAD does not hold: %w", err)
	}
	l.signer = &signer
	l.proofs[tree] = p
	return nil
}

func (l *lander) mainMoved(now string) error {
	return fmt.Errorf("%w: %s/%s is at %s; nothing was pushed", ErrMainMoved, l.Remote, l.Main, short(now))
}
