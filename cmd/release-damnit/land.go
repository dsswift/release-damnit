package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/land"
)

// exitMainMoved is land --proven's exit code when the remote main moved and
// the proven commit no longer lands as it is.
const exitMainMoved = 3

func runLand(args []string) int {
	fs := flag.NewFlagSet("land", flag.ContinueOnError)
	remote := fs.String("remote", "origin", "Remote to land on")
	mainBranch := fs.String("main", "main", "The remote's main branch")
	attempts := fs.Int("attempts", 5, "Tries before giving up when main keeps moving")
	proven := fs.Bool("proven", false, "Run no check: land main's commits only with the valid proof HEAD carries (release-damnit attest), and exit 3 if main moved")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit land [options] [branch]

On main with no branch: rebase main's local commits onto the remote main,
check them, and push. With a branch (or on a branch): merge it into the remote
main with --no-ff in a temporary worktree, check its commits, push, and move
local main to the result. Retries when main moves on the remote meanwhile.

Before each push it runs the repository's check (release-please-config.json
"check") on exactly the tree that lands, signs a landing proof with your SSH
signing key (git's user.signingkey), and pushes it as a git note on
refs/notes/release-damnit with the commit, atomically. A failed check pushes
nothing. release-damnit verify checks the proof in the pipeline.

With --proven, for a lander whose check must run away from its keys: land
runs no check. It lands main's own commits only when HEAD already carries a
proof for its tree, naming the repository's check, signed by this lander's
key (release-damnit attest makes one). When the remote main has moved, it
pushes nothing and exits 3; rebase, check the new tree, attest, and land
again.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, land.Prefix+err.Error())
		return 1
	}
	err = land.Run(land.Options{
		Dir:      wd,
		Branch:   fs.Arg(0),
		Remote:   *remote,
		Main:     *mainBranch,
		Attempts: *attempts,
		Out:      os.Stderr,
		Proven:   *proven,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, land.Prefix+err.Error())
		if errors.Is(err, land.ErrMainMoved) {
			return exitMainMoved
		}
		return 1
	}
	return 0
}
