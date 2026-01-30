package config

import (
	"os"
	"path/filepath"
	"testing"
)

// createTestRepo creates a temporary directory with release-please config files.
func createTestRepo(t *testing.T, configJSON, manifestJSON string) string {
	t.Helper()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "release-please-config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "release-please-manifest.json"), []byte(manifestJSON), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	return dir
}

func TestLoad_BasicConfig(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {
				"component": "service-a",
				"changelog-path": "CHANGELOG.md"
			},
			"workloads/service-b": {
				"component": "service-b"
			}
		}
	}`

	manifestJSON := `{
		"workloads/service-a": "1.2.3",
		"workloads/service-b": "0.1.0"
	}`

	dir := createTestRepo(t, configJSON, manifestJSON)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check packages
	if len(cfg.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(cfg.Packages))
	}

	// Check service-a
	pkgA := cfg.Packages["workloads/service-a"]
	if pkgA == nil {
		t.Fatal("expected package workloads/service-a")
	}
	if pkgA.Component != "service-a" {
		t.Errorf("expected component service-a, got %s", pkgA.Component)
	}
	if pkgA.CurrentVersion != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", pkgA.CurrentVersion)
	}
	if pkgA.ChangelogPath != "CHANGELOG.md" {
		t.Errorf("expected changelog-path CHANGELOG.md, got %s", pkgA.ChangelogPath)
	}

	// Check service-b (default changelog path)
	pkgB := cfg.Packages["workloads/service-b"]
	if pkgB == nil {
		t.Fatal("expected package workloads/service-b")
	}
	if pkgB.ChangelogPath != "CHANGELOG.md" {
		t.Errorf("expected default changelog-path CHANGELOG.md, got %s", pkgB.ChangelogPath)
	}
}

func TestLoad_LinkedVersions(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-b": {"component": "service-b"},
			"workloads/service-c": {"component": "service-c"}
		},
		"plugins": [
			{
				"type": "linked-versions",
				"groupName": "services-ab",
				"components": ["service-a", "service-b"]
			}
		]
	}`

	manifestJSON := `{
		"workloads/service-a": "1.0.0",
		"workloads/service-b": "1.0.0",
		"workloads/service-c": "2.0.0"
	}`

	dir := createTestRepo(t, configJSON, manifestJSON)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check linked groups
	if len(cfg.LinkedGroups) != 1 {
		t.Errorf("expected 1 linked group, got %d", len(cfg.LinkedGroups))
	}

	components := cfg.LinkedGroups["services-ab"]
	if len(components) != 2 {
		t.Errorf("expected 2 components in group, got %d", len(components))
	}

	// Check package linked group assignment
	pkgA := cfg.Packages["workloads/service-a"]
	if pkgA.LinkedGroup != "services-ab" {
		t.Errorf("expected LinkedGroup services-ab, got %s", pkgA.LinkedGroup)
	}

	pkgC := cfg.Packages["workloads/service-c"]
	if pkgC.LinkedGroup != "" {
		t.Errorf("expected no LinkedGroup for service-c, got %s", pkgC.LinkedGroup)
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestLoad_MissingManifestFile(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{"packages": {}}`
	os.WriteFile(filepath.Join(dir, "release-please-config.json"), []byte(configJSON), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for missing manifest file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "release-please-config.json"), []byte("not json"), 0644)
	os.WriteFile(filepath.Join(dir, "release-please-manifest.json"), []byte("{}"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoad_MissingComponent(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {}
		}
	}`
	manifestJSON := `{}`

	dir := createTestRepo(t, configJSON, manifestJSON)

	_, err := Load(dir)
	if err == nil {
		t.Error("expected error for missing component")
	}
}

func TestFindPackageForPath_BasicMatch(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-b": {"component": "service-b"}
		}
	}`
	manifestJSON := `{"workloads/service-a": "1.0.0", "workloads/service-b": "1.0.0"}`

	dir := createTestRepo(t, configJSON, manifestJSON)
	cfg, _ := Load(dir)

	tests := []struct {
		path     string
		expected string
	}{
		{"workloads/service-a/src/main.go", "service-a"},
		{"workloads/service-a/VERSION", "service-a"},
		{"workloads/service-b/src/lib.go", "service-b"},
		{"workloads/service-c/src/main.go", ""}, // No match
		{"README.md", ""},                       // No match
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			pkg := cfg.FindPackageForPath(tc.path)
			if tc.expected == "" {
				if pkg != nil {
					t.Errorf("expected no match, got %s", pkg.Component)
				}
			} else {
				if pkg == nil {
					t.Errorf("expected match %s, got nil", tc.expected)
				} else if pkg.Component != tc.expected {
					t.Errorf("expected %s, got %s", tc.expected, pkg.Component)
				}
			}
		})
	}
}

func TestFindPackageForPath_DeepestMatchWins(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/jarvis": {"component": "jarvis"},
			"workloads/jarvis/clients/web": {"component": "jarvis-web"}
		}
	}`
	manifestJSON := `{"workloads/jarvis": "1.0.0", "workloads/jarvis/clients/web": "1.0.0"}`

	dir := createTestRepo(t, configJSON, manifestJSON)
	cfg, _ := Load(dir)

	tests := []struct {
		path     string
		expected string
	}{
		{"workloads/jarvis/backend/main.go", "jarvis"},
		{"workloads/jarvis/clients/web/src/App.tsx", "jarvis-web"},
		{"workloads/jarvis/clients/android/app.kt", "jarvis"}, // Falls back to parent
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			pkg := cfg.FindPackageForPath(tc.path)
			if pkg == nil {
				t.Errorf("expected match %s, got nil", tc.expected)
			} else if pkg.Component != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, pkg.Component)
			}
		})
	}
}

func TestGetLinkedPackages_Linked(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-b": {"component": "service-b"},
			"workloads/service-c": {"component": "service-c"}
		},
		"plugins": [
			{
				"type": "linked-versions",
				"groupName": "services-ab",
				"components": ["service-a", "service-b"]
			}
		]
	}`
	manifestJSON := `{
		"workloads/service-a": "1.0.0",
		"workloads/service-b": "1.0.0",
		"workloads/service-c": "2.0.0"
	}`

	dir := createTestRepo(t, configJSON, manifestJSON)
	cfg, _ := Load(dir)

	// Get linked packages for service-a
	pkgA := cfg.Packages["workloads/service-a"]
	linked := cfg.GetLinkedPackages(pkgA)

	if len(linked) != 2 {
		t.Errorf("expected 2 linked packages, got %d", len(linked))
	}

	// Should include both service-a and service-b, sorted by path
	if linked[0].Component != "service-a" {
		t.Errorf("expected first linked package to be service-a, got %s", linked[0].Component)
	}
	if linked[1].Component != "service-b" {
		t.Errorf("expected second linked package to be service-b, got %s", linked[1].Component)
	}
}

func TestGetLinkedPackages_NotLinked(t *testing.T) {
	configJSON := `{
		"packages": {
			"workloads/service-a": {"component": "service-a"},
			"workloads/service-c": {"component": "service-c"}
		}
	}`
	manifestJSON := `{
		"workloads/service-a": "1.0.0",
		"workloads/service-c": "2.0.0"
	}`

	dir := createTestRepo(t, configJSON, manifestJSON)
	cfg, _ := Load(dir)

	pkgC := cfg.Packages["workloads/service-c"]
	linked := cfg.GetLinkedPackages(pkgC)

	if len(linked) != 1 {
		t.Errorf("expected 1 package (itself), got %d", len(linked))
	}
	if linked[0].Component != "service-c" {
		t.Errorf("expected service-c, got %s", linked[0].Component)
	}
}

func TestPackagesSortedByPath(t *testing.T) {
	configJSON := `{
		"packages": {
			"z/package": {"component": "z"},
			"a/package": {"component": "a"},
			"m/package": {"component": "m"}
		}
	}`
	manifestJSON := `{"z/package": "1.0.0", "a/package": "1.0.0", "m/package": "1.0.0"}`

	dir := createTestRepo(t, configJSON, manifestJSON)
	cfg, _ := Load(dir)

	sorted := cfg.PackagesSortedByPath()

	if len(sorted) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(sorted))
	}

	expected := []string{"a/package", "m/package", "z/package"}
	for i, pkg := range sorted {
		if pkg.Path != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], pkg.Path)
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"./workloads/jarvis", "workloads/jarvis"},
		{"workloads/jarvis/", "workloads/jarvis"},
		{"/workloads/jarvis", "workloads/jarvis"},
		{"./workloads/jarvis/", "workloads/jarvis"},
		{"workloads/jarvis", "workloads/jarvis"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizePath(tc.input)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}
