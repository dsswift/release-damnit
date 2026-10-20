package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/land"
)

func runLand(args []string) int {
	fs := flag.NewFlagSet("land", flag.ContinueOnError)
	remote := fs.String("remote", "origin", "Remote to land on")
	mainBranch := fs.String("main", "main", "The remote's main branch")
	attempts := fs.Int("attempts", 5, "Tries before giving up when main keeps moving")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit land [options] [branch]

On main with no branch: rebase main's local commits onto the remote main,
check them, and push. With a branch (or on a branch): merge it into the remote
main with --no-ff in a temporary worktree, check its commits, push, and move
local main to the result. Retries when main moves on the remote meanwhile.

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
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, land.Prefix+err.Error())
		return 1
	}
	return 0
}
