package proof

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
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

// key makes an SSH key with no passphrase and returns its private key path
// and public key line.
func key(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test", "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	return path, strings.TrimSpace(string(pub))
}

func allowedSigners(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "allowed_signers")
	write(t, filepath.Dir(path), filepath.Base(path), strings.Join(lines, "\n")+"\n")
	return path
}

const signer = "user@example.com"

// repo is a git repository whose release config names the check "make check".
type repo struct {
	dir     string
	signer  Signer
	allowed string
}

func newRepo(t *testing.T) repo {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "init", "-q", "--initial-branch=main")
	run(t, dir, "config", "user.email", signer)
	run(t, dir, "config", "user.name", "Test")
	write(t, dir, "release-please-config.json", `{"check": "make check", "packages": {".": {"component": "app"}}}`)
	write(t, dir, "release-please-manifest.json", `{".": "1.0.0"}`)
	write(t, dir, "VERSION", "1.0.0\n")
	commit(t, dir, "main.go", "package main\n", "feat: start")
	priv, pub := key(t)
	return repo{
		dir:     dir,
		signer:  Signer{Key: priv, Principal: signer},
		allowed: allowedSigners(t, signer+` namespaces="`+Namespace+`" `+pub),
	}
}

func (r repo) prove(t *testing.T, commit, tree, check string) {
	t.Helper()
	if tree == "" {
		tree = run(t, r.dir, "rev-parse", commit+"^{tree}")
	}
	p, err := r.signer.Sign(Statement{Tree: tree, Check: check, Result: ResultPass, Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := Attach(r.dir, commit, p, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func (r repo) verify(t *testing.T, allow ...string) (Verified, error) {
	t.Helper()
	return Verify(VerifyOptions{Dir: r.dir, AllowedSigners: r.allowed, Allow: allow, TempDir: t.TempDir()})
}

func TestParse_RoundTripsANote(t *testing.T) {
	r := newRepo(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p, err := r.signer.Sign(Statement{Tree: "abc", Check: "make check", Result: ResultPass, Time: at})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(p.Note())
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, p.Note())
	}
	if got.Statement != p.Statement || got.Signature != p.Signature {
		t.Errorf("round trip changed the proof:\n got %+v\nwant %+v", got, p)
	}
	if got.Statement.Signer != signer {
		t.Errorf("signer = %q, want the signer's principal %q", got.Statement.Signer, signer)
	}
}

func TestParse_RefusesWhatIsNotAProof(t *testing.T) {
	r := newRepo(t)
	p, err := r.signer.Sign(Statement{Tree: "abc", Check: "make check", Result: ResultPass, Time: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for name, note := range map[string]string{
		"no signature":   p.Statement.Text(),
		"no header":      strings.Replace(p.Note(), "landing proof v1", "landing proof v2", 1),
		"extra line":     strings.Replace(p.Note(), "signer ", "extra x\nsigner ", 1),
		"failed result":  strings.Replace(p.Note(), "result pass", "result fail", 1),
		"loose spacing":  strings.Replace(p.Note(), "tree abc", "tree  abc", 1),
		"time not RFC 3": strings.Replace(p.Note(), "time 20", "time x20", 1),
	} {
		if _, err := Parse(note); err == nil {
			t.Errorf("%s: Parse accepted\n%s", name, note)
		}
	}
}

func TestVerify_AcceptsAGoodProof(t *testing.T) {
	r := newRepo(t)
	head := run(t, r.dir, "rev-parse", "HEAD")
	r.prove(t, head, "", "make check")
	v, err := r.verify(t)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if v.Commit != head || v.Released != head || v.Proof.Statement.Check != "make check" {
		t.Errorf("verified %+v, want the proof on %s", v, head)
	}
}

func TestVerify_RefusesACommitWithNoProof(t *testing.T) {
	r := newRepo(t)
	_, err := r.verify(t)
	if !errors.Is(err, ErrNoProof) {
		t.Fatalf("err = %v, want ErrNoProof", err)
	}
	if !strings.Contains(err.Error(), "release-damnit land") {
		t.Errorf("the message does not say how to land: %v", err)
	}
}

func TestVerify_RefusesAProofForAnotherTree(t *testing.T) {
	r := newRepo(t)
	checked := run(t, r.dir, "rev-parse", "HEAD^{tree}")
	head := commit(t, r.dir, "main.go", "package main // changed after the check\n", "fix: change")
	r.prove(t, head, checked, "make check")
	if _, err := r.verify(t); !errors.Is(err, ErrWrongTree) {
		t.Fatalf("err = %v, want ErrWrongTree", err)
	}
}

func TestVerify_RefusesAnUnknownSigner(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "make check")
	_, otherPub := key(t)
	r.allowed = allowedSigners(t, signer+" "+otherPub)
	if _, err := r.verify(t); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("err = %v, want ErrUnknownSigner", err)
	}
}

func TestVerify_RefusesASignerNamedForAnotherPrincipal(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "make check")
	pub := strings.TrimPrefix(strings.SplitN(readFile(t, r.allowed), " namespaces", 2)[1], `="`+Namespace+`" `)
	r.allowed = allowedSigners(t, "someone-else@example.com "+strings.TrimSpace(pub))
	_, err := r.verify(t)
	if err == nil || !strings.Contains(err.Error(), "someone-else@example.com") {
		t.Fatalf("err = %v, want it to name the principal the key is listed for", err)
	}
}

func TestVerify_RefusesATamperedStatement(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "make check")
	note, _ := Read(r.dir, "HEAD")
	write(t, r.dir, "../note", strings.Replace(note, "time 20", "time 19", 1))
	run(t, r.dir, "notes", "--ref", NotesRef, "add", "-f", "-F", "../note", "HEAD")
	_, err := r.verify(t)
	if err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("err = %v, want a signature failure", err)
	}
}

func TestVerify_RefusesASignatureFromAnotherNamespace(t *testing.T) {
	r := newRepo(t)
	tree := run(t, r.dir, "rev-parse", "HEAD^{tree}")
	st := Statement{Tree: tree, Check: "make check", Result: ResultPass, Time: time.Now(), Signer: signer}
	cmd := exec.Command("ssh-keygen", "-Y", "sign", "-f", r.signer.Key, "-n", "git")
	cmd.Stdin = strings.NewReader(st.Text())
	sig, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := Attach(r.dir, "HEAD", Proof{Statement: st, Signature: string(sig)}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// Listing the key for every namespace leaves only the namespace to refuse it.
	pub := strings.Fields(readFile(t, r.allowed))
	r.allowed = allowedSigners(t, signer+" "+pub[2]+" "+pub[3])
	if _, err := r.verify(t); err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("err = %v, want a signature made for git refused", err)
	}
}

func TestVerify_RefusesAProofOfAnotherCheck(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "true")
	_, err := r.verify(t)
	if err == nil || !strings.Contains(err.Error(), `"make check"`) {
		t.Fatalf("err = %v, want a proof of another command refused", err)
	}
}

func TestVerify_ChecksTheLandedCommitUnderVersionCommits(t *testing.T) {
	r := newRepo(t)
	landed := run(t, r.dir, "rev-parse", "HEAD")
	r.prove(t, landed, "", "make check")
	write(t, r.dir, "release-please-manifest.json", `{".": "1.1.0"}`)
	write(t, r.dir, "CHANGELOG.md", "# 1.1.0\n")
	commit(t, r.dir, "VERSION", "1.1.0\n", "chore: release v1.1.0 [skip ci]")
	v, err := r.verify(t)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if v.Commit != landed {
		t.Errorf("verified %s, want the landed commit %s under the version commit", v.Commit, landed)
	}
}

func TestVerify_RefusesAVersionCommitThatChangesCode(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "make check")
	write(t, r.dir, "VERSION", "1.1.0\n")
	commit(t, r.dir, "main.go", "package main // sneaked in\n", "chore: release v1.1.0 [skip ci]")
	if _, err := r.verify(t); !errors.Is(err, ErrNoProof) {
		t.Fatalf("err = %v, want ErrNoProof", err)
	}
}

func TestVerify_AllowsWhatBeforePublishAdds(t *testing.T) {
	r := newRepo(t)
	r.prove(t, "HEAD", "", "make check")
	write(t, r.dir, "VERSION", "1.1.0\n")
	commit(t, r.dir, "schema/api.graphql", "{}\n", "chore: release v1.1.0 [skip ci]")
	if _, err := r.verify(t); !errors.Is(err, ErrNoProof) {
		t.Fatalf("without --allow: err = %v, want ErrNoProof", err)
	}
	if _, err := r.verify(t, "schema/api.graphql"); err != nil {
		t.Fatalf("with --allow schema/api.graphql: %v", err)
	}
}

func TestVerify_NeedsAllowedSigners(t *testing.T) {
	r := newRepo(t)
	r.allowed = ""
	_, err := r.verify(t)
	if err == nil || !strings.Contains(err.Error(), "RELEASE_DAMNIT_ALLOWED_SIGNERS") {
		t.Fatalf("err = %v, want a message naming where the allowed signers come from", err)
	}
}

func TestGitSigner_ReadsGitsSSHSigningConfig(t *testing.T) {
	r := newRepo(t)
	// Empty values hide what the machine's own git config names.
	run(t, r.dir, "config", "user.signingkey", "")
	run(t, r.dir, "config", "gpg.format", "openpgp")
	if _, err := GitSigner(r.dir, t.TempDir()); !errors.Is(err, ErrNoSigningKey) {
		t.Fatalf("no key: err = %v, want ErrNoSigningKey", err)
	}
	run(t, r.dir, "config", "user.signingkey", r.signer.Key)
	if _, err := GitSigner(r.dir, t.TempDir()); err == nil || !strings.Contains(err.Error(), "gpg.format") {
		t.Fatalf("openpgp format: err = %v, want a gpg.format message", err)
	}
	run(t, r.dir, "config", "gpg.format", "ssh")
	got, err := GitSigner(r.dir, t.TempDir())
	if err != nil || got.Key != r.signer.Key || !strings.HasPrefix(got.Principal, "SHA256:") {
		t.Fatalf("GitSigner = %+v, %v; want the key %s under its fingerprint", got, err, r.signer.Key)
	}
	pub := strings.Fields(readFile(t, r.signer.Key+".pub"))
	run(t, r.dir, "config", "user.signingkey", "key::"+pub[0]+" "+pub[1])
	literal, err := GitSigner(r.dir, t.TempDir())
	if err != nil || literal.Principal != got.Principal {
		t.Fatalf("key:: literal: GitSigner = %+v, %v; want the same fingerprint %s", literal, err, got.Principal)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestVerify_PassesCommitsThatTouchOnlyMachinePaths(t *testing.T) {
	r := newRepo(t)
	write(t, r.dir, "release-please-config.json", `{"check": "make check", "machine-paths": ["deploy/", "perf/latest.json", "apps/*/tags.yaml"], "packages": {".": {"component": "app"}}}`)
	landed := commit(t, r.dir, "main.go", "package main // landed\n", "chore: name the machine paths")
	r.prove(t, landed, "", "make check")
	commit(t, r.dir, "deploy/app/tags.yaml", "tag: 1.1.0\n", "chore(deploy): update image tags [skip ci]")
	commit(t, r.dir, "perf/latest.json", "{}\n", "chore: record perf result [skip ci]")
	commit(t, r.dir, "apps/web/tags.yaml", "tag: 2\n", "chore(apps--web): deploy web 2 [skip ci]")
	v, err := r.verify(t)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if v.Commit != landed {
		t.Errorf("verified %s, want the landed commit %s under the machine commits", v.Commit, landed)
	}

	commit(t, r.dir, "apps/web/nested/tags.yaml", "tag: 3\n", "chore: deploy outside the pattern")
	if _, err := r.verify(t); !errors.Is(err, ErrNoProof) {
		t.Fatalf("a file the pattern does not match: err = %v, want ErrNoProof", err)
	}
	run(t, r.dir, "reset", "-q", "--hard", "HEAD^")

	commit(t, r.dir, "deploy/app/tags.yaml", "tag: 1.2.0\n", "chore(deploy): update image tags")
	write(t, r.dir, "deploy/app/tags.yaml", "tag: 1.3.0\n")
	commit(t, r.dir, "main.go", "package main // and code\n", "chore(deploy): update image tags")
	if _, err := r.verify(t); !errors.Is(err, ErrNoProof) {
		t.Fatalf("a machine-path commit that also changes code: err = %v, want ErrNoProof", err)
	}
}

func TestVerify_RefusesTheConfigAsAMachinePath(t *testing.T) {
	r := newRepo(t)
	write(t, r.dir, "release-please-config.json", `{"check": "make check", "machine-paths": ["release-please-*.json"], "packages": {".": {"component": "app"}}}`)
	commit(t, r.dir, "main.go", "package main // landed\n", "chore: widen the machine paths")
	r.prove(t, "HEAD", "", "make check")
	if _, err := r.verify(t); err == nil || !strings.Contains(err.Error(), "would let a commit") {
		t.Fatalf("err = %v, want the config refused as a machine path", err)
	}
}
