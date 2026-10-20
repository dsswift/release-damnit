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
