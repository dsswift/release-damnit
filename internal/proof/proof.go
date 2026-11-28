// Package proof is the landing proof: a statement that a repository's check
// passed on one tree, signed with the lander's SSH key the way git signs with
// SSH (ssh-keygen -Y sign), and kept as a git note on the landed commit under
// NotesRef. The note is the statement's text followed by its armored
// signature; the signature covers the statement's text exactly.
package proof

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dsswift/release-damnit/internal/git"
)

const (
	// NotesRef holds every landing proof, one note per landed commit.
	NotesRef = "refs/notes/release-damnit"

	// Namespace is the SSH signature namespace of a landing proof, so a
	// signature made for anything else (a commit, a file) never passes as one.
	Namespace = "release-damnit-landing"

	// ResultPass is the only result a proof records: a check that fails
	// lands nothing, so nothing is signed for it.
	ResultPass = "pass"

	header   = "release-damnit landing proof v1"
	sigBegin = "-----BEGIN SSH SIGNATURE-----"
)

// Statement is what a landing proof says.
type Statement struct {
	// Tree is the git tree hash the check passed on: exactly what landed.
	Tree string
	// Check is the command that ran, from the repository's config.
	Check string
	// Result is ResultPass.
	Result string
	// Time is when the check finished, in UTC.
	Time time.Time
	// Signer is the principal the allowed signers list the key under: the
	// key's SHA256 fingerprint, which names the key and no person.
	Signer string
}

var fields = []string{"tree", "check", "result", "time", "signer"}

// Text is the statement as signed: a header line, then one "key value" line
// per field in a fixed order.
func (s Statement) Text() string {
	values := []string{s.Tree, s.Check, s.Result, s.Time.UTC().Format(time.RFC3339), s.Signer}
	var b strings.Builder
	b.WriteString(header + "\n")
	for i, f := range fields {
		b.WriteString(f + " " + values[i] + "\n")
	}
	return b.String()
}

// Proof is a signed statement.
type Proof struct {
	Statement Statement
	// Signature is the armored SSH signature over Statement.Text().
	Signature string
}

// Note is the proof as stored in its git note.
func (p Proof) Note() string { return p.Statement.Text() + p.Signature }

// Parse reads a note written by Note. The statement must be in the exact
// form Text writes, so the fields read are the bytes the signature covers.
func Parse(note string) (Proof, error) {
	text, sig, ok := strings.Cut(note, sigBegin)
	if !ok {
		return Proof{}, errors.New("the proof has no signature")
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) != len(fields)+1 || lines[0] != header {
		return Proof{}, errors.New("the proof is not a release-damnit landing proof v1")
	}
	values := make([]string, len(fields))
	for i, f := range fields {
		v, ok := strings.CutPrefix(lines[i+1], f+" ")
		if !ok || v == "" || v != strings.TrimSpace(v) {
			return Proof{}, fmt.Errorf("the proof's line %d should be %q followed by a value", i+2, f)
		}
		values[i] = v
	}
	at, err := time.Parse(time.RFC3339, values[3])
	if err != nil {
		return Proof{}, fmt.Errorf("the proof's time %q is not RFC 3339", values[3])
	}
	p := Proof{
		Statement: Statement{Tree: values[0], Check: values[1], Result: values[2], Time: at, Signer: values[4]},
		Signature: sigBegin + sig,
	}
	if p.Statement.Text() != text {
		return Proof{}, errors.New("the proof's statement is not in its canonical form")
	}
	if p.Statement.Result != ResultPass {
		return Proof{}, fmt.Errorf("the proof records the result %q, not %q", p.Statement.Result, ResultPass)
	}
	return p, nil
}

// Signer signs statements with an SSH key.
type Signer struct {
	// Key is the key file ssh-keygen signs with: a private key, or a public
	// key whose private half is in the SSH agent.
	Key string
	// Principal is the identity the statement names as its signer: the
	// key's fingerprint (Fingerprint).
	Principal string
}

// ErrNoSigningKey is returned when git names no SSH signing key.
var ErrNoSigningKey = errors.New("no SSH signing key: run git config --global gpg.format ssh and git config --global user.signingkey <path to your SSH public key>")

// GitSigner is the signer git's own SSH signing would use in dir: the key in
// user.signingkey (with gpg.format ssh), under the principal of its
// fingerprint. A literal key ("key::ssh-ed25519 ...") is written to a file
// in tempDir.
func GitSigner(dir, tempDir string) (Signer, error) {
	key, _ := git.Output(dir, "config", "--get", "user.signingkey")
	if key == "" {
		return Signer{}, ErrNoSigningKey
	}
	if format, _ := git.Output(dir, "config", "--get", "gpg.format"); format != "ssh" {
		return Signer{}, fmt.Errorf("user.signingkey is set, but gpg.format is %q, not ssh; landing proofs are signed with SSH keys: git config --global gpg.format ssh", format)
	}
	if literal, ok := strings.CutPrefix(key, "key::"); ok {
		f, err := os.CreateTemp(tempDir, "release-damnit-key-*.pub")
		if err != nil {
			return Signer{}, err
		}
		defer f.Close()
		if _, err := f.WriteString(literal + "\n"); err != nil {
			return Signer{}, err
		}
		key = f.Name()
	} else if rest, ok := strings.CutPrefix(key, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return Signer{}, err
		}
		key = filepath.Join(home, rest)
	}
	fingerprint, err := Fingerprint(key)
	if err != nil {
		return Signer{}, err
	}
	return Signer{Key: key, Principal: fingerprint}, nil
}

// Fingerprint is the SHA256 fingerprint of an SSH key file, public or
// private, as ssh-keygen prints it: "SHA256:..." . It is the principal a
// landing key is listed under in the allowed signers.
func Fingerprint(key string) (string, error) {
	out, err := sshKeygen("", "-l", "-E", "sha256", "-f", key)
	if err != nil {
		return "", fmt.Errorf("could not read the SSH key %s: %v", key, err)
	}
	fields := strings.Fields(out)
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "SHA256:") {
		return "", fmt.Errorf("ssh-keygen printed no fingerprint for %s: %q", key, out)
	}
	return fields[1], nil
}

// Sign signs s and returns the proof.
func (sg Signer) Sign(s Statement) (Proof, error) {
	s.Signer = sg.Principal
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("ssh-keygen", "-Y", "sign", "-f", sg.Key, "-n", Namespace)
	cmd.Stdin = strings.NewReader(s.Text())
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Proof{}, fmt.Errorf("ssh-keygen could not sign with %s: %v: %s", sg.Key, err, strings.TrimSpace(stderr.String()))
	}
	return Proof{Statement: s, Signature: stdout.String()}, nil
}

// ErrUnknownSigner is returned when the allowed signers do not list the key
// that signed a proof.
var ErrUnknownSigner = errors.New("the proof is signed by a key the allowed signers do not list")

// Check verifies p's signature against an allowed signers file (the format
// of ssh-keygen's ALLOWED SIGNERS section) and returns nil when a key listed
// for the statement's signer made it, for this namespace.
func Check(p Proof, allowedSigners, tempDir string) error {
	sig, err := os.CreateTemp(tempDir, "release-damnit-proof-*.sig")
	if err != nil {
		return err
	}
	defer os.Remove(sig.Name())
	if _, err := sig.WriteString(p.Signature); err != nil {
		sig.Close()
		return err
	}
	sig.Close()

	out, err := sshKeygen("", "-Y", "find-principals", "-s", sig.Name(), "-f", allowedSigners)
	if err != nil {
		return ErrUnknownSigner
	}
	principals := strings.Fields(out)
	listed := false
	for _, pr := range principals {
		listed = listed || pr == p.Statement.Signer
	}
	if !listed {
		return fmt.Errorf("the proof names %s as its signer, but the allowed signers list its key only for %s", p.Statement.Signer, strings.Join(principals, ", "))
	}
	if _, err := sshKeygen(p.Statement.Text(), "-Y", "verify", "-f", allowedSigners, "-I", p.Statement.Signer, "-n", Namespace, "-s", sig.Name()); err != nil {
		return fmt.Errorf("the proof's signature does not verify for %s: %v", p.Statement.Signer, err)
	}
	return nil
}

func sshKeygen(stdin string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("ssh-keygen", args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()+" "+stdout.String()))
	}
	return stdout.String(), nil
}

// Read returns the proof note on commit, and false when it has none.
func Read(dir, commit string) (string, bool) {
	note, err := git.Output(dir, "notes", "--ref", NotesRef, "show", commit)
	if err != nil {
		return "", false
	}
	return note + "\n", true
}

// Attach writes p as the note on commit, replacing any note there.
func Attach(dir, commit string, p Proof, tempDir string) error {
	f, err := os.CreateTemp(tempDir, "release-damnit-note-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(p.Note()); err != nil {
		f.Close()
		return err
	}
	f.Close()
	_, err = git.Output(dir, "notes", "--ref", NotesRef, "add", "--force", "--file", f.Name(), commit)
	return err
}

// FetchNotes makes the local NotesRef a copy of the remote's and returns the
// remote's SHA for it, or "" when the remote has no proofs yet (the local
// ref is then removed, so a new note starts from nothing).
func FetchNotes(dir, remote string) (string, error) {
	out, err := git.Output(dir, "ls-remote", remote, NotesRef)
	if err != nil {
		return "", err
	}
	sha, _, _ := strings.Cut(out, "\t")
	if sha == "" {
		_, _ = git.Output(dir, "update-ref", "-d", NotesRef)
		return "", nil
	}
	if _, err := git.Output(dir, "fetch", "--quiet", remote, "+"+NotesRef+":"+NotesRef); err != nil {
		return "", err
	}
	return sha, nil
}
