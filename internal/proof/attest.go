package proof

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dsswift/release-damnit/internal/git"
)

// ErrNoCheck is returned when a proof is asked for a commit whose release
// config names no check: there is nothing it could say passed.
var ErrNoCheck = errors.New(`release-please-config.json names no "check"`)

// Attest signs a proof that the repository's check passed on commit's tree,
// with git's signing key in dir, and writes it as commit's note. It runs
// nothing: the caller ran the check elsewhere and vouches for it, as a
// lander whose check must run away from its keys does.
func Attest(dir, commit, tempDir string) (Proof, error) {
	check, err := CheckAt(dir, commit)
	if err != nil {
		return Proof{}, err
	}
	if check == "" {
		return Proof{}, ErrNoCheck
	}
	tree, err := git.Output(dir, "rev-parse", commit+"^{tree}")
	if err != nil {
		return Proof{}, err
	}
	signer, err := GitSigner(dir, tempDir)
	if err != nil {
		return Proof{}, err
	}
	p, err := signer.Sign(Statement{Tree: tree, Check: check, Result: ResultPass, Time: time.Now()})
	if err != nil {
		return Proof{}, err
	}
	if err := Attach(dir, commit, p, tempDir); err != nil {
		return Proof{}, err
	}
	return p, nil
}

// Holds checks that p is this signer's: it names the signer's fingerprint,
// and its signature verifies under the signer's public key for Namespace.
// A lander can check its own proofs this way without the estate's allowed
// signers.
func (sg Signer) Holds(p Proof, tempDir string) error {
	if p.Statement.Signer != sg.Principal {
		return fmt.Errorf("the proof is signed as %s, not by this lander's key %s", p.Statement.Signer, sg.Principal)
	}
	pub, err := publicKey(sg.Key)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(tempDir, "release-damnit-self-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = fmt.Fprintf(f, "%s namespaces=%q %s\n", sg.Principal, Namespace, pub)
	f.Close()
	if err != nil {
		return err
	}
	return Check(p, f.Name(), tempDir)
}

// publicKey is a key file's public half as "<type> <base64>": the file
// itself when it is a public key, otherwise what ssh-keygen derives from the
// private key.
func publicKey(key string) (string, error) {
	data, err := os.ReadFile(key)
	if err != nil {
		return "", fmt.Errorf("could not read the SSH key %s: %v", key, err)
	}
	text := string(data)
	if !strings.HasPrefix(strings.TrimSpace(text), "-----BEGIN") {
		if f := strings.Fields(text); len(f) >= 2 {
			return f[0] + " " + f[1], nil
		}
	}
	out, err := sshKeygen("", "-y", "-f", key)
	if err != nil {
		return "", fmt.Errorf("could not read the public half of the SSH key %s: %v", key, err)
	}
	f := strings.Fields(out)
	if len(f) < 2 {
		return "", fmt.Errorf("ssh-keygen printed no public key for %s", key)
	}
	return f[0] + " " + f[1], nil
}
