package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/land"
)

func runSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	remote := fs.String("remote", "origin", "Remote to sync from")
	mainBranch := fs.String("main", "", "The remote's main branch (default: the remote's HEAD, or main)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit sync [options] [dir...]

Fetch the remote main and fast-forward local main to it, in each repository
named (default: the current one). Where main is checked out, uncommitted
changes are carried forward: a file the incoming commits also change is
merged three ways, and when any such file would conflict, nothing moves.
Local main with commits the remote lacks is left for land.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	dirs := fs.Args()
	if len(dirs) == 0 {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, land.SyncPrefix+err.Error())
			return 1
		}
		dirs = []string{wd}
	}
	code := 0
	for _, dir := range dirs {
		err := land.Sync(land.SyncOptions{Dir: dir, Remote: *remote, Main: *mainBranch, Out: os.Stderr})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s%s: %v\n", land.SyncPrefix, dir, err)
			code = 1
		}
	}
	return code
}
