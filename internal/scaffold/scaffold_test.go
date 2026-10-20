package scaffold

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/config"
)

func newRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func assertGolden(t *testing.T, dir, path, golden string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", golden))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s =\n%s\nwant (testdata/%s)\n%s", path, got, golden, want)
	}
}

func TestRun_PlainRepoFromTags(t *testing.T) {
	dir := newRepo(t, "widget")
	gitIn(t, dir, "-c", "user.email=user@example.com", "-c", "user.name=Test", "commit", "-q", "--allow-empty", "-m", "chore: initial")
	for _, tag := range []string{"v1.9.0", "v2.5.0", "v2.10.0-rc.1", "v0.1.0", "not-a-version", "vnext"} {
		gitIn(t, dir, "tag", tag)
	}
	// v2.10.0-rc.1 is skipped: a prerelease is not the version of record.
	result, err := Run(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Component != "widget" {
		t.Errorf("component = %q, want the directory name", result.Component)
	}
	want := []string{"release-please-config.json", "release-please-manifest.json", "VERSION", "CHANGELOG.md", ".github/workflows/release.yml"}
	if !reflect.DeepEqual(result.Written, want) {
		t.Errorf("written = %v, want %v", result.Written, want)
	}
	assertGolden(t, dir, "release-please-config.json", "plain/release-please-config.json")
	assertGolden(t, dir, "release-please-manifest.json", "plain/release-please-manifest.json")
	assertGolden(t, dir, "VERSION", "plain/VERSION")
	assertGolden(t, dir, ".github/workflows/release.yml", "release.yml")

	// What init writes, Load reads back as a root package with bare tags.
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tag := cfg.Packages[""].TagName("2.6.0"); tag != "v2.6.0" {
		t.Errorf("TagName = %q", tag)
	}
}

func TestRun_NodeRepoWithExtraFiles(t *testing.T) {
	dir := newRepo(t, "ignored")
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name": "web-app", "version": "0.4.2"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var extra []config.ExtraFile
	for _, flag := range []string{"json:package.json:$.version", `json:package-lock.json:$.packages[""].version`, "generic:src/version.ts"} {
		f, err := ParseExtraFile(flag)
		if err != nil {
			t.Fatalf("ParseExtraFile(%s): %v", flag, err)
		}
		extra = append(extra, f)
	}
	result, err := Run(Options{Dir: dir, Component: "web-app", ExtraFiles: extra})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Version != "0.4.2" {
		t.Errorf("version = %q, want package.json's", result.Version)
	}
	assertGolden(t, dir, "release-please-config.json", "node/release-please-config.json")
	assertGolden(t, dir, "release-please-manifest.json", "node/release-please-manifest.json")
	if _, err := os.Stat(filepath.Join(dir, "VERSION")); !os.IsNotExist(err) {
		t.Error("VERSION written although package.json carries the version")
	}
	if cfg, err := config.Load(dir); err != nil || !reflect.DeepEqual(cfg.Packages[""].ExtraFiles, extra) {
		t.Errorf("Load: %v, extra-files %+v", err, cfg)
	}
}

func TestRun_RefusesExistingFilesWithoutForce(t *testing.T) {
	dir := newRepo(t, "tool")
	for path, content := range map[string]string{"VERSION": "9.9.9\n", "CHANGELOG.md": "# Mine\n", ".github/workflows/release.yml": "old\n"} {
		full := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte(content), 0644)
	}

	_, err := Run(Options{Dir: dir})
	if !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "VERSION, .github/workflows/release.yml") {
		t.Errorf("err = %v, want ErrExists listing VERSION and the workflow", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, config.ConfigFile)); !os.IsNotExist(statErr) {
		t.Error("files were written before refusing")
	}

	result, err := Run(Options{Dir: dir, Force: true})
	if err != nil {
		t.Fatalf("--force: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "VERSION")); string(got) != "0.0.0\n" {
		t.Errorf("VERSION = %q, want overwritten with 0.0.0", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "CHANGELOG.md")); string(got) != "# Mine\n" {
		t.Error("an existing CHANGELOG.md was overwritten")
	}
	for _, path := range result.Written {
		if path == "CHANGELOG.md" {
			t.Error("CHANGELOG.md reported as written")
		}
	}
}

func TestParseExtraFile_Rejects(t *testing.T) {
	for _, s := range []string{"package.json", "json:package.json", "json::$.version", "json:package.json:version", "generic:", "xml:pom.xml"} {
		if _, err := ParseExtraFile(s); err == nil {
			t.Errorf("ParseExtraFile(%q) accepted", s)
		}
	}
}

func TestRun_RejectsABadVersion(t *testing.T) {
	if _, err := Run(Options{Dir: newRepo(t, "x"), Version: "one"}); err == nil {
		t.Error("accepted --version one")
	}
}
