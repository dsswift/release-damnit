package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createTestRepo creates a temporary git repository for testing.
func createTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

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
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

// setupBasicRepo creates a repo with release-please config and one package.
func setupBasicRepo(t *testing.T) string {
	t.Helper()

	dir := createTestRepo(t)

	// Create config
	writeFile(t, dir, "release-please-config.json", `{
		"packages": {
			"workloads/service-a": {
				"component": "service-a"
			}
		}
	}`)

	// Create manifest
	writeFile(t, dir, "release-please-manifest.json", `{
		"workloads/service-a": "0.1.0"
	}`)

	// Create package structure
	writeFile(t, dir, "workloads/service-a/VERSION", "0.1.0 # x-release-please-version\n")
	writeFile(t, dir, "workloads/service-a/CHANGELOG.md", "# Changelog\n\n## [0.1.0] - Initial\n")
	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n")

	// Initial commit
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")

	return dir
}

func TestAnalyze_NonMergeCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := setupBasicRepo(t)

	// Add a feature commit
	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n// Feature\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add feature")

	// Analyze
	opts := &Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should detect non-merge commit
	if result.MergeInfo.IsMerge {
		t.Error("expected non-merge commit")
	}

	// Should have 1 commit
	if len(result.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Commits))
	}

	// Should have 1 release
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	if rel.Package.Component != "service-a" {
		t.Errorf("expected service-a, got %s", rel.Package.Component)
	}
	if rel.OldVersion != "0.1.0" {
		t.Errorf("expected old version 0.1.0, got %s", rel.OldVersion)
	}
	// With TreatPreMajorAsMinor=true, feat on 0.x becomes patch
	if rel.NewVersion != "0.1.1" {
		t.Errorf("expected new version 0.1.1, got %s", rel.NewVersion)
	}
}

func TestAnalyze_MergeCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := setupBasicRepo(t)

	// Create feature branch
	runCmd(t, dir, "git", "checkout", "-b", "feature/test")

	// Add commits on feature branch
	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n// Feature 1\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add feature 1")

	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n// Feature 1\n// Fix\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "fix(service-a): fix bug")

	// Merge to main with --no-ff
	runCmd(t, dir, "git", "checkout", "main")
	runCmd(t, dir, "git", "merge", "--no-ff", "feature/test", "-m", "Merge branch 'feature/test'")

	// Analyze
	opts := &Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should detect merge commit
	if !result.MergeInfo.IsMerge {
		t.Error("expected merge commit")
	}

	// Should have 2 commits from the merge
	if len(result.Commits) != 2 {
		t.Errorf("expected 2 commits, got %d", len(result.Commits))
	}

	// Should have 1 release
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	// Highest bump is feat (minor), but with TreatPreMajorAsMinor, becomes patch
	if rel.NewVersion != "0.1.1" {
		t.Errorf("expected new version 0.1.1, got %s", rel.NewVersion)
	}
}

func TestAnalyze_MultiplePackages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestRepo(t)

	// Create config with multiple packages
	writeFile(t, dir, "release-please-config.json", `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-b": {"component": "service-b"}
		}
	}`)

	writeFile(t, dir, "release-please-manifest.json", `{
		"workloads/service-a": "0.1.0",
		"workloads/service-b": "1.0.0"
	}`)

	// Create package structures
	writeFile(t, dir, "workloads/service-a/VERSION", "0.1.0\n")
	writeFile(t, dir, "workloads/service-a/CHANGELOG.md", "# Changelog\n")
	writeFile(t, dir, "workloads/service-a/src/main.go", "// A\n")

	writeFile(t, dir, "workloads/service-b/VERSION", "1.0.0\n")
	writeFile(t, dir, "workloads/service-b/CHANGELOG.md", "# Changelog\n")
	writeFile(t, dir, "workloads/service-b/src/main.go", "// B\n")

	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")

	// Create feature branch with commits touching both packages
	runCmd(t, dir, "git", "checkout", "-b", "feature/multi")

	writeFile(t, dir, "workloads/service-a/src/main.go", "// A\n// Feature\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add feature to A")

	writeFile(t, dir, "workloads/service-b/src/main.go", "// B\n// Fix\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "fix(service-b): fix bug in B")

	// Merge
	runCmd(t, dir, "git", "checkout", "main")
	runCmd(t, dir, "git", "merge", "--no-ff", "feature/multi", "-m", "Merge branch 'feature/multi'")

	// Analyze
	opts := &Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have 2 releases
	if len(result.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(result.Releases))
	}

	// Find each release
	var relA, relB *PackageRelease
	for _, rel := range result.Releases {
		switch rel.Package.Component {
		case "service-a":
			relA = rel
		case "service-b":
			relB = rel
		}
	}

	if relA == nil {
		t.Fatal("expected release for service-a")
	}
	if relB == nil {
		t.Fatal("expected release for service-b")
	}

	// service-a: feat on 0.x with TreatPreMajorAsMinor -> 0.1.1
	if relA.NewVersion != "0.1.1" {
		t.Errorf("service-a: expected 0.1.1, got %s", relA.NewVersion)
	}

	// service-b: fix on 1.x -> 1.0.1
	if relB.NewVersion != "1.0.1" {
		t.Errorf("service-b: expected 1.0.1, got %s", relB.NewVersion)
	}
}

func TestAnalyze_LinkedVersions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := createTestRepo(t)

	// Create config with linked packages
	writeFile(t, dir, "release-please-config.json", `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-b": {"component": "service-b"}
		},
		"plugins": [
			{
				"type": "linked-versions",
				"groupName": "services",
				"components": ["service-a", "service-b"]
			}
		]
	}`)

	writeFile(t, dir, "release-please-manifest.json", `{
		"workloads/service-a": "1.0.0",
		"workloads/service-b": "1.0.0"
	}`)

	writeFile(t, dir, "workloads/service-a/VERSION", "1.0.0\n")
	writeFile(t, dir, "workloads/service-a/CHANGELOG.md", "# Changelog\n")
	writeFile(t, dir, "workloads/service-a/src/main.go", "// A\n")

	writeFile(t, dir, "workloads/service-b/VERSION", "1.0.0\n")
	writeFile(t, dir, "workloads/service-b/CHANGELOG.md", "# Changelog\n")
	writeFile(t, dir, "workloads/service-b/src/main.go", "// B\n")

	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")

	// Only touch service-a
	writeFile(t, dir, "workloads/service-a/src/main.go", "// A\n// Feature\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add feature")

	// Analyze
	opts := &Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: false, // Don't treat pre-major as minor
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have 2 releases (both linked packages bump together)
	if len(result.Releases) != 2 {
		t.Fatalf("expected 2 releases (linked), got %d", len(result.Releases))
	}

	// Both should bump to 1.1.0 (feat = minor)
	for _, rel := range result.Releases {
		if rel.NewVersion != "1.1.0" {
			t.Errorf("%s: expected 1.1.0, got %s", rel.Package.Component, rel.NewVersion)
		}
	}
}

func TestAnalyze_NoReleasableCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := setupBasicRepo(t)

	// Add a chore commit (not releasable)
	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n// Comment update\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore(service-a): update comments")

	// Analyze
	opts := &Options{
		RepoPath: dir,
		DryRun:   true,
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have no releases
	if len(result.Releases) != 0 {
		t.Errorf("expected 0 releases for chore commit, got %d", len(result.Releases))
	}
}

func TestApply_UpdatesFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := setupBasicRepo(t)

	// Add a feature commit
	writeFile(t, dir, "workloads/service-a/src/main.go", "// Initial\n// Feature\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat(service-a): add feature")

	// Analyze
	opts := &Options{
		RepoPath:             dir,
		DryRun:               false,
		TreatPreMajorAsMinor: true,
	}

	result, err := Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply changes
	if err := Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify VERSION file was updated
	versionContent, err := os.ReadFile(filepath.Join(dir, "workloads/service-a/VERSION"))
	if err != nil {
		t.Fatalf("failed to read VERSION file: %v", err)
	}
	if string(versionContent) != "0.1.1 # x-release-please-version\n" {
		t.Errorf("VERSION file not updated correctly: %s", string(versionContent))
	}

	// Verify CHANGELOG was updated
	changelogContent, err := os.ReadFile(filepath.Join(dir, "workloads/service-a/CHANGELOG.md"))
	if err != nil {
		t.Fatalf("failed to read CHANGELOG file: %v", err)
	}
	if !contains(string(changelogContent), "## [0.1.1]") {
		t.Errorf("CHANGELOG missing new version entry: %s", string(changelogContent))
	}
	if !contains(string(changelogContent), "add feature") {
		t.Errorf("CHANGELOG missing commit description: %s", string(changelogContent))
	}

	// Verify manifest was updated
	manifestContent, err := os.ReadFile(filepath.Join(dir, "release-please-manifest.json"))
	if err != nil {
		t.Fatalf("failed to read manifest file: %v", err)
	}
	if !contains(string(manifestContent), `"0.1.1"`) {
		t.Errorf("manifest not updated correctly: %s", string(manifestContent))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
