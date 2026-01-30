//go:build e2e
// +build e2e

// Package e2e contains end-to-end tests that run against the mock--gitops-playground
// GitHub repository. These tests require network access and GitHub authentication.
//
// The mock repo mirrors sh-monorepo structure with:
// - Nested packages (jarvis, jarvis-web inside jarvis/clients/web)
// - Linked versions (ma-observe-client and ma-observe-server)
// - Multiple feature branches testing different scenarios
//
// Run with: go test -tags=e2e ./e2e/...
//
// Reset the mock repo with: ./scripts/setup-mock-repo.sh
package e2e

import (
	"encoding/json"
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
const mockRepoHTTPS = "https://github.com/spraguehouse/mock--gitops-playground"

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

// =============================================================================
// Scenario Tests - Each tests a specific release scenario
// =============================================================================

// TestE2E_Scenario_SingleFeat tests a single feat commit to one package.
// Expected: jarvis bumps from 0.1.0 to 0.1.1
func TestE2E_Scenario_SingleFeat(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/single-feat to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/single-feat", "-m", "Merge feature/single-feat")

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

	// Verify merge commit detection
	if !result.MergeInfo.IsMerge {
		t.Error("expected merge commit")
	}

	// Should have 1 commit
	if len(result.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Commits))
	}

	// Should have 1 release (jarvis only)
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	if rel.Package.Component != "jarvis" {
		t.Errorf("expected jarvis, got %s", rel.Package.Component)
	}
	if rel.OldVersion != "0.1.0" {
		t.Errorf("expected old version 0.1.0, got %s", rel.OldVersion)
	}
	if rel.NewVersion != "0.1.1" {
		t.Errorf("expected new version 0.1.1, got %s", rel.NewVersion)
	}
}

// TestE2E_Scenario_MultiPackage tests commits touching multiple packages.
// Expected: jarvis, sandbox-portal, infrastructure all bump to 0.1.1
func TestE2E_Scenario_MultiPackage(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/multi-package to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/multi-package", "-m", "Merge feature/multi-package")

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

	// Should have 3 commits
	if len(result.Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(result.Commits))
	}

	// Should have 3 releases
	if len(result.Releases) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(result.Releases))
	}

	// Check each release
	components := make(map[string]*release.PackageRelease)
	for _, rel := range result.Releases {
		components[rel.Package.Component] = rel
	}

	expectedComponents := []string{"jarvis", "sandbox-portal", "infrastructure"}
	for _, comp := range expectedComponents {
		rel, ok := components[comp]
		if !ok {
			t.Errorf("missing release for %s", comp)
			continue
		}
		if rel.NewVersion != "0.1.1" {
			t.Errorf("%s: expected 0.1.1, got %s", comp, rel.NewVersion)
		}
	}
}

// TestE2E_Scenario_BreakingChange tests a breaking change commit.
// Expected: sandbox-portal bumps from 0.1.0 to 1.0.0 (major bump)
func TestE2E_Scenario_BreakingChange(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/breaking-change to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/breaking-change", "-m", "Merge feature/breaking-change")

	// Analyze WITHOUT TreatPreMajorAsMinor to see major bump
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               true,
		TreatPreMajorAsMinor: false, // Don't suppress major bump
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have 1 commit
	if len(result.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Commits))
	}

	// Verify the commit is marked as breaking
	if len(result.Commits) > 0 && !result.Commits[0].IsBreaking {
		t.Error("expected commit to be marked as breaking")
	}

	// Should have 1 release with major bump
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	if rel.Package.Component != "sandbox-portal" {
		t.Errorf("expected sandbox-portal, got %s", rel.Package.Component)
	}
	if rel.NewVersion != "1.0.0" {
		t.Errorf("expected major bump to 1.0.0, got %s", rel.NewVersion)
	}
}

// TestE2E_Scenario_LinkedVersions tests linked versions behavior.
// Expected: Change to ma-observe-client bumps BOTH client and server
func TestE2E_Scenario_LinkedVersions(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/linked-versions to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/linked-versions", "-m", "Merge feature/linked-versions")

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

	// Should have 1 commit (only touched client)
	if len(result.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Commits))
	}

	// Should have 2 releases (both client AND server due to linking)
	if len(result.Releases) != 2 {
		t.Fatalf("expected 2 releases (linked), got %d", len(result.Releases))
	}

	// Both should bump to same version
	components := make(map[string]string)
	for _, rel := range result.Releases {
		components[rel.Package.Component] = rel.NewVersion
	}

	if _, ok := components["ma-observe-client"]; !ok {
		t.Error("missing ma-observe-client release")
	}
	if _, ok := components["ma-observe-server"]; !ok {
		t.Error("missing ma-observe-server release (should be linked)")
	}

	// Both should have same version
	clientVersion := components["ma-observe-client"]
	serverVersion := components["ma-observe-server"]
	if clientVersion != serverVersion {
		t.Errorf("linked packages have different versions: client=%s, server=%s", clientVersion, serverVersion)
	}
	if clientVersion != "0.1.1" {
		t.Errorf("expected 0.1.1, got %s", clientVersion)
	}
}

// TestE2E_Scenario_StackedCommits tests multiple commits with varying severities.
// Expected: Highest severity (feat) wins, jarvis-discord bumps to 0.1.1
func TestE2E_Scenario_StackedCommits(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/stacked-commits to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/stacked-commits", "-m", "Merge feature/stacked-commits")

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

	// Should have 4 commits (chore, fix, feat, fix)
	if len(result.Commits) != 4 {
		t.Errorf("expected 4 commits, got %d", len(result.Commits))
	}

	// Count by type
	typeCounts := make(map[string]int)
	for _, c := range result.Commits {
		typeCounts[c.Type]++
	}

	if typeCounts["chore"] != 1 {
		t.Errorf("expected 1 chore commit, got %d", typeCounts["chore"])
	}
	if typeCounts["fix"] != 2 {
		t.Errorf("expected 2 fix commits, got %d", typeCounts["fix"])
	}
	if typeCounts["feat"] != 1 {
		t.Errorf("expected 1 feat commit, got %d", typeCounts["feat"])
	}

	// Should have 1 release (jarvis-discord)
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	if rel.Package.Component != "jarvis-discord" {
		t.Errorf("expected jarvis-discord, got %s", rel.Package.Component)
	}
	// Feat wins over fix and chore
	if rel.NewVersion != "0.1.1" {
		t.Errorf("expected 0.1.1 (feat wins), got %s", rel.NewVersion)
	}
}

// TestE2E_Scenario_NestedPackage tests nested package path matching.
// Expected: Change to jarvis/clients/web bumps jarvis-web, NOT jarvis
func TestE2E_Scenario_NestedPackage(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature/nested-package to main with --no-ff
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/nested-package", "-m", "Merge feature/nested-package")

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

	// Should have 1 commit
	if len(result.Commits) != 1 {
		t.Errorf("expected 1 commit, got %d", len(result.Commits))
	}

	// Should have 1 release - jarvis-web (NOT jarvis)
	if len(result.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(result.Releases))
	}

	rel := result.Releases[0]
	if rel.Package.Component != "jarvis-web" {
		t.Errorf("expected jarvis-web (deepest match), got %s", rel.Package.Component)
	}
	if rel.NewVersion != "0.1.1" {
		t.Errorf("expected 0.1.1, got %s", rel.NewVersion)
	}
}

// =============================================================================
// Full GitHub Release Flow Tests
// =============================================================================

// TestE2E_FullGitHubReleaseFlow_SingleFeat tests the complete GitHub release flow.
// This is the true end-to-end test that:
// 1. Clones the mock repo
// 2. Creates a unique test branch
// 3. Merges feature/single-feat
// 4. Applies version updates
// 5. Commits and pushes to origin
// 6. Creates real GitHub releases
// 7. Verifies releases exist on GitHub
// 8. Cleans up releases and test branch
func TestE2E_FullGitHubReleaseFlow_SingleFeat(t *testing.T) {
	if err := release.CheckGHCLI(); err != nil {
		t.Fatalf("gh CLI not authenticated: %v", err)
	}

	dir := cloneMockRepo(t)

	// Create unique test branch
	testBranch := fmt.Sprintf("test-single-feat-%d", time.Now().UnixNano())
	runCmd(t, dir, "git", "checkout", "-b", testBranch)

	// Merge feature
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/single-feat", "-m", "Merge feature/single-feat")

	// Analyze
	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		RepoURL:              mockRepoHTTPS,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Commit and push
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: release jarvis-v0.1.1")
	runCmd(t, dir, "git", "push", "-u", "origin", testBranch)

	// Track for cleanup
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

	for _, ghRel := range ghReleases {
		createdTags = append(createdTags, ghRel.TagName)
	}

	// Verify
	if len(ghReleases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(ghReleases))
	}

	ghRel := ghReleases[0]
	if ghRel.TagName != "jarvis-v0.1.1" {
		t.Errorf("expected tag jarvis-v0.1.1, got %s", ghRel.TagName)
	}

	// Verify on GitHub
	cmd := exec.Command("gh", "release", "view", ghRel.TagName, "--json", "tagName,name")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release %s not found on GitHub: %v\n%s", ghRel.TagName, err, out)
	}

	t.Logf("Created and verified GitHub release: %s", ghRel.TagName)
}

// TestE2E_FullGitHubReleaseFlow_LinkedVersions tests linked versions with real GitHub releases.
func TestE2E_FullGitHubReleaseFlow_LinkedVersions(t *testing.T) {
	if err := release.CheckGHCLI(); err != nil {
		t.Fatalf("gh CLI not authenticated: %v", err)
	}

	dir := cloneMockRepo(t)

	testBranch := fmt.Sprintf("test-linked-%d", time.Now().UnixNano())
	runCmd(t, dir, "git", "checkout", "-b", testBranch)
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/linked-versions", "-m", "Merge feature/linked-versions")

	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		RepoURL:              mockRepoHTTPS,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: release ma-observe-v0.1.1")
	runCmd(t, dir, "git", "push", "-u", "origin", testBranch)

	var createdTags []string
	defer func() {
		for _, tag := range createdTags {
			t.Logf("Cleaning up release: %s", tag)
			runCmdIgnoreError(dir, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
		}
		t.Logf("Cleaning up branch: %s", testBranch)
		runCmdIgnoreError(dir, "git", "push", "origin", "--delete", testBranch)
	}()

	ghOpts := &release.GitHubReleaseOptions{
		RepoPath: dir,
		DryRun:   false,
	}

	ghReleases, err := release.CreateGitHubReleases(result, ghOpts)
	if err != nil {
		t.Fatalf("CreateGitHubReleases failed: %v", err)
	}

	for _, ghRel := range ghReleases {
		createdTags = append(createdTags, ghRel.TagName)
	}

	// Should have 2 releases (client AND server due to linking)
	if len(ghReleases) != 2 {
		t.Fatalf("expected 2 releases (linked), got %d", len(ghReleases))
	}

	// Verify both exist on GitHub
	expectedTags := []string{"ma-observe-client-v0.1.1", "ma-observe-server-v0.1.1"}
	for _, expectedTag := range expectedTags {
		found := false
		for _, ghRel := range ghReleases {
			if ghRel.TagName == expectedTag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected release: %s", expectedTag)
		}

		cmd := exec.Command("gh", "release", "view", expectedTag, "--json", "tagName")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("release %s not found on GitHub: %v\n%s", expectedTag, err, out)
		}
	}

	t.Logf("Created and verified %d linked GitHub releases", len(ghReleases))
}

// TestE2E_FullGitHubReleaseFlow_MultiPackage tests multiple packages with real GitHub releases.
func TestE2E_FullGitHubReleaseFlow_MultiPackage(t *testing.T) {
	if err := release.CheckGHCLI(); err != nil {
		t.Fatalf("gh CLI not authenticated: %v", err)
	}

	dir := cloneMockRepo(t)

	testBranch := fmt.Sprintf("test-multi-%d", time.Now().UnixNano())
	runCmd(t, dir, "git", "checkout", "-b", testBranch)
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/multi-package", "-m", "Merge feature/multi-package")

	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		RepoURL:              mockRepoHTTPS,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: release multi-package")
	runCmd(t, dir, "git", "push", "-u", "origin", testBranch)

	var createdTags []string
	defer func() {
		for _, tag := range createdTags {
			t.Logf("Cleaning up release: %s", tag)
			runCmdIgnoreError(dir, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
		}
		t.Logf("Cleaning up branch: %s", testBranch)
		runCmdIgnoreError(dir, "git", "push", "origin", "--delete", testBranch)
	}()

	ghOpts := &release.GitHubReleaseOptions{
		RepoPath: dir,
		DryRun:   false,
	}

	ghReleases, err := release.CreateGitHubReleases(result, ghOpts)
	if err != nil {
		t.Fatalf("CreateGitHubReleases failed: %v", err)
	}

	for _, ghRel := range ghReleases {
		createdTags = append(createdTags, ghRel.TagName)
	}

	// Should have 3 releases
	if len(ghReleases) != 3 {
		t.Fatalf("expected 3 releases, got %d", len(ghReleases))
	}

	// Verify expected components
	expectedComponents := map[string]bool{
		"jarvis":          false,
		"sandbox-portal":  false,
		"infrastructure":  false,
	}

	for _, ghRel := range ghReleases {
		comp := ghRel.PackageInfo.Package.Component
		if _, ok := expectedComponents[comp]; ok {
			expectedComponents[comp] = true
		}

		// Verify on GitHub
		cmd := exec.Command("gh", "release", "view", ghRel.TagName, "--json", "tagName")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("release %s not found on GitHub: %v\n%s", ghRel.TagName, err, out)
		}
	}

	for comp, found := range expectedComponents {
		if !found {
			t.Errorf("missing release for component: %s", comp)
		}
	}

	t.Logf("Created and verified %d GitHub releases for multi-package scenario", len(ghReleases))
}

// =============================================================================
// File Update Verification Tests
// =============================================================================

// TestE2E_ApplyUpdatesFiles verifies that Apply correctly updates all files.
func TestE2E_ApplyUpdatesFiles(t *testing.T) {
	dir := cloneMockRepo(t)

	// Merge feature
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/single-feat", "-m", "Merge feature/single-feat")

	opts := &release.Options{
		RepoPath:             dir,
		DryRun:               false,
		TreatPreMajorAsMinor: true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Apply
	if err := release.Apply(result, false); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify VERSION file
	versionPath := filepath.Join(dir, "workloads/jarvis/VERSION")
	versionContent, err := os.ReadFile(versionPath)
	if err != nil {
		t.Fatalf("failed to read VERSION: %v", err)
	}
	if !strings.Contains(string(versionContent), "0.1.1") {
		t.Errorf("VERSION not updated: %s", versionContent)
	}

	// Verify CHANGELOG
	changelogPath := filepath.Join(dir, "workloads/jarvis/CHANGELOG.md")
	changelogContent, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("failed to read CHANGELOG: %v", err)
	}
	if !strings.Contains(string(changelogContent), "## [0.1.1]") {
		t.Error("CHANGELOG missing new version header")
	}
	if !strings.Contains(string(changelogContent), "calendar integration") {
		t.Error("CHANGELOG missing feature commit description")
	}

	// Verify manifest
	manifestPath := filepath.Join(dir, "release-please-manifest.json")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var manifest map[string]string
	if err := json.Unmarshal(manifestContent, &manifest); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if manifest["workloads/jarvis"] != "0.1.1" {
		t.Errorf("manifest not updated: jarvis = %s", manifest["workloads/jarvis"])
	}

	// Other packages should remain 0.1.0
	if manifest["workloads/jarvis/clients/web"] != "0.1.0" {
		t.Errorf("jarvis-web should be unchanged: %s", manifest["workloads/jarvis/clients/web"])
	}
}

// TestE2E_DryRunNoChanges verifies dry run doesn't modify files.
func TestE2E_DryRunNoChanges(t *testing.T) {
	dir := cloneMockRepo(t)

	// Read original files
	origVersion, _ := os.ReadFile(filepath.Join(dir, "workloads/jarvis/VERSION"))
	origManifest, _ := os.ReadFile(filepath.Join(dir, "release-please-manifest.json"))
	origChangelog, _ := os.ReadFile(filepath.Join(dir, "workloads/jarvis/CHANGELOG.md"))

	// Merge feature
	runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/single-feat", "-m", "Merge feature/single-feat")

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

	// Verify files unchanged
	newVersion, _ := os.ReadFile(filepath.Join(dir, "workloads/jarvis/VERSION"))
	if string(newVersion) != string(origVersion) {
		t.Error("dry run modified VERSION")
	}

	newManifest, _ := os.ReadFile(filepath.Join(dir, "release-please-manifest.json"))
	if string(newManifest) != string(origManifest) {
		t.Error("dry run modified manifest")
	}

	newChangelog, _ := os.ReadFile(filepath.Join(dir, "workloads/jarvis/CHANGELOG.md"))
	if string(newChangelog) != string(origChangelog) {
		t.Error("dry run modified CHANGELOG")
	}
}

// =============================================================================
// Config Verification Tests
// =============================================================================

// TestE2E_ConfigParsing verifies the mock repo config is parsed correctly.
func TestE2E_ConfigParsing(t *testing.T) {
	dir := cloneMockRepo(t)

	opts := &release.Options{
		RepoPath: dir,
		DryRun:   true,
	}

	result, err := release.Analyze(opts)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should have 8 packages
	if len(result.Config.Packages) != 8 {
		t.Errorf("expected 8 packages, got %d", len(result.Config.Packages))
	}

	// Should have 1 linked group
	if len(result.Config.LinkedGroups) != 1 {
		t.Errorf("expected 1 linked group, got %d", len(result.Config.LinkedGroups))
	}

	// Verify linked group contains ma-observe
	if components, ok := result.Config.LinkedGroups["ma-observe"]; ok {
		if len(components) != 2 {
			t.Errorf("expected 2 components in linked group, got %d", len(components))
		}
	} else {
		t.Error("missing linked group 'ma-observe'")
	}

	// Verify nested package paths
	expectedPaths := map[string]string{
		"jarvis":                  "workloads/jarvis",
		"jarvis-web":              "workloads/jarvis/clients/web",
		"jarvis-discord":          "workloads/jarvis/clients/discord",
		"ma-observe-client":       "workloads/ma-observe/client",
		"ma-observe-server":       "workloads/ma-observe/server",
		"sandbox-portal":          "platforms/sandbox/portal",
		"sandbox-image-claude-code": "platforms/sandbox/images/claude-code",
		"infrastructure":          "infrastructure/terraform",
	}

	for comp, expectedPath := range expectedPaths {
		found := false
		for _, pkg := range result.Config.Packages {
			if pkg.Component == comp {
				found = true
				if pkg.Path != expectedPath {
					t.Errorf("%s: expected path %s, got %s", comp, expectedPath, pkg.Path)
				}
				break
			}
		}
		if !found {
			t.Errorf("missing package: %s", comp)
		}
	}
}

// =============================================================================
// Utility Tests
// =============================================================================

// TestE2E_CheckGHCLI verifies gh CLI authentication.
func TestE2E_CheckGHCLI(t *testing.T) {
	err := release.CheckGHCLI()
	if err != nil {
		t.Fatalf("gh CLI not authenticated - E2E tests require authentication: %v", err)
	}
	t.Log("gh CLI is authenticated")
}
