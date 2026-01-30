// Package git provides functions for analyzing git history.
// It shells out to git commands rather than using a library for simplicity
// and to match the exact behavior of git itself.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spraguehouse/release-damnit/pkg/contracts"
)

// Commit represents a parsed git commit.
type Commit struct {
	// SHA is the full commit hash.
	SHA string

	// ShortSHA is the first 7 characters of the hash.
	ShortSHA string

	// Type is the conventional commit type (feat, fix, chore, etc.).
	Type string

	// Scope is the conventional commit scope (optional).
	Scope string

	// Description is the commit message subject (without type/scope prefix).
	Description string

	// IsBreaking indicates if this is a breaking change (! suffix or BREAKING CHANGE footer).
	IsBreaking bool

	// Files is the list of files changed by this commit.
	Files []string
}

// MergeInfo contains information about a merge commit.
type MergeInfo struct {
	// IsMerge is true if HEAD is a merge commit.
	IsMerge bool

	// MergeBase is the common ancestor of the merge (parent of first parent).
	MergeBase string

	// MergeHead is the tip of the merged branch (second parent).
	MergeHead string

	// HeadSHA is the SHA of HEAD.
	HeadSHA string
}

// conventionalCommitRegex parses conventional commit messages.
// Format: type(scope)!: description  OR  type!: description  OR  type: description
var conventionalCommitRegex = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?\s*:\s*(.+)$`)

// AnalyzeHead determines if HEAD is a merge commit and returns merge information.
func AnalyzeHead(repoPath string) (*MergeInfo, error) {
	contracts.RequireNotEmpty(repoPath, "repoPath")

	info := &MergeInfo{}

	// Get HEAD SHA
	headSHA, err := runGit(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}
	info.HeadSHA = headSHA

	// Try to get second parent (HEAD^2) - if this fails, it's not a merge commit
	mergeHead, err := runGit(repoPath, "rev-parse", "HEAD^2")
	if err != nil {
		// Not a merge commit - fall back to HEAD~1..HEAD
		info.IsMerge = false
		return info, nil
	}

	info.IsMerge = true
	info.MergeHead = mergeHead

	// Get merge base (common ancestor)
	firstParent, err := runGit(repoPath, "rev-parse", "HEAD^1")
	if err != nil {
		return nil, fmt.Errorf("failed to get first parent: %w", err)
	}

	mergeBase, err := runGit(repoPath, "merge-base", firstParent, mergeHead)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge base: %w", err)
	}
	info.MergeBase = mergeBase

	return info, nil
}

// GetCommitsInRange returns all commits in the range base..head (exclusive of base).
// If head is empty, it defaults to HEAD.
func GetCommitsInRange(repoPath, base, head string) ([]*Commit, error) {
	contracts.RequireNotEmpty(repoPath, "repoPath")
	contracts.RequireNotEmpty(base, "base")

	if head == "" {
		head = "HEAD"
	}

	// Get commit list with format: SHA|subject
	rangeSpec := fmt.Sprintf("%s..%s", base, head)
	output, err := runGit(repoPath, "log", "--format=%H|%s", "--reverse", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits in range %s: %w", rangeSpec, err)
	}

	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	var commits []*Commit

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}

		sha := parts[0]
		subject := parts[1]

		commit := parseCommit(sha, subject)

		// Get changed files for this commit
		files, err := getChangedFiles(repoPath, sha)
		if err != nil {
			return nil, fmt.Errorf("failed to get changed files for %s: %w", sha[:7], err)
		}
		commit.Files = files

		commits = append(commits, commit)
	}

	return commits, nil
}

// GetCommitsSinceLastTag returns commits since the last tag matching the pattern.
// If no tag is found, returns all commits.
func GetCommitsSinceLastTag(repoPath, tagPattern string) ([]*Commit, error) {
	contracts.RequireNotEmpty(repoPath, "repoPath")

	// Get the most recent tag matching pattern
	tag, err := runGit(repoPath, "describe", "--tags", "--abbrev=0", "--match", tagPattern)
	if err != nil {
		// No matching tag - get all commits from root
		// Use empty tree as base
		return GetCommitsInRange(repoPath, "$(git rev-list --max-parents=0 HEAD)", "HEAD")
	}

	return GetCommitsInRange(repoPath, tag, "HEAD")
}

// parseCommit parses a commit SHA and subject into a Commit struct.
func parseCommit(sha, subject string) *Commit {
	commit := &Commit{
		SHA:      sha,
		ShortSHA: sha[:7],
	}

	// Try to parse as conventional commit
	matches := conventionalCommitRegex.FindStringSubmatch(subject)
	if matches != nil {
		commit.Type = strings.ToLower(matches[1])
		commit.Scope = matches[2]
		commit.IsBreaking = matches[3] == "!"
		commit.Description = matches[4]
	} else {
		// Not a conventional commit - treat as unknown type
		commit.Type = ""
		commit.Description = subject
	}

	return commit
}

// getChangedFiles returns the list of files changed by a commit.
func getChangedFiles(repoPath, sha string) ([]string, error) {
	output, err := runGit(repoPath, "diff-tree", "--no-commit-id", "--name-only", "-r", sha)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	files := strings.Split(output, "\n")
	var result []string
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}
	return result, nil
}

// runGit executes a git command and returns stdout as a string.
func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// IsValidSHA checks if a string looks like a valid git SHA.
func IsValidSHA(sha string) bool {
	if len(sha) < 7 || len(sha) > 40 {
		return false
	}
	for _, c := range sha {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
