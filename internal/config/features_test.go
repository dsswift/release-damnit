package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoad_RootPackage(t *testing.T) {
	dir := createTestRepo(t,
		`{"packages": {".": {"component": "tool"}, "sdk/go": {"component": "sdk"}}}`,
		`{".": "1.3.0", "sdk/go": "0.2.0"}`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	root := cfg.Packages[""]
	if root == nil {
		t.Fatalf("root package not keyed by the empty path: %v", cfg.Packages)
	}
	if root.CurrentVersion != "1.3.0" {
		t.Errorf("CurrentVersion = %q, want 1.3.0", root.CurrentVersion)
	}
	if root.ManifestKey != "." {
		t.Errorf("ManifestKey = %q, want .", root.ManifestKey)
	}
	if root.VersionFile() != "VERSION" || root.ChangelogFile() != "CHANGELOG.md" {
		t.Errorf("root files = %q, %q; want VERSION, CHANGELOG.md", root.VersionFile(), root.ChangelogFile())
	}

	for file, want := range map[string]string{
		"main.go":            "tool",
		"cmd/tool/main.go":   "tool",
		"sdk/go/client.go":   "sdk",
		"sdk/gopher/x.go":    "tool",
		"README.md":          "tool",
		"sdk/go/sub/deep.go": "sdk",
	} {
		if got := cfg.FindPackageForPath(file); got == nil || got.Component != want {
			t.Errorf("FindPackageForPath(%q) = %v, want %s", file, got, want)
		}
	}
}

func TestLoad_ManifestKeySpelling(t *testing.T) {
	dir := createTestRepo(t,
		`{"packages": {"apps/web": {"component": "web"}}}`,
		`{"./apps/web": "2.0.0"}`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pkg := cfg.Packages["apps/web"]
	if pkg.CurrentVersion != "2.0.0" || pkg.ManifestKey != "./apps/web" {
		t.Errorf("got version %q key %q; want 2.0.0 under ./apps/web", pkg.CurrentVersion, pkg.ManifestKey)
	}
}

func TestLoad_IncludeComponentInTag(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   map[string]string
	}{
		{
			name:   "default includes the component",
			config: `{"packages": {"a": {"component": "a"}}}`,
			want:   map[string]string{"a": "a-v1.0.0"},
		},
		{
			name:   "repo-wide false drops it",
			config: `{"include-component-in-tag": false, "packages": {".": {"component": "tool"}}}`,
			want:   map[string]string{"": "v1.0.0"},
		},
		{
			name: "package value wins over repo-wide",
			config: `{"include-component-in-tag": false, "packages": {
				"a": {"component": "a", "include-component-in-tag": true},
				"b": {"component": "b"}}}`,
			want: map[string]string{"a": "a-v1.0.0", "b": "v1.0.0"},
		},
		{
			name:   "package false with separator configured",
			config: `{"packages": {"sdk/go": {"component": "sdk/go", "tag-separator": "/", "include-component-in-tag": false}}}`,
			want:   map[string]string{"sdk/go": "v1.0.0"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(createTestRepo(t, tc.config, `{}`))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			for path, want := range tc.want {
				if got := cfg.Packages[path].TagName("1.0.0"); got != want {
					t.Errorf("%s TagName = %q, want %q", path, got, want)
				}
			}
		})
	}
}

func TestLoad_ExtraFiles(t *testing.T) {
	dir := createTestRepo(t, `{"packages": {"web": {"component": "web", "extra-files": [
		"src/version.ts",
		{"type": "generic", "path": "Chart.yaml"},
		{"type": "json", "path": "package-lock.json", "jsonpath": "$.packages[\"\"].version"}
	]}}}`, `{"web": "1.0.0"}`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []ExtraFile{
		{Kind: ExtraFileGeneric, Path: "src/version.ts"},
		{Kind: ExtraFileGeneric, Path: "Chart.yaml"},
		{Kind: ExtraFileJSON, Path: "package-lock.json", JSONPath: `$.packages[""].version`},
	}
	if got := cfg.Packages["web"].ExtraFiles; !reflect.DeepEqual(got, want) {
		t.Errorf("ExtraFiles = %+v, want %+v", got, want)
	}
	if got := cfg.Packages["web"].RepoPath("package-lock.json"); got != "web/package-lock.json" {
		t.Errorf("RepoPath = %q", got)
	}
}

func TestLoad_ExtraFilesRejectsBadEntries(t *testing.T) {
	for name, entry := range map[string]string{
		"unsupported type":      `{"type": "xml", "path": "pom.xml"}`,
		"json without path":     `{"type": "json", "path": "", "jsonpath": "$.version"}`,
		"json without jsonpath": `{"type": "json", "path": "package.json"}`,
		"missing type":          `{"path": "package.json"}`,
		"empty string":          `""`,
		"number":                `3`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := createTestRepo(t, `{"packages": {"web": {"component": "web", "extra-files": [`+entry+`]}}}`, `{}`)
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "extra-files") {
				t.Errorf("Load error = %v, want an extra-files error", err)
			}
		})
	}
}

func TestLoad_CommitRules(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := Load(createTestRepo(t, `{"packages": {}}`, `{}`))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !reflect.DeepEqual(cfg.Types, []string{"feat", "fix", "chore", "docs"}) {
			t.Errorf("Types = %v", cfg.Types)
		}
		if cfg.SubjectMaxLength != 50 || cfg.Scopes != nil {
			t.Errorf("SubjectMaxLength = %d, Scopes = %v", cfg.SubjectMaxLength, cfg.Scopes)
		}
	})

	t.Run("configured, with feat! folded into feat and unknown keys ignored", func(t *testing.T) {
		cfg, err := Load(createTestRepo(t, `{
			"scopes": ["repo", "docs", "ci", "deps"],
			"types": ["feat!", "feat", "fix", "perf"],
			"subject-max-length": 72,
			"bootstrap-sha": "abc",
			"packages": {}}`, `{}`))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !reflect.DeepEqual(cfg.Scopes, []string{"repo", "docs", "ci", "deps"}) {
			t.Errorf("Scopes = %v", cfg.Scopes)
		}
		if !reflect.DeepEqual(cfg.Types, []string{"feat", "fix", "perf"}) {
			t.Errorf("Types = %v", cfg.Types)
		}
		if cfg.SubjectMaxLength != 72 {
			t.Errorf("SubjectMaxLength = %d", cfg.SubjectMaxLength)
		}
	})

	t.Run("rejects a non-positive subject-max-length", func(t *testing.T) {
		if _, err := Load(createTestRepo(t, `{"subject-max-length": 0, "packages": {}}`, `{}`)); err == nil {
			t.Error("expected an error")
		}
	})
}

func TestNormalizePath_Root(t *testing.T) {
	for _, in := range []string{".", "./"} {
		if got := normalizePath(in); got != "" {
			t.Errorf("normalizePath(%q) = %q, want empty", in, got)
		}
	}
}
