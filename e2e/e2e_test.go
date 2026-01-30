//go:build e2e
// +build e2e

// Package e2e contains end-to-end tests that run against the mock--gitops-playground
// GitHub repository. These tests require network access and GitHub authentication.
//
// Run with: go test -tags=e2e ./e2e/...
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spraguehouse/release-damnit/internal/release"
)

const mockRepoURL = "git@github.com:spraguehouse/mock--gitops-playground.git"

// cloneMockRepo clones the mock repo to a temp directory.
func cloneMockRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmd := exec.Command("git", "clone", mockRepoURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone mock repo: %v\n%s", err, out)
	}

	// Configure git
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")

	return dir
}

// runCmd runs a command in the given directory.
func runCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

// TestE2E_AnalyzeMockRepo verifies we can analyze the mock repo structure.
func TestE2E_AnalyzeMockRepo(t *testing.T) {
	dir := cloneMockRepo(t)

	// Verify the repo structure
	if _, err := os.Stat(filepath.Join(dir, "release-please-config.json")); err != nil {
		t.Fatalf("missing release-please-config.json: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "workloads/service-a/VERSION")); err != nil {
		t.Fatalf("missing service-a VERSION: %v", err)
	}

	// Load config to verify it parses correctly
	opts := &release.Options{
		RepoPath: dir,
		DryRun:   true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have config loaded with 4 packages
	if len(result.Config.Packages) != 4 {
		t.Errorf("expected 4 packages, got %d", len(result.Config.Packages))
	}

	// Check linked versions are detected
	if len(result.Config.LinkedGroups) != 1 {
		t.Errorf("expected 1 linked group, got %d", len(result.Config.LinkedGroups))
	}

	// Without a merge, there should be no releases (single commit repo)
	if len(result.Releases) != 0 {
		t.Errorf("expected 0 releases without merge, got %d", len(result.Releases))
	}
}

// TestE2E_MergeFeatureBranch tests merging the feature/service-a-update branch.
func TestE2E_MergeFeatureBranch(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/service-a-update to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/service-a-update", "-m", "Merge feature/service-a-update")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should be a merge commit
	if !result.MergeInfo.IsMerge {
		t.Error("expected merge commit")
	}

	// Should have 3 commits from the feature branch
	if len(result.Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(result.Commits))
	}

	// Count commit types
	var featCount, fixCount, choreCount int
	for _, c := range result.Commits {
		switch c.Type {
		case "feat":
			featCount++
		case "fix":
			fixCount++
		case "chore":
			choreCount++
		}
	}

	if featCount != 1 {
		t.Errorf("expected 1 feat commit, got %d", featCount)
	}
	if fixCount != 1 {
		t.Errorf("expected 1 fix commit, got %d", fixCount)
	}
	if choreCount != 1 {
		t.Errorf("expected 1 chore commit, got %d", choreCount)
	}

	// Should have 2 releases (service-a and service-b are linked)
	if len(result.Releases) != 2 {
		t.Fatalf("expected 2 releases (linked), got %d", len(result.Releases))
	}

	// Both should bump from 0.1.0 to 0.1.1 (feat on pre-1.0 with TreatPreMajorAsMinor)
	for _, rel := range result.Releases {
		if rel.OldVersion != "0.1.0" {
			t.Errorf("%s: expected old version 0.1.0, got %s", rel.Package.Component, rel.OldVersion)
		}
		if rel.NewVersion != "0.1.1" {
			t.Errorf("%s: expected new version 0.1.1, got %s", rel.Package.Component, rel.NewVersion)
		}
	}
}

// TestE2E_MergeMultiServiceBranch tests merging the feature/multi-service branch.
func TestE2E_MergeMultiServiceBranch(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/multi-service to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/multi-service", "-m", "Merge feature/multi-service")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have 3 commits from the feature branch
	if len(result.Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(result.Commits))
	}

	// Should have 4 releases:
	// - service-a and service-b (linked, bump together due to service-b change)
	// - service-c (standalone)
	// - shared (standalone)
	if len(result.Releases) != 4 {
		t.Fatalf("expected 4 releases, got %d", len(result.Releases))
	}

	// Verify each release
	for _, rel := range result.Releases {
		switch rel.Package.Component {
		case "service-a", "service-b":
			// Linked - both bump together
			if rel.NewVersion != "0.1.1" {
				t.Errorf("%s: expected 0.1.1, got %s", rel.Package.Component, rel.NewVersion)
			}
		case "service-c":
			// Standalone, fix commit
			if rel.NewVersion != "0.1.1" {
				t.Errorf("service-c: expected 0.1.1 (fix), got %s", rel.NewVersion)
			}
		case "shared":
			// Standalone, feat commit
			if rel.NewVersion != "0.1.1" {
				t.Errorf("shared: expected 0.1.1 (feat with TreatPreMajorAsMinor), got %s", rel.NewVersion)
			}
		}
	}
}

// TestE2E_ApplyAndVerify tests actually applying changes (without pushing).
func TestE2E_ApplyAndVerify(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature branch
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/service-a-update", "-m", "Merge feature/service-a-update")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply changes
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify VERSION files were updated
	for _, rel := range result.Releases {
		versionPath := filepath.Join(dir, rel.Package.Path, "VERSION")
		content, err := os.ReadFile(versionPath)
		if err != nil {
			t.Fatalf("failed to read VERSION for %s: %v", rel.Package.Component, err)
		}

		if !strings.Contains(string(content), rel.NewVersion) {
			t.Errorf("%s VERSION not updated: expected %s, got %s",
				rel.Package.Component, rel.NewVersion, string(content))
		}
	}

	// Verify CHANGELOG was updated
	changelogPath := filepath.Join(dir, "workloads/service-a/CHANGELOG.md")
	content, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("failed to read CHANGELOG: %v", err)
	}

	if !strings.Contains(string(content), "## [0.1.1]") {
		t.Error("CHANGELOG missing new version header")
	}

	if !strings.Contains(string(content), "add new endpoint") {
		t.Error("CHANGELOG missing feature commit")
	}

	// Verify manifest was updated
	manifestPath := filepath.Join(dir, "release-please-manifest.json")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	if !strings.Contains(string(manifestContent), `"0.1.1"`) {
		t.Error("manifest not updated to 0.1.1")
	}
}

// TestE2E_DryRunMakesNoChanges verifies dry run doesn't modify files.
func TestE2E_DryRunMakesNoChanges(t *testing.T) {
	dir := cloneMockRepo(t)

	// Record original file contents
	originalVersion, _ := os.ReadFile(filepath.Join(dir, "workloads/service-a/VERSION"))
	originalManifest, _ := os.ReadFile(filepath.Join(dir, "release-please-manifest.json"))

	// Merge feature branch
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/service-a-update", "-m", "Merge feature/service-a-update")

	// Analyze with dry run
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply with dry run
	if err := release.Apply(result, true); err != nil {
		t.Fatalf("Apply (dry run) failed: %v", err)
	}

	// Verify files are unchanged
	currentVersion, _ := os.ReadFile(filepath.Join(dir, "workloads/service-a/VERSION"))
	if string(currentVersion) != string(originalVersion) {
		t.Error("dry run modified VERSION file")
	}

	currentManifest, _ := os.ReadFile(filepath.Join(dir, "release-please-manifest.json"))
	if string(currentManifest) != string(originalManifest) {
		t.Error("dry run modified manifest file")
	}
}
