package commit

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dsswift/release-damnit/internal/commitcheck"
	"github.com/dsswift/release-damnit/internal/config"
)

// Reply is the commit plan Claude returns: the commits in the order to create them.
type Reply struct {
	Commits []PlannedCommit `json:"commits"`
}

// PlannedCommit is one commit of a plan. Files name changes by their path
// (the new path of a rename).
type PlannedCommit struct {
	Message string   `json:"message"`
	Files   []string `json:"files"`
}

// replySchema is the JSON schema Claude's structured output must satisfy.
const replySchema = `{"type":"object","additionalProperties":false,"required":["commits"],"properties":{"commits":{"type":"array","minItems":1,"items":{"type":"object","additionalProperties":false,"required":["message","files"],"properties":{"message":{"type":"string"},"files":{"type":"array","minItems":1,"items":{"type":"string"}}}}}}}`

// PackageChanges are the changes inside one package.
type PackageChanges struct {
	Package *config.Package
	Changes []Change
}

// Partition is the pending changes split by the package that owns them.
type Partition struct {
	// Packages hold the changes inside a package, sorted by scope.
	Packages []PackageChanges

	// Loose are the changes outside every package. In a repo with no
	// release config, every change is loose.
	Loose []Change
}

// partition splits changes by package, as the commit checker assigns files.
// A nil cfg means the repo has no release config. A rename across two
// packages cannot be one valid commit and is an error.
func partition(cfg *config.Config, changes []Change) (Partition, error) {
	var p Partition
	if cfg == nil {
		p.Loose = changes
		return p, nil
	}
	index := map[*config.Package]int{}
	for _, c := range changes {
		pkg := cfg.FindPackageForPath(c.Path)
		if c.OldPath != "" {
			old := cfg.FindPackageForPath(c.OldPath)
			switch {
			case pkg == nil:
				pkg = old
			case old != nil && old != pkg:
				return Partition{}, fmt.Errorf("the rename %s -> %s moves a file from package %s to %s; commit it by hand as two commits", c.OldPath, c.Path, old.Scope(), pkg.Scope())
			}
		}
		if pkg == nil {
			p.Loose = append(p.Loose, c)
			continue
		}
		i, ok := index[pkg]
		if !ok {
			i = len(p.Packages)
			index[pkg] = i
			p.Packages = append(p.Packages, PackageChanges{Package: pkg})
		}
		p.Packages[i].Changes = append(p.Packages[i].Changes, c)
	}
	sort.Slice(p.Packages, func(i, j int) bool { return p.Packages[i].Package.Scope() < p.Packages[j].Package.Scope() })
	return p, nil
}

// input is everything a plan is built from and checked against.
type input struct {
	rules commitcheck.Rules
	parts Partition

	// diffs maps a change's path to its diff, already truncated.
	diffs map[string]string
}

func (in input) changes() []Change {
	all := append([]Change(nil), in.parts.Loose...)
	for _, pc := range in.parts.Packages {
		all = append(all, pc.Changes...)
	}
	return all
}

// check returns the problems that make the plan unusable. Empty means every
// change is in exactly one commit and every commit passes the commit rules.
func (in input) check(reply Reply) []string {
	byPath := map[string]Change{}
	for _, c := range in.changes() {
		byPath[c.Path] = c
	}

	var problems []string
	if len(reply.Commits) == 0 {
		problems = append(problems, "the plan has no commits")
	}
	seen := map[string]int{}
	for i, pc := range reply.Commits {
		n := i + 1
		msg := strings.TrimSpace(pc.Message)
		if strings.Contains(msg, "\n") {
			problems = append(problems, fmt.Sprintf("commit %d: the message must be one header line, no body", n))
		}
		if len(pc.Files) == 0 {
			problems = append(problems, fmt.Sprintf("commit %d (%q) lists no files", n, msg))
		}

		var paths []string
		packages := map[string]bool{}
		for _, f := range pc.Files {
			c, ok := byPath[f]
			if !ok {
				problems = append(problems, fmt.Sprintf("commit %d lists %q, which is not a pending file", n, f))
				continue
			}
			if prev, dup := seen[f]; dup {
				problems = append(problems, fmt.Sprintf("%q is in commit %d and commit %d; put each file in exactly one commit", f, prev, n))
				continue
			}
			seen[f] = n
			paths = append(paths, c.Paths()...)
			if in.rules.Config != nil {
				if pkg := in.rules.Config.FindPackageForPath(c.Path); pkg != nil {
					packages[pkg.Scope()] = true
				}
			}
		}
		if len(packages) > 1 {
			problems = append(problems, fmt.Sprintf("commit %d (%q) mixes packages %s; one commit may touch one package", n, msg, strings.Join(sortedKeys(packages), ", ")))
			continue
		}

		if in.rules.Config == nil {
			if h, ok := commitcheck.ParseHeader(msg); ok && h.HasScope {
				problems = append(problems, fmt.Sprintf("commit %d (%q): this repo has no packages; drop the scope", n, msg))
			}
		}
		for _, p := range commitcheck.Check(in.rules, msg, paths).Problems {
			problems = append(problems, fmt.Sprintf("commit %d (%q): %s", n, msg, p))
		}
	}
	for _, c := range in.changes() {
		if _, ok := seen[c.Path]; !ok {
			problems = append(problems, fmt.Sprintf("%q is in no commit; every file must be in exactly one commit", c.Path))
		}
	}
	return problems
}

// prompt asks for a commit plan: the rules, the files by package, and the diffs.
func (in input) prompt() string {
	var b strings.Builder
	allowed := append([]string(nil), in.rules.Types...)
	if contains(allowed, "feat") {
		allowed = append(allowed, "feat!")
	}

	b.WriteString(`Group these pending git changes into commits and write each commit message.

Rules:
- Each commit message is a single header line: <type>(<scope>): <description>, or <type>: <description> where a scope is not required.
`)
	fmt.Fprintf(&b, "- The type is one of: %s. Use feat! only for a breaking change.\n", strings.Join(allowed, ", "))
	fmt.Fprintf(&b, "- The description is lowercase, imperative (\"add\", not \"added\"), has no trailing period, and is at most %d characters.\n", in.rules.SubjectMaxLength)
	b.WriteString(`- Every file below goes in exactly one commit. List files by the path shown (the new path of a rename).
- A commit touches at most one package. Group related changes together; split unrelated changes into separate commits.
- Prefer few commits. Order them so earlier commits do not depend on later ones.
`)

	single := in.rules.Config != nil && len(in.rules.Config.Packages) == 1
	switch {
	case in.rules.Config == nil:
		b.WriteString("- This repo has no packages: write every message with no scope, as <type>: <description>.\n")
	case single:
		b.WriteString("- This repo has one package: write every message with no scope, as <type>: <description>.\n")
	}

	for _, pc := range in.parts.Packages {
		if single {
			b.WriteString("\nFiles:\n")
		} else {
			fmt.Fprintf(&b, "\nPackage %s: its commits use the scope %q.\n", pc.Package.Scope(), pc.Package.Scope())
		}
		writeFiles(&b, pc.Changes)
	}
	if len(in.parts.Loose) > 0 {
		switch {
		case in.rules.Config == nil:
			b.WriteString("\nFiles:\n")
		case single:
			b.WriteString("\nFiles outside the package: they may join any commit, or form their own.\n")
		case len(in.rules.Scopes) == 0:
			b.WriteString("\nFiles outside every package: each must join a commit of one of the packages above.\n")
		default:
			fmt.Fprintf(&b, "\nFiles outside every package: each may join a commit of one package above, or form commits of their own with one of these scopes: %s.\n", strings.Join(in.rules.Scopes, ", "))
		}
		writeFiles(&b, in.parts.Loose)
	}

	b.WriteString("\nDiffs:\n")
	for _, c := range in.changes() {
		fmt.Fprintf(&b, "\n=== %s\n%s\n", c.Path, in.diffs[c.Path])
	}
	return b.String()
}

// retryPrompt asks again, showing the rejected reply and why it was rejected.
func (in input) retryPrompt(reply Reply, problems []string) string {
	prev, _ := json.Marshal(reply)
	var b strings.Builder
	b.WriteString(in.prompt())
	fmt.Fprintf(&b, "\nYour previous plan was rejected:\n%s\n\nProblems:\n", prev)
	for _, p := range problems {
		fmt.Fprintf(&b, "- %s\n", p)
	}
	b.WriteString("\nReturn a corrected plan that fixes every problem.\n")
	return b.String()
}

func writeFiles(b *strings.Builder, changes []Change) {
	for _, c := range changes {
		switch {
		case c.OldPath != "":
			fmt.Fprintf(b, "- %s (renamed from %s)\n", c.Path, c.OldPath)
		case c.Untracked:
			fmt.Fprintf(b, "- %s (new file)\n", c.Path)
		default:
			fmt.Fprintf(b, "- %s\n", c.Path)
		}
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
