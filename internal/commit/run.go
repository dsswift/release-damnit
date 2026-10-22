package commit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/commitcheck"
	"github.com/dsswift/release-damnit/internal/config"
)

// Prefix starts every status line commit prints.
const Prefix = "release-damnit commit: "

// Diff size limits: a large diff is cut so the prompt stays small and fast.
const (
	maxFileDiff  = 6000
	maxTotalDiff = 80000
)

// Options configures a commit run.
type Options struct {
	// Dir is any directory inside the repository.
	Dir string

	// Staged plans only what is already staged.
	Staged bool

	// Yes creates the commits without asking.
	Yes bool

	// DryRun shows the plan and stops.
	DryRun bool

	// Planner writes the plan.
	Planner Planner

	// In answers the confirmation prompt. Out receives the plan, progress,
	// and git's own output (hooks).
	In  io.Reader
	Out io.Writer
}

// Run plans and creates commits for the pending changes.
func Run(ctx context.Context, opts Options) error {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.In == nil {
		opts.In = strings.NewReader("")
	}
	out := opts.Out

	rules, root, err := commitcheck.LoadRules(opts.Dir)
	if err != nil {
		return err
	}
	changes, err := pending(root, opts.Staged)
	if err != nil {
		return err
	}
	changes, secret := splitSecrets(changes)
	if len(secret) > 0 {
		fmt.Fprintln(out, Prefix+"skipped, may hold secrets (commit their non-secret parts by hand):")
		for _, c := range secret {
			fmt.Fprintln(out, "  "+describe(c))
		}
	}
	if len(changes) == 0 {
		fmt.Fprintln(out, Prefix+"nothing to commit")
		return nil
	}

	parts, err := partition(rules.Config, changes)
	if err != nil {
		return err
	}
	if rules.Config != nil && len(rules.Config.Packages) > 1 && len(rules.Scopes) == 0 && len(parts.Packages) == 0 {
		return fmt.Errorf("every change is outside the packages, and %s lists no repo scopes for such a commit; add one to \"scopes\"", config.ConfigFile)
	}

	base, hasHead, err := diffBase(root)
	if err != nil {
		return err
	}
	in := input{rules: rules, parts: parts, diffs: diffs(root, base, opts.Staged, changes)}

	fmt.Fprintf(out, "%sasking Claude to plan %d file(s)\n", Prefix, len(changes))
	reply, err := plan(ctx, opts.Planner, in)
	if err != nil {
		return err
	}

	writePlan(out, in, reply)
	if opts.DryRun {
		return nil
	}
	if !opts.Yes && !confirm(opts.In, out, len(reply.Commits)) {
		fmt.Fprintln(out, Prefix+"nothing committed")
		return nil
	}

	byPath := map[string]Change{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	var done []string
	for _, pc := range reply.Commits {
		var paths []string
		for _, f := range pc.Files {
			paths = append(paths, byPath[f].Paths()...)
		}
		if err := createCommit(root, opts.Staged, hasHead, strings.TrimSpace(pc.Message), paths, out); err != nil {
			if len(done) == 0 {
				fmt.Fprintln(out, Prefix+"no commits were created")
			} else {
				fmt.Fprintf(out, "%scommitted %d of %d before the failure:\n", Prefix, len(done), len(reply.Commits))
				for _, d := range done {
					fmt.Fprintln(out, "  "+d)
				}
			}
			return err
		}
		hasHead = true
		line, err := gitRun(root, nil, "", "log", "-1", "--format=%h %s")
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		done = append(done, line)
		fmt.Fprintln(out, Prefix+"committed "+line)
	}
	return nil
}

// plan asks for a plan and checks it, retrying once with the problems found.
func plan(ctx context.Context, planner Planner, in input) (Reply, error) {
	reply, err := planner.Plan(ctx, in.prompt())
	if err != nil {
		return Reply{}, err
	}
	problems := in.check(reply)
	if len(problems) == 0 {
		return reply, nil
	}

	reply, err = planner.Plan(ctx, in.retryPrompt(reply, problems))
	if err != nil {
		return Reply{}, err
	}
	problems = in.check(reply)
	if len(problems) == 0 {
		return reply, nil
	}
	return Reply{}, fmt.Errorf("Claude's plan broke the commit rules twice; nothing was committed:\n  %s", strings.Join(problems, "\n  "))
}

func writePlan(out io.Writer, in input, reply Reply) {
	byPath := map[string]Change{}
	for _, c := range in.changes() {
		byPath[c.Path] = c
	}
	fmt.Fprintf(out, "%splan, %d commit(s):\n", Prefix, len(reply.Commits))
	for i, pc := range reply.Commits {
		fmt.Fprintf(out, "\n  %d. %s\n", i+1, strings.TrimSpace(pc.Message))
		for _, f := range pc.Files {
			fmt.Fprintln(out, "       "+describe(byPath[f]))
		}
	}
	fmt.Fprintln(out)
}

func describe(c Change) string {
	if c.OldPath != "" {
		return c.OldPath + " -> " + c.Path
	}
	return c.Path
}

func confirm(in io.Reader, out io.Writer, n int) bool {
	fmt.Fprintf(out, "Create %d commit(s)? [y/N] ", n)
	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// diffBase returns what the changes are diffed against: HEAD, or the empty
// tree in a repo with no commits yet.
func diffBase(root string) (base string, hasHead bool, err error) {
	if head, err := gitRun(root, nil, "", "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		return strings.TrimSpace(head), true, nil
	}
	empty, err := gitRun(root, nil, "", "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(empty), false, nil
}

// diffs returns each change's diff, cut to the size limits. An untracked
// file is diffed against nothing.
func diffs(root, base string, staged bool, changes []Change) map[string]string {
	result := map[string]string{}
	total := 0
	for _, c := range changes {
		if total >= maxTotalDiff {
			result[c.Path] = "(diff omitted: the total diff is too large)"
			continue
		}
		var d string
		var err error
		switch {
		case c.Untracked:
			d, err = gitRun(root, nil, "", "diff", "--no-color", "--no-ext-diff", "--no-index", "--", os.DevNull, c.Path)
			// --no-index exits 1 when the files differ, which they always do here.
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				err = nil
			}
		case staged:
			d, err = gitRun(root, nil, "", append([]string{"--literal-pathspecs", "diff", "--no-color", "--no-ext-diff", "-M", "--cached", base, "--"}, c.Paths()...)...)
		default:
			d, err = gitRun(root, nil, "", append([]string{"--literal-pathspecs", "diff", "--no-color", "--no-ext-diff", "-M", base, "--"}, c.Paths()...)...)
		}
		if err != nil {
			d = "(diff unavailable: " + err.Error() + ")"
		}
		if len(d) > maxFileDiff {
			d = d[:maxFileDiff] + fmt.Sprintf("\n(diff cut: %d more bytes)", len(d)-maxFileDiff)
		}
		total += len(d)
		result[c.Path] = d
	}
	return result
}

// createCommit commits exactly paths with message, with hooks running. In
// the default mode the paths are staged first from the work tree. The commit
// is built in a temporary index holding HEAD plus the paths' staged entries,
// so nothing else staged rides along and the real index is left as is.
func createCommit(root string, staged, hasHead bool, message string, paths []string, out io.Writer) error {
	if !staged {
		if _, err := gitRun(root, nil, "", append([]string{"--literal-pathspecs", "add", "-A", "--"}, paths...)...); err != nil {
			return err
		}
	}

	tmp, err := os.MkdirTemp("", "release-damnit-commit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}

	if hasHead {
		_, err = gitRun(root, env, "", "read-tree", "HEAD")
	} else {
		_, err = gitRun(root, env, "", "read-tree", "--empty")
	}
	if err != nil {
		return err
	}

	listed, err := gitRun(root, nil, "", append([]string{"--literal-pathspecs", "ls-files", "-s", "-z", "--"}, paths...)...)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	var entries strings.Builder
	present := map[string]bool{}
	for _, rec := range strings.Split(listed, "\x00") {
		_, p, ok := strings.Cut(rec, "\t")
		if !ok || !want[p] {
			continue
		}
		present[p] = true
		entries.WriteString(rec + "\x00")
	}
	if entries.Len() > 0 {
		if _, err := gitRun(root, env, entries.String(), "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	var removed []string
	for _, p := range paths {
		if !present[p] {
			removed = append(removed, p)
		}
	}
	if len(removed) > 0 {
		if _, err := gitRun(root, env, "", append([]string{"update-index", "--force-remove", "--"}, removed...)...); err != nil {
			return err
		}
	}

	cmd := exec.Command("git", "commit", "-q", "-m", message)
	cmd.Dir = root
	cmd.Env = append(cmd.Environ(), env...)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit %q failed: %w", message, err)
	}
	return nil
}
