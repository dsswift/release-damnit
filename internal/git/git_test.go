package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createTestGitRepo creates a temporary git repository for testing.
func createTestGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize repo
	runCmd(t, dir, "git", "init", "--initial-branch=main")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")

	return dir
}

// runCmd runs a command in the given directory.
func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", name, args, err, out)
	}
}

// writeFile creates a file with content in the repo.
func writeFile(t *testing.T, dir, path, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, path)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func TestParseCommit_ConventionalCommit(t *testing.T) {
	tests := []struct {
		subject     string
		wantType    string
		wantScope   string
		wantBreak   bool
		wantDesc    string
	}{
		{
			subject:   "feat: add new feature",
			wantType:  "feat",
			wantScope: "",
			wantBreak: false,
			wantDesc:  "add new feature",
		},
		{
			subject:   "fix(auth): handle null input",
			wantType:  "fix",
			wantScope: "auth",
			wantBreak: false,
			wantDesc:  "handle null input",
		},
		{
			subject:   "feat!: breaking change",
			wantType:  "feat",
			wantScope: "",
			wantBreak: true,
			wantDesc:  "breaking change",
		},
		{
			subject:   "feat(api)!: breaking API change",
			wantType:  "feat",
			wantScope: "api",
			wantBreak: true,
			wantDesc:  "breaking API change",
		},
		{
			subject:   "chore: update dependencies",
			wantType:  "chore",
			wantScope: "",
			wantBreak: false,
			wantDesc:  "update dependencies",
		},
		{
			subject:   "FEAT: uppercase type",
			wantType:  "feat",
			wantScope: "",
			wantBreak: false,
			wantDesc:  "uppercase type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			commit := parseCommit("abc1234567890", tc.subject)

			if commit.Type != tc.wantType {
				t.Errorf("type: got %s, want %s", commit.Type, tc.wantType)
			}
			if commit.Scope != tc.wantScope {
				t.Errorf("scope: got %s, want %s", commit.Scope, tc.wantScope)
			}
			if commit.IsBreaking != tc.wantBreak {
				t.Errorf("breaking: got %v, want %v", commit.IsBreaking, tc.wantBreak)
			}
			if commit.Description != tc.wantDesc {
				t.Errorf("description: got %s, want %s", commit.Description, tc.wantDesc)
			}
		})
	}
}

func TestParseCommit_NonConventionalCommit(t *testing.T) {
	tests := []string{
		"just a regular commit message",
		"Update readme",
		"Merge branch 'feature' into main",
	}

	for _, subject := range tests {
		t.Run(subject, func(t *testing.T) {
			commit := parseCommit("abc1234567890", subject)

			if commit.Type != "" {
				t.Errorf("expected empty type for non-conventional commit, got %s", commit.Type)
			}
			if commit.Description != subject {
				t.Errorf("expected description to be full subject, got %s", commit.Description)
			}
		})
	}
}

func TestAnalyzeHead_NonMergeCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestGitRepo(t)

	// Create a simple commit
	writeFile(t, dir, "file.txt", "content")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "feat: initial commit")

	info, err := AnalyzeHead(dir)
	if err != nil {
		t.Fatalf("AnalyzeHead failed: %v", err)
	}

	if info.IsMerge {
		t.Error("expected non-merge commit")
	}
	if info.HeadSHA == "" {
		t.Error("expected HEAD SHA to be set")
	}
}

func TestAnalyzeHead_MergeCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestGitRepo(t)

	// Create initial commit on main
	writeFile(t, dir, "file.txt", "initial")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "feat: initial commit")

	// Create feature branch
	runCmd(t, dir, "git", "checkout", "-b", "feature")
	writeFile(t, dir, "feature.txt", "feature content")
	runCmd(t, dir, "git", "add", "feature.txt")
	runCmd(t, dir, "git", "commit", "-m", "feat: add feature")

	// Go back to main and merge with --no-ff
	runCmd(t, dir, "git", "checkout", "main")
	runCmd(t, dir, "git", "merge", "--no-ff", "feature", "-m", "Merge branch 'feature'")

	info, err := AnalyzeHead(dir)
	if err != nil {
		t.Fatalf("AnalyzeHead failed: %v", err)
	}

	if !info.IsMerge {
		t.Error("expected merge commit")
	}
	if info.MergeBase == "" {
		t.Error("expected MergeBase to be set")
	}
	if info.MergeHead == "" {
		t.Error("expected MergeHead to be set")
	}
}

func TestGetCommitsInRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestGitRepo(t)

	// Create initial commit
	writeFile(t, dir, "file.txt", "initial")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")

	// Get the initial commit SHA
	initialSHA, _ := runGit(dir, "rev-parse", "HEAD")

	// Create more commits
	writeFile(t, dir, "file.txt", "update 1")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "feat: first feature")

	writeFile(t, dir, "other.txt", "other content")
	runCmd(t, dir, "git", "add", "other.txt")
	runCmd(t, dir, "git", "commit", "-m", "fix(auth): fix bug")

	commits, err := GetCommitsInRange(dir, initialSHA, "HEAD")
	if err != nil {
		t.Fatalf("GetCommitsInRange failed: %v", err)
	}

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// Commits should be in chronological order (oldest first due to --reverse)
	if commits[0].Type != "feat" {
		t.Errorf("first commit type: got %s, want feat", commits[0].Type)
	}
	if commits[1].Type != "fix" {
		t.Errorf("second commit type: got %s, want fix", commits[1].Type)
	}
	if commits[1].Scope != "auth" {
		t.Errorf("second commit scope: got %s, want auth", commits[1].Scope)
	}
}

func TestGetCommitsInRange_WithFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestGitRepo(t)

	// Create initial commit
	writeFile(t, dir, "file.txt", "initial")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")
	initialSHA, _ := runGit(dir, "rev-parse", "HEAD")

	// Create commit with multiple files
	writeFile(t, dir, "workloads/service-a/main.go", "code")
	writeFile(t, dir, "workloads/service-a/config.yaml", "config")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add service")

	commits, err := GetCommitsInRange(dir, initialSHA, "HEAD")
	if err != nil {
		t.Fatalf("GetCommitsInRange failed: %v", err)
	}

	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}

	if len(commits[0].Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(commits[0].Files))
	}
}

func TestGetCommitsInRange_MergeCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestGitRepo(t)

	// Create initial commit on main
	writeFile(t, dir, "file.txt", "initial")
	runCmd(t, dir, "git", "add", "file.txt")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")
	initialSHA, _ := runGit(dir, "rev-parse", "HEAD")

	// Create feature branch with commits
	runCmd(t, dir, "git", "checkout", "-b", "feature")

	writeFile(t, dir, "feature1.txt", "content")
	runCmd(t, dir, "git", "add", "feature1.txt")
	runCmd(t, dir, "git", "commit", "-m", "feat: feature 1")

	writeFile(t, dir, "feature2.txt", "content")
	runCmd(t, dir, "git", "add", "feature2.txt")
	runCmd(t, dir, "git", "commit", "-m", "fix: fix in feature")

	featureHeadSHA, _ := runGit(dir, "rev-parse", "HEAD")

	// Go back to main and merge with --no-ff
	runCmd(t, dir, "git", "checkout", "main")
	runCmd(t, dir, "git", "merge", "--no-ff", "feature", "-m", "Merge branch 'feature'")

	// Now analyze the merge and get commits
	info, err := AnalyzeHead(dir)
	if err != nil {
		t.Fatalf("AnalyzeHead failed: %v", err)
	}

	if !info.IsMerge {
		t.Fatal("expected merge commit")
	}

	// Get commits from merge base to feature head
	commits, err := GetCommitsInRange(dir, info.MergeBase, featureHeadSHA)
	if err != nil {
		t.Fatalf("GetCommitsInRange failed: %v", err)
	}

	if len(commits) != 2 {
		t.Errorf("expected 2 commits in merge, got %d", len(commits))
	}

	// Verify we got the feature commits, not just the merge commit
	foundFeat := false
	foundFix := false
	for _, c := range commits {
		if c.Type == "feat" {
			foundFeat = true
		}
		if c.Type == "fix" {
			foundFix = true
		}
	}
	if !foundFeat {
		t.Error("expected to find feat commit")
	}
	if !foundFix {
		t.Error("expected to find fix commit")
	}

	// Verify initial commit is not included
	for _, c := range commits {
		if c.SHA == initialSHA {
			t.Error("initial commit should not be in range")
		}
	}
}

func TestIsValidSHA(t *testing.T) {
	tests := []struct {
		sha   string
		valid bool
	}{
		{"abc1234", true},
		{"abc123456789012345678901234567890123456a", true}, // 40 chars exactly
		{"abc123", false},  // Too short
		{"xyz1234", false}, // Invalid character
		{"ABC1234", false}, // Uppercase
		{"", false},
		{"abc1234567890123456789012345678901234567890", false}, // 41 chars - Too long
	}

	for _, tc := range tests {
		t.Run(tc.sha, func(t *testing.T) {
			if got := IsValidSHA(tc.sha); got != tc.valid {
				t.Errorf("IsValidSHA(%s) = %v, want %v", tc.sha, got, tc.valid)
			}
		})
	}
}
