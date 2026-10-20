// Package scaffold writes the release setup for a single-package GitHub
// repo: the release-please config and manifest, the version file, the
// changelog, and the release workflow.
package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/changelog"
	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/version"
)

// SchemaURL is release-please's config schema.
const SchemaURL = "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json"

// WorkflowPath is where the release workflow goes.
const WorkflowPath = ".github/workflows/release.yml"

// Options configures a scaffold.
type Options struct {
	// Dir is any directory inside the repository.
	Dir string

	// Component names the package. Empty means the repo directory name.
	Component string

	// Version is the current version. Empty means DetectVersion.
	Version string

	// ExtraFiles carry the version besides (or instead of) VERSION.
	ExtraFiles []config.ExtraFile

	// Force overwrites existing files.
	Force bool

	// ActionRef is the action the workflow uses. Empty means DefaultActionRef.
	ActionRef string
}

// ErrExists is returned when a file would be overwritten without Force.
var ErrExists = errors.New("files already exist")

// Result lists what Run wrote, repo-relative, in order.
type Result struct {
	Root      string
	Component string
	Version   string
	Written   []string
}

type file struct {
	path    string
	content []byte
	// keep means an existing file is left alone, even with Force.
	keep bool
}

// Run writes the scaffold.
func Run(opts Options) (Result, error) {
	root, err := git.Toplevel(opts.Dir)
	if err != nil {
		return Result{}, err
	}
	result := Result{Root: root, Component: opts.Component, Version: opts.Version}
	if result.Component == "" {
		result.Component = filepath.Base(root)
	}
	if result.Version == "" {
		if result.Version, err = DetectVersion(root); err != nil {
			return Result{}, err
		}
	}
	if _, err := version.Parse(result.Version); err != nil {
		return Result{}, fmt.Errorf("--version: %w", err)
	}
	if opts.ActionRef == "" {
		opts.ActionRef = DefaultActionRef
	}

	configJSON, err := ConfigJSON(result.Component, opts.ExtraFiles)
	if err != nil {
		return Result{}, err
	}
	files := []file{
		{path: config.ConfigFile, content: configJSON},
		{path: config.ManifestFile, content: []byte(fmt.Sprintf("{\n  %q: %q\n}\n", config.RootManifestKey, result.Version))},
	}
	if len(opts.ExtraFiles) == 0 {
		files = append(files, file{path: "VERSION", content: []byte(result.Version + "\n")})
	}
	files = append(files,
		file{path: "CHANGELOG.md", content: []byte(changelog.InitialChangelog()), keep: true},
		file{path: WorkflowPath, content: []byte(ReleaseWorkflow(opts.ActionRef))},
	)

	var existing []string
	for _, f := range files {
		if !f.keep && exists(filepath.Join(root, f.path)) {
			existing = append(existing, f.path)
		}
	}
	if len(existing) > 0 && !opts.Force {
		return Result{}, fmt.Errorf("%w: %s; rerun with --force to overwrite them", ErrExists, strings.Join(existing, ", "))
	}

	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f.path))
		if f.keep && exists(full) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(full, f.content, 0644); err != nil {
			return Result{}, err
		}
		result.Written = append(result.Written, f.path)
	}
	return result, nil
}

type scaffoldConfig struct {
	Schema                string                     `json:"$schema"`
	IncludeComponentInTag bool                       `json:"include-component-in-tag"`
	Types                 []string                   `json:"types"`
	Scopes                []string                   `json:"scopes"`
	Packages              map[string]scaffoldPackage `json:"packages"`
}

type scaffoldPackage struct {
	Component     string             `json:"component"`
	ChangelogPath string             `json:"changelog-path"`
	ExtraFiles    []config.ExtraFile `json:"extra-files,omitempty"`
}

// ConfigJSON renders the release-please config for one root package whose
// tags are bare versions (v1.2.3).
func ConfigJSON(component string, extraFiles []config.ExtraFile) ([]byte, error) {
	cfg := scaffoldConfig{
		Schema:                SchemaURL,
		IncludeComponentInTag: false,
		Types:                 config.DefaultTypes,
		Scopes:                []string{},
		Packages: map[string]scaffoldPackage{
			config.RootManifestKey: {Component: component, ChangelogPath: "CHANGELOG.md", ExtraFiles: extraFiles},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// ParseExtraFile parses --extra-file: json:<path>:<jsonpath> or generic:<path>.
func ParseExtraFile(s string) (config.ExtraFile, error) {
	kind, rest, _ := strings.Cut(s, ":")
	switch config.ExtraFileKind(kind) {
	case config.ExtraFileGeneric:
		if rest == "" {
			return config.ExtraFile{}, fmt.Errorf("--extra-file %q: generic needs a path (generic:<path>)", s)
		}
		return config.ExtraFile{Kind: config.ExtraFileGeneric, Path: rest}, nil
	case config.ExtraFileJSON:
		path, jsonPath, ok := strings.Cut(rest, ":")
		if !ok || path == "" || !strings.HasPrefix(jsonPath, "$") {
			return config.ExtraFile{}, fmt.Errorf("--extra-file %q: use json:<path>:<jsonpath>, e.g. json:package.json:$.version", s)
		}
		return config.ExtraFile{Kind: config.ExtraFileJSON, Path: path, JSONPath: jsonPath}, nil
	}
	return config.ExtraFile{}, fmt.Errorf("--extra-file %q: use json:<path>:<jsonpath> or generic:<path>", s)
}

// DetectVersion finds the current version: the highest v* semver tag that is
// not a prerelease, else package.json's "version", else 0.0.0.
func DetectVersion(root string) (string, error) {
	tags, err := git.Output(root, "tag", "--list", "v*")
	if err != nil {
		return "", err
	}
	var best *version.Version
	for _, tag := range strings.Fields(tags) {
		v, err := version.Parse(tag)
		if err != nil || v.IsPrerelease() {
			continue
		}
		if best == nil || v.Compare(best) > 0 {
			best = v
		}
	}
	if best != nil {
		return best.String(), nil
	}

	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err == nil {
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			if _, err := version.Parse(pkg.Version); err == nil {
				return pkg.Version, nil
			}
		}
	}
	return "0.0.0", nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
