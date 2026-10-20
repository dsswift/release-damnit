// Package hooks installs the commit-msg hook that runs the commit check.
package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/git"
)

// CheckLine is the hook command: git passes the message file as $1.
const CheckLine = `release-damnit check-commit "$1"`

// marker identifies a hook this package wrote, so reinstalling can replace it.
const marker = "# Installed by release-damnit hooks."

// Script is the plain git commit-msg hook.
const Script = `#!/bin/sh
` + marker + `
if ! command -v release-damnit >/dev/null 2>&1; then
  echo "release-damnit is not installed: go install github.com/dsswift/release-damnit/cmd/release-damnit@latest" >&2
  exit 1
fi
exec ` + CheckLine + `
`

// ErrForeignHook is returned when a commit-msg hook this package did not
// write is already in place and force is off.
var ErrForeignHook = errors.New("a different commit-msg hook is already installed")

// Result says what Install did.
type Result struct {
	// Path is the hook file.
	Path string

	// Changed is false when the hook was already in place as written.
	Changed bool

	// Husky is true when the hook lives in .husky/.
	Husky bool
}

// Install puts the commit-msg hook in the repo containing dir. A repo with a
// .husky directory gets the check in .husky/commit-msg, replacing any
// commitlint line. Any other repo gets a git hook in its hooks directory.
func Install(dir string, force bool) (Result, error) {
	root, err := git.Toplevel(dir)
	if err != nil {
		return Result{}, err
	}
	if info, err := os.Stat(filepath.Join(root, ".husky")); err == nil && info.IsDir() {
		return installHusky(filepath.Join(root, ".husky", "commit-msg"))
	}

	hooksDir, err := git.Output(root, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return Result{}, err
	}
	return installGit(filepath.Join(hooksDir, "commit-msg"), force)
}

func installGit(path string, force bool) (Result, error) {
	result := Result{Path: path}
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return result, err
	case string(existing) == Script:
		return result, ensureExecutable(path)
	case !bytes.Contains(existing, []byte(marker)) && !force:
		return result, fmt.Errorf("%w at %s; remove it or rerun with --force", ErrForeignHook, path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return result, err
	}
	if err := os.WriteFile(path, []byte(Script), 0755); err != nil {
		return result, err
	}
	result.Changed = true
	return result, ensureExecutable(path)
}

// installHusky keeps every line of an existing .husky/commit-msg except
// commitlint, and adds the check once.
func installHusky(path string) (Result, error) {
	result := Result{Path: path, Husky: true}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}

	var lines []string
	hasCheck := false
	if len(existing) > 0 {
		for _, line := range strings.Split(strings.TrimRight(string(existing), "\n"), "\n") {
			if strings.Contains(line, "commitlint") {
				continue
			}
			if strings.Contains(line, "release-damnit check-commit") {
				hasCheck = true
			}
			lines = append(lines, line)
		}
	}
	if !hasCheck {
		lines = append(lines, CheckLine)
	}
	updated := strings.Join(lines, "\n") + "\n"

	if updated != string(existing) {
		if err := os.WriteFile(path, []byte(updated), 0755); err != nil {
			return result, err
		}
		result.Changed = true
	}
	return result, ensureExecutable(path)
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0111 != 0111 {
		return os.Chmod(path, info.Mode().Perm()|0111)
	}
	return nil
}
