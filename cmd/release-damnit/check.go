package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dsswift/release-damnit/internal/commitcheck"
)

const checkPrefix = commitcheck.Prefix

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
	commitcheck.WriteProblems(os.Stderr, result.Problems, "")
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
	commitcheck.WriteFailures(os.Stderr, failed)
	return 1
}
