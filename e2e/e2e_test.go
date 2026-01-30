//go:build e2e
// +build e2e

// Package e2e contains end-to-end tests that run against the mock--gitops-playground
// GitHub repository. These tests require network access and GitHub authentication.
//
// Run with: go test -tags=e2e ./e2e/...
//
// The mock repo can be reset to a clean state with: ./scripts/setup-mock-repo.sh
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// runCmdIgnoreError runs a command and ignores errors (for cleanup).
func runCmdIgnoreError(dir string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	_ = cmd.Run()
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

// TestE2E_FullGitHubReleaseFlow tests the complete flow including pushing to GitHub
// and creating real GitHub releases. This is the true end-to-end test.
//
// This test:
// 1. Clones the mock repo
// 2. Merges a feature branch
// 3. Applies version updates
// 4. Commits and pushes to a test branch
// 5. Creates real GitHub releases
// 6. Verifies releases exist on GitHub
// 7. Cleans up releases and test branch
func TestE2E_FullGitHubReleaseFlow(t *testing.T) {
	// Check if gh CLI is authenticated
	if err := release.CheckGHCLI(); err != nil {
		t.Fatalf("gh CLI not authenticated: %v", err)
	}

	dir := cloneMockRepo(t)

	// Create a unique test branch to avoid conflicts with parallel tests
	testBranch := fmt.Sprintf("test-release-%d", time.Now().UnixNano())
	runCmd(t, dir, "git", "checkout", "-b", testBranch)

	// Merge feature/service-a-update with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/service-a-update", "-m", "Merge feature/service-a-update")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		RepoURL:              "https://github.com/spraguehouse/mock--gitops-playground",
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply version updates
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Commit the version changes
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: release versions")

	// Push to the test branch on origin
	runCmd(t, dir, "git", "push", "-u", "origin", testBranch)

	// Track created releases for cleanup
	var createdTags []string
	defer func() {
		// Cleanup: delete releases and test branch
		for _, tag := range createdTags {
			t.Logf("Cleaning up release: %s", tag)
			runCmdIgnoreError(dir, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
		}
		t.Logf("Cleaning up branch: %s", testBranch)
		runCmdIgnoreError(dir, "git", "push", "origin", "--delete", testBranch)
	}()

	// Create GitHub releases
	ghOpts := &release.GitHubReleaseOptions{
		RepoPath: dir,
		DryRun:   false,
	}

	ghReleases, err := release.CreateGitHubReleases(result, ghOpts)
	if err != nil {
		t.Fatalf("CreateGitHubReleases failed: %v", err)
	}

	// Track releases for cleanup
	for _, ghRel := range ghReleases {
		createdTags = append(createdTags, ghRel.TagName)
	}

	// Verify we created the expected number of releases
	if len(ghReleases) != 2 {
		t.Fatalf("expected 2 releases (service-a and service-b linked), got %d", len(ghReleases))
	}

	// Verify each release exists on GitHub
	for _, ghRel := range ghReleases {
		t.Logf("Verifying release: %s", ghRel.TagName)

		// Use gh CLI to verify the release exists
		cmd := exec.Command("gh", "release", "view", ghRel.TagName, "--json", "tagName,name")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("release %s not found on GitHub: %v\n%s", ghRel.TagName, err, out)
			continue
		}

		// Verify the release has the expected tag name
		if !strings.Contains(string(out), ghRel.TagName) {
			t.Errorf("release %s: unexpected response: %s", ghRel.TagName, out)
		}
	}

	// Verify release notes content
	for _, ghRel := range ghReleases {
		if ghRel.Notes == "" {
			t.Errorf("release %s has empty notes", ghRel.TagName)
		}
		if !strings.Contains(ghRel.Notes, "## ") {
			t.Errorf("release %s notes missing version header", ghRel.TagName)
		}
	}

	t.Logf("Successfully created and verified %d GitHub releases", len(ghReleases))
}

// TestE2E_MultiServiceGitHubReleases tests creating releases for multiple services.
func TestE2E_MultiServiceGitHubReleases(t *testing.T) {
	// Check if gh CLI is authenticated
	if err := release.CheckGHCLI(); err != nil {
		t.Fatalf("gh CLI not authenticated: %v", err)
	}

	dir := cloneMockRepo(t)

	// Create a unique test branch
	testBranch := fmt.Sprintf("test-multi-%d", time.Now().UnixNano())
	runCmd(t, dir, "git", "checkout", "-b", testBranch)

	// Merge feature/multi-service with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/multi-service", "-m", "Merge feature/multi-service")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		RepoURL:              "https://github.com/spraguehouse/mock--gitops-playground",
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply version updates
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Commit and push
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: release versions")
	runCmd(t, dir, "git", "push", "-u", "origin", testBranch)

	// Track created releases for cleanup
	var createdTags []string
	defer func() {
		for _, tag := range createdTags {
			t.Logf("Cleaning up release: %s", tag)
			runCmdIgnoreError(dir, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
		}
		t.Logf("Cleaning up branch: %s", testBranch)
		runCmdIgnoreError(dir, "git", "push", "origin", "--delete", testBranch)
	}()

	// Create GitHub releases
	ghOpts := &release.GitHubReleaseOptions{
		RepoPath: dir,
		DryRun:   false,
	}

	ghReleases, err := release.CreateGitHubReleases(result, ghOpts)
	if err != nil {
		t.Fatalf("CreateGitHubReleases failed: %v", err)
	}

	// Track releases for cleanup
	for _, ghRel := range ghReleases {
		createdTags = append(createdTags, ghRel.TagName)
	}

	// Should have 4 releases (service-a, service-b linked, plus service-c, shared)
	if len(ghReleases) != 4 {
		t.Fatalf("expected 4 releases, got %d", len(ghReleases))
	}

	// Verify all releases exist on GitHub
	for _, ghRel := range ghReleases {
		cmd := exec.Command("gh", "release", "view", ghRel.TagName, "--json", "tagName")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("release %s not found on GitHub: %v\n%s", ghRel.TagName, err, out)
		}
	}

	// Check that the right components were released
	components := make(map[string]bool)
	for _, ghRel := range ghReleases {
		components[ghRel.PackageInfo.Package.Component] = true
	}

	expectedComponents := []string{"service-a", "service-b", "service-c", "shared"}
	for _, comp := range expectedComponents {
		if !components[comp] {
			t.Errorf("missing release for component: %s", comp)
		}
	}

	t.Logf("Successfully created and verified %d GitHub releases", len(ghReleases))
}

// TestE2E_CheckGHCLI verifies the gh CLI check works.
func TestE2E_CheckGHCLI(t *testing.T) {
	err := release.CheckGHCLI()
	if err != nil {
		t.Fatalf("gh CLI not authenticated - E2E tests require authentication: %v", err)
	}
	t.Log("gh CLI is authenticated")
}
