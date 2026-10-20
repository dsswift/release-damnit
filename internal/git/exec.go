package git

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ErrNotRepository is returned when a directory is not inside a git work tree.
var ErrNotRepository = errors.New("not inside a git repository")

// Output runs git in dir and returns its stdout with surrounding whitespace
// trimmed. The error carries git's stderr.
func Output(dir string, args ...string) (string, error) {
	return runGit(dir, args...)
}

// OutputNUL runs git in dir and splits its NUL-separated stdout, as written by
// -z, into fields. Paths with unusual characters come back unquoted.
func OutputNUL(dir string, args ...string) ([]string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	var fields []string
	for _, f := range strings.Split(stdout.String(), "\x00") {
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields, nil
}

// Stream runs git in dir with its output going to out, so a hook's or a
// server's messages reach the user as git prints them.
func Stream(dir string, out io.Writer, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Toplevel returns the root of the work tree containing dir.
func Toplevel(dir string) (string, error) {
	top, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepository
	}
	return top, nil
}

// WebURL returns the browsable https URL of a remote: SSH forms for GitHub
// and GitLab hosts become https, and a trailing .git is dropped. Empty when
// the remote does not exist.
func WebURL(dir, remote string) string {
	url, err := runGit(dir, "remote", "get-url", remote)
	if err != nil {
		return ""
	}

	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
	}
	if strings.HasPrefix(url, "git@gitlab") {
		parts := strings.SplitN(strings.TrimPrefix(url, "git@"), ":", 2)
		if len(parts) == 2 {
			url = "https://" + parts[0] + "/" + parts[1]
		}
	}

	return strings.TrimSuffix(url, ".git")
}
