package commitcheck

import (
	"fmt"
	"io"
	"strings"
)

// Prefix starts every line the commit check prints.
const Prefix = "release-damnit check-commit: "

// WriteProblems prints one problem per line. A problem that spans lines (the
// per-package file list) keeps its continuation lines under the first.
func WriteProblems(w io.Writer, problems []string, indent string) {
	for _, p := range problems {
		lines := strings.Split(p, "\n")
		fmt.Fprintln(w, Prefix+indent+lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintln(w, strings.Repeat(" ", len(Prefix))+indent+line)
		}
	}
}

// WriteFailures prints each failing commit (short SHA and header) with its
// problems, then a closing line saying what to do.
func WriteFailures(w io.Writer, failed []CommitResult) {
	for _, c := range failed {
		fmt.Fprintf(w, "%s%s %s\n", Prefix, c.Short(), c.Header)
		WriteProblems(w, c.Problems, "  ")
	}
	fmt.Fprintf(w, "%s%d commit(s) break the commit rules; reword them (git rebase -i) and try again\n", Prefix, len(failed))
}
