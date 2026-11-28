package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/proof"
)

const attestPrefix = "release-damnit attest: "

func runAttest(args []string) int {
	fs := flag.NewFlagSet("attest", flag.ContinueOnError)
	commit := fs.String("commit", "HEAD", "The commit whose tree the check passed on")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit attest [options]

Sign a landing proof for a commit's tree, saying the repository's check
(release-please-config.json "check") passed on it, with your SSH signing key
(git's user.signingkey), and write it as the commit's note on
refs/notes/release-damnit. It runs nothing: you ran the check elsewhere, on
exactly this tree, and your key vouches for it. For a lander whose check must
run away from its keys; land it with release-damnit land --proven.

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
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, attestPrefix+err.Error())
		return 1
	}
	p, err := proof.Attest(wd, *commit, os.TempDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, attestPrefix+err.Error())
		return 1
	}
	st := p.Statement
	fmt.Printf("%s%q passed on tree %s; signed as %s\n", attestPrefix, st.Check, short(st.Tree), st.Signer)
	return 0
}
