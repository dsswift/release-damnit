// Package commit turns pending changes into conventional commits: it groups
// the files by package, asks Claude for the commit split and messages, checks
// the reply against the commit rules, and creates the commits with git.
package commit

import (
	"bytes"
	"fmt"
	"os/exec"
	"path"
	"strings"
)

// Change is one pending file change. A rename carries both paths and is
// committed as one unit.
type Change struct {
	// Path is the file's path from the repo root, the new path of a rename.
	Path string

	// OldPath is the source of a rename; empty otherwise.
	OldPath string

	// Untracked marks a file git does not know yet.
	Untracked bool
}

// Paths returns every path the change touches: both sides of a rename.
func (c Change) Paths() []string {
	if c.OldPath != "" {
		return []string{c.Path, c.OldPath}
	}
	return []string{c.Path}
}

// secretNames are files that hold secrets in practice, whatever their name
// suggests. They are never read, sent, or committed.
var secretNames = map[string]bool{
	"appsettings.json":    true,
	"local.settings.json": true,
	"docker-compose.yml":  true,
	".env":                true,
	"credentials.json":    true,
}

// IsSecret reports whether a path names a secret-bearing file.
func IsSecret(p string) bool {
	base := path.Base(p)
	return secretNames[base] || strings.HasPrefix(base, ".env.")
}

// splitSecrets separates the changes that touch a secret-bearing file.
func splitSecrets(changes []Change) (keep, secret []Change) {
	for _, c := range changes {
		if IsSecret(c.Path) || (c.OldPath != "" && IsSecret(c.OldPath)) {
			secret = append(secret, c)
		} else {
			keep = append(keep, c)
		}
	}
	return keep, secret
}

// parseStatus parses `git status --porcelain=v1 -z --untracked-files=all`.
// With stagedOnly, it keeps only the entries with a staged change.
func parseStatus(out string, stagedOnly bool) ([]Change, error) {
	fields := strings.Split(out, "\x00")
	var changes []Change
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if entry == "" {
			continue
		}
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, fmt.Errorf("unexpected git status entry %q", entry)
		}
		x, y, p := entry[0], entry[1], entry[3:]

		c := Change{Path: p}
		if x == 'R' || x == 'C' {
			i++
			if i >= len(fields) {
				return nil, fmt.Errorf("git status entry %q has no source path", entry)
			}
			if x == 'R' {
				c.OldPath = fields[i]
			}
		}

		switch {
		case x == '!' && y == '!':
			continue
		case x == '?' && y == '?':
			if stagedOnly {
				continue
			}
			c.Untracked = true
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			return nil, fmt.Errorf("%s has merge conflicts; resolve them first", p)
		case stagedOnly && x == ' ':
			continue
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// pending lists the changes to commit: everything modified, deleted, renamed
// or untracked (not ignored), or with stagedOnly just what is staged.
func pending(root string, stagedOnly bool) ([]Change, error) {
	out, err := gitRun(root, nil, "", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parseStatus(out, stagedOnly)
}

// gitRun runs git in dir with extra environment and stdin, returning stdout.
// The error carries git's stderr.
func gitRun(dir string, env []string, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
