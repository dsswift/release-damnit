package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/hooks"
)

const hooksPrefix = "release-damnit hooks: "

func runHooks(args []string) int {
	fs := flag.NewFlagSet("hooks", flag.ContinueOnError)
	force := fs.Bool("force", false, "Replace a commit-msg hook release-damnit did not write")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit hooks [--force]

Install the commit-msg hook that runs release-damnit check-commit. A repo with
a .husky directory gets it in .husky/commit-msg (any commitlint line is
removed); any other repo gets a git hook. Safe to run again.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, hooksPrefix+err.Error())
		return 1
	}
	if err := installHooks(wd, *force); err != nil {
		fmt.Fprintln(os.Stderr, hooksPrefix+err.Error())
		return 1
	}
	return 0
}

// installHooks installs the hook and says what happened. init runs it too.
func installHooks(dir string, force bool) error {
	result, err := hooks.Install(dir, force)
	if err != nil {
		return err
	}
	switch {
	case !result.Changed:
		fmt.Printf("%scommit-msg hook already in place at %s\n", hooksPrefix, result.Path)
	case result.Husky:
		fmt.Printf("%s%s now runs release-damnit check-commit\n", hooksPrefix, result.Path)
	default:
		fmt.Printf("%sinstalled the commit-msg hook at %s\n", hooksPrefix, result.Path)
	}
	return nil
}
