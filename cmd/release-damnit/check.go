package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dsswift/release-damnit/internal/commitcheck"
)

const checkPrefix = "release-damnit check-commit: "

func runCheckCommit(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, checkPrefix+"usage: release-damnit check-commit <msgfile>")
		return 1
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, checkPrefix+err.Error())
		return 1
	}
	result, err := commitcheck.CheckStaged(wd, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, checkPrefix+err.Error())
		return 1
	}
	if result.OK() {
		return 0
	}
	writeProblems(os.Stderr, result.Problems, "")
	return 1
}

func runCheckCommits(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, checkPrefix+"usage: release-damnit check-commits <rev-range>")
		return 1
	}
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, checkPrefix+err.Error())
		return 1
	}
	results, err := commitcheck.CheckRange(wd, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, checkPrefix+err.Error())
		return 1
	}
	failed := commitcheck.Failures(results)
	if len(failed) == 0 {
		fmt.Printf("%d commit(s) in %s pass\n", len(results), args[0])
		return 0
	}
	writeCommitFailures(os.Stderr, failed)
	return 1
}

// writeProblems prints one problem per line. A problem that spans lines (the
// per-package file list) keeps its continuation lines under the first.
func writeProblems(w io.Writer, problems []string, indent string) {
	for _, p := range problems {
		lines := strings.Split(p, "\n")
		fmt.Fprintln(w, checkPrefix+indent+lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintln(w, strings.Repeat(" ", len(checkPrefix))+indent+line)
		}
	}
}

func writeCommitFailures(w io.Writer, failed []commitcheck.CommitResult) {
	for _, c := range failed {
		fmt.Fprintf(w, "%s%s %s\n", checkPrefix, c.Short(), c.Header)
		writeProblems(w, c.Problems, "  ")
	}
	fmt.Fprintf(w, "%s%d commit(s) break the commit rules; reword them (git rebase -i) and try again\n", checkPrefix, len(failed))
}
