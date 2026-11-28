package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dsswift/release-damnit/internal/proof"
)

// allowedSignersEnv holds the allowed signers themselves, for a pipeline that
// keeps them in a variable rather than a file.
const allowedSignersEnv = "RELEASE_DAMNIT_ALLOWED_SIGNERS"

const verifyPrefix = "release-damnit verify: "

// stringList is a flag that may be given more than once.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, " ") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	commit := fs.String("commit", "HEAD", "The commit being released")
	remote := fs.String("remote", "origin", "Remote to fetch the landing proofs from; empty fetches nothing")
	signers := fs.String("allowed-signers", "", "Allowed signers file (ssh-keygen format); default: the "+allowedSignersEnv+" variable's contents")
	var allow stringList
	fs.Var(&allow, "allow", "A further path a version commit may change (what before-publish adds); repeatable")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit verify [options]

Check that the commit being released carries a landing proof: a note that
release-damnit land signed after the repository's check passed, signed by a
key the allowed signers list, for exactly that commit's tree, naming the
check the repository names. A version commit CI made needs none; the commit
under it is checked. Fails with a plain message when any part is missing or
wrong. Run it as a release pipeline's first step.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, verifyPrefix+err.Error())
		return 1
	}
	wd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	path := *signers
	if path == "" {
		if listed := os.Getenv(allowedSignersEnv); strings.TrimSpace(listed) != "" {
			f, err := os.CreateTemp("", "release-damnit-allowed-signers-*")
			if err != nil {
				return fail(err)
			}
			defer os.Remove(f.Name())
			_, err = f.WriteString(strings.TrimSpace(listed) + "\n")
			f.Close()
			if err != nil {
				return fail(err)
			}
			path = f.Name()
		}
	}
	v, err := proof.Verify(proof.VerifyOptions{
		Dir:            wd,
		Commit:         *commit,
		Remote:         *remote,
		AllowedSigners: path,
		Allow:          allow,
	})
	if err != nil {
		return fail(err)
	}
	st := v.Proof.Statement
	if v.Commit != v.Released {
		fmt.Printf("%s%s is a version commit on %s\n", verifyPrefix, short(v.Released), short(v.Commit))
	}
	fmt.Printf("%s%s landed by %s at %s: %q passed on tree %s\n", verifyPrefix, short(v.Commit), st.Signer, st.Time.UTC().Format("2006-01-02 15:04 UTC"), st.Check, short(st.Tree))
	return 0
}

func short(sha string) string { return sha[:min(len(sha), 9)] }
