package release

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestApply_RootPackageWithBareTags covers a single-repo config: package ".",
// include-component-in-tag false, VERSION at the root.
func TestApply_RootPackageWithBareTags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dir := createTestRepo(t)
	writeFile(t, dir, "release-please-config.json", `{"include-component-in-tag": false, "packages": {".": {"component": "tool"}}}`)
	writeFile(t, dir, "release-please-manifest.json", "{\n  \".\": \"1.3.0\"\n}\n")
	writeFile(t, dir, "VERSION", "1.3.0\n")
	writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [1.3.0](x) (2026-01-01)\n")
	writeFile(t, dir, "main.go", "package main\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")
	writeFile(t, dir, "cmd/tool/new.go", "package main\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "feat: add a command")

	result, err := Analyze(&Options{RepoPath: dir, RepoURL: "https://github.com/owner/tool", TreatPreMajorAsMinor: true})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(result.Releases) != 1 || result.Releases[0].NewVersion != "1.4.0" {
		t.Fatalf("releases = %+v, want tool 1.4.0", result.Releases)
	}
	report := BuildReleaseReport(result, result.RepoURL)
	if got := report.Releases[0]; got.TagName != "v1.4.0" || !reflect.DeepEqual(got.Files, []string{"VERSION", "CHANGELOG.md"}) {
		t.Errorf("report release = tag %q files %v", got.TagName, got.Files)
	}

	if err := Apply(result, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := readRepoFile(t, dir, "VERSION"); got != "1.4.0\n" {
		t.Errorf("VERSION = %q", got)
	}
	if got := readRepoFile(t, dir, "release-please-manifest.json"); got != "{\n  \".\": \"1.4.0\"\n}\n" {
		t.Errorf("manifest = %q", got)
	}
	if cl := readRepoFile(t, dir, "CHANGELOG.md"); !strings.Contains(cl, "https://github.com/owner/tool/compare/v1.3.0...v1.4.0") {
		t.Errorf("changelog compare link does not use the bare tags:\n%s", cl)
	}
}

// TestApply_ExtraFilesAndVersionRule: a package with extra-files and no
// VERSION gets none; one with extra-files and an existing VERSION keeps it.
func TestApply_ExtraFilesAndVersionRule(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dir := createTestRepo(t)
	writeFile(t, dir, "release-please-config.json", `{"packages": {
		"web": {"component": "web", "extra-files": [
			{"type": "json", "path": "package.json", "jsonpath": "$.version"},
			{"type": "json", "path": "package-lock.json", "jsonpath": "$.packages[\"\"].version"}]},
		"api": {"component": "api", "extra-files": ["src/version.go"]}}}`)
	writeFile(t, dir, "release-please-manifest.json", `{"web": "1.0.0", "api": "2.0.0"}`)
	pkgJSON := "{\n  \"name\": \"web\",\n  \"version\": \"1.0.0\"\n}\n"
	lockJSON := "{\n  \"name\": \"web\",\n  \"version\": \"1.0.0\",\n  \"packages\": {\n    \"\": {\n      \"version\": \"1.0.0\"\n    }\n  }\n}\n"
	writeFile(t, dir, "web/package.json", pkgJSON)
	writeFile(t, dir, "web/package-lock.json", lockJSON)
	writeFile(t, dir, "api/VERSION", "2.0.0\n")
	writeFile(t, dir, "api/src/version.go", "const Version = \"2.0.0\" // x-release-please-version\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")
	writeFile(t, dir, "web/index.js", "1\n")
	writeFile(t, dir, "api/main.go", "1\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "fix: two packages")

	result, err := Analyze(&Options{RepoPath: dir, TreatPreMajorAsMinor: true})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	report := BuildReleaseReport(result, "")
	files := map[string][]string{}
	for _, rel := range report.Releases {
		files[rel.Component] = rel.Files
	}
	wantFiles := map[string][]string{
		"api": {"api/VERSION", "api/CHANGELOG.md", "api/src/version.go"},
		"web": {"web/CHANGELOG.md", "web/package.json", "web/package-lock.json"},
	}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("report files = %v, want %v", files, wantFiles)
	}

	if err := Apply(result, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "web/VERSION")); !os.IsNotExist(err) {
		t.Errorf("web/VERSION was created; the package.json is the version of record")
	}
	if got := readRepoFile(t, dir, "web/package.json"); got != strings.Replace(pkgJSON, "1.0.0", "1.0.1", 1) {
		t.Errorf("package.json = %q", got)
	}
	wantLock := strings.Replace(lockJSON, "      \"version\": \"1.0.0\"", "      \"version\": \"1.0.1\"", 1)
	if got := readRepoFile(t, dir, "web/package-lock.json"); got != wantLock {
		t.Errorf("package-lock.json = %q, want %q", got, wantLock)
	}
	if got := readRepoFile(t, dir, "api/VERSION"); got != "2.0.1\n" {
		t.Errorf("api/VERSION = %q", got)
	}
	if got := readRepoFile(t, dir, "api/src/version.go"); got != "const Version = \"2.0.1\" // x-release-please-version\n" {
		t.Errorf("version.go = %q", got)
	}
}

func TestApply_MissingJSONPathFailsLoudly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dir := createTestRepo(t)
	writeFile(t, dir, "release-please-config.json", `{"packages": {"web": {"component": "web", "extra-files": [{"type": "json", "path": "package.json", "jsonpath": "$.nope"}]}}}`)
	writeFile(t, dir, "release-please-manifest.json", `{"web": "1.0.0"}`)
	writeFile(t, dir, "web/package.json", `{"version": "1.0.0"}`)
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "chore: initial commit")
	writeFile(t, dir, "web/a.js", "1\n")
	runCmd(t, dir, "git", "add", "-A")
	runCmd(t, dir, "git", "commit", "-m", "fix: a")

	result, err := Analyze(&Options{RepoPath: dir})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	err = Apply(result, false)
	if err == nil || !strings.Contains(err.Error(), "web/package.json") || !strings.Contains(err.Error(), "$.nope") {
		t.Errorf("Apply error = %v, want it to name the file and the jsonpath", err)
	}
}
