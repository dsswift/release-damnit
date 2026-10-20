// Package config handles parsing of Release Please configuration files.
// This package reads release-please-config.json and release-please-manifest.json
// to understand package structure, linked versions, and current versions.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dsswift/release-damnit/pkg/contracts"
)

// Config represents the parsed release-please-config.json and manifest.
type Config struct {
	// Packages maps path (relative to repo root) to package configuration.
	// Path is the key (e.g., "workloads/jarvis").
	Packages map[string]*Package

	// LinkedGroups maps group name to the set of component names that are linked.
	// When any component in a group is bumped, all are bumped to the same version.
	LinkedGroups map[string][]string

	// RepoRoot is the absolute path to the repository root.
	RepoRoot string

	// Scopes are the repo-level commit scopes: the scopes allowed on a commit
	// that touches no package (e.g. "repo", "docs", "ci", "deps").
	Scopes []string

	// Types are the allowed conventional commit types. A breaking marker is
	// allowed only on feat.
	Types []string

	// SubjectMaxLength is the longest commit description allowed, counted
	// after the "type(scope): " prefix.
	SubjectMaxLength int
}

// ConfigFile and ManifestFile are the release-please file names at the repo root.
const (
	ConfigFile   = "release-please-config.json"
	ManifestFile = "release-please-manifest.json"
)

// RootManifestKey is the manifest and config key of a root package.
const RootManifestKey = "."

// DefaultTypes are the commit types allowed when the config sets none.
var DefaultTypes = []string{"feat", "fix", "chore", "docs"}

// DefaultSubjectMaxLength is the description limit when the config sets none.
const DefaultSubjectMaxLength = 50

// Package represents a single package's configuration.
type Package struct {
	// Path is the relative path from repo root (e.g., "workloads/jarvis").
	// The root package (keyed "." in the config) has the empty path.
	Path string

	// ManifestKey is the key this package's version is stored under in the
	// manifest: the key as the manifest spells it, else the normalized path,
	// else "." for the root package.
	ManifestKey string

	// Component is the package name used in releases (e.g., "jarvis").
	Component string

	// ChangelogPath is the relative path to changelog from package root.
	// Defaults to "CHANGELOG.md".
	ChangelogPath string

	// TagSeparator is the string between the component and the version in
	// this package's tag: component + separator + "v" + version. Defaults to
	// "-", producing "engine-v1.2.3".
	//
	// A nested Go module needs "/" instead. The Go module proxy resolves
	// github.com/org/repo/sdk/go@vX.Y.Z only against a tag named exactly
	// "sdk/go/vX.Y.Z", so a module released under "sdk/go-v1.0.0" is not
	// fetchable at a version at all.
	TagSeparator string

	// CurrentVersion is the current version from the manifest.
	CurrentVersion string

	// LinkedGroup is the name of the linked-versions group, if any.
	LinkedGroup string

	// OmitComponentFromTag is the inverse of release-please's
	// include-component-in-tag. When true, TagName is "v" + version.
	OmitComponentFromTag bool

	// ExtraFiles are further files carrying this package's version, with
	// paths relative to the package path.
	ExtraFiles []ExtraFile
}

// releasePleaseConfig represents the JSON structure of release-please-config.json.
type releasePleaseConfig struct {
	Packages map[string]packageConfig `json:"packages"`
	Plugins  []pluginConfig           `json:"plugins"`
	// TagSeparator is the repo-wide default, applied to any package that does
	// not set its own. Absent means the conventional "-".
	TagSeparator *string `json:"tag-separator"`
	// IncludeComponentInTag is the repo-wide default; absent means true.
	IncludeComponentInTag *bool    `json:"include-component-in-tag"`
	Scopes                []string `json:"scopes"`
	Types                 []string `json:"types"`
	SubjectMaxLength      *int     `json:"subject-max-length"`
}

type packageConfig struct {
	Component     string `json:"component"`
	ChangelogPath string `json:"changelog-path"`
	// TagSeparator overrides the string between the component and the version
	// in this package's tag. Release Please supports it both at the top level
	// and per package; we honour both, with the per-package value winning.
	TagSeparator          *string     `json:"tag-separator"`
	IncludeComponentInTag *bool       `json:"include-component-in-tag"`
	ExtraFiles            []ExtraFile `json:"extra-files"`
}

type pluginConfig struct {
	Type       string   `json:"type"`
	GroupName  string   `json:"groupName"`
	Components []string `json:"components"`
}

// Load reads and parses the Release Please configuration from the given directory.
// It expects release-please-config.json and release-please-manifest.json to exist.
func Load(repoRoot string) (*Config, error) {
	contracts.RequireNotEmpty(repoRoot, "repoRoot")

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	configPath := filepath.Join(absRoot, ConfigFile)
	manifestPath := filepath.Join(absRoot, ManifestFile)

	// Read config file
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read release-please-config.json: %w", err)
	}

	var rpConfig releasePleaseConfig
	if err := json.Unmarshal(configData, &rpConfig); err != nil {
		return nil, fmt.Errorf("failed to parse release-please-config.json: %w", err)
	}

	// Read manifest file
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read release-please-manifest.json: %w", err)
	}

	var manifest map[string]string
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse release-please-manifest.json: %w", err)
	}

	// Manifest keys may be spelled "./a", "a/" or "."; index them by path.
	manifestKeys := make(map[string]string, len(manifest))
	for key := range manifest {
		manifestKeys[normalizePath(key)] = key
	}

	types, err := resolveTypes(rpConfig.Types)
	if err != nil {
		return nil, err
	}
	subjectMax := DefaultSubjectMaxLength
	if rpConfig.SubjectMaxLength != nil {
		if *rpConfig.SubjectMaxLength <= 0 {
			return nil, fmt.Errorf("subject-max-length must be positive, got %d", *rpConfig.SubjectMaxLength)
		}
		subjectMax = *rpConfig.SubjectMaxLength
	}

	// Build config
	config := &Config{
		Packages:         make(map[string]*Package),
		LinkedGroups:     make(map[string][]string),
		RepoRoot:         absRoot,
		Scopes:           rpConfig.Scopes,
		Types:            types,
		SubjectMaxLength: subjectMax,
	}

	// Build linked groups lookup (component name -> group name)
	componentToGroup := make(map[string]string)
	for _, plugin := range rpConfig.Plugins {
		if plugin.Type == "linked-versions" {
			config.LinkedGroups[plugin.GroupName] = plugin.Components
			for _, comp := range plugin.Components {
				componentToGroup[comp] = plugin.GroupName
			}
		}
	}

	// Build packages
	for path, pkgConfig := range rpConfig.Packages {
		// Normalize path (remove leading ./ or trailing /)
		path = normalizePath(path)

		manifestKey, inManifest := manifestKeys[path]
		if !inManifest {
			manifestKey = path
			if path == "" {
				manifestKey = RootManifestKey
			}
		}

		pkg := &Package{
			Path:                 path,
			ManifestKey:          manifestKey,
			Component:            pkgConfig.Component,
			ChangelogPath:        pkgConfig.ChangelogPath,
			CurrentVersion:       manifest[manifestKey],
			LinkedGroup:          componentToGroup[pkgConfig.Component],
			TagSeparator:         resolveTagSeparator(rpConfig.TagSeparator, pkgConfig.TagSeparator),
			OmitComponentFromTag: !resolveIncludeComponent(rpConfig.IncludeComponentInTag, pkgConfig.IncludeComponentInTag),
			ExtraFiles:           pkgConfig.ExtraFiles,
		}

		// Default changelog path
		if pkg.ChangelogPath == "" {
			pkg.ChangelogPath = "CHANGELOG.md"
		}

		// Validate
		if pkg.Component == "" {
			return nil, fmt.Errorf("package %s missing component name", path)
		}
		if _, dup := config.Packages[path]; dup {
			return nil, fmt.Errorf("package path %q is configured twice", path)
		}

		config.Packages[path] = pkg
	}

	contracts.Ensure(config.Packages != nil, "packages map must be initialized")
	contracts.Ensure(config.RepoRoot != "", "repo root must be set")

	return config, nil
}

// FindPackageForPath returns the package that owns a given file path.
// Uses deepest-match-wins logic for nested packages.
// Returns nil if no package matches.
func (c *Config) FindPackageForPath(filePath string) *Package {
	contracts.RequireNotEmpty(filePath, "filePath")

	// Normalize the input path
	filePath = normalizePath(filePath)

	var bestMatch *Package

	for path, pkg := range c.Packages {
		// The root package (empty path) owns every file; any other package
		// owns the files under its directory.
		if path == "" || strings.HasPrefix(filePath, path+"/") || filePath == path {
			if bestMatch == nil || len(path) > len(bestMatch.Path) {
				bestMatch = pkg
			}
		}
	}

	return bestMatch
}

// GetLinkedPackages returns all packages in the same linked group as the given package.
// Returns just the package itself if it's not in a linked group.
func (c *Config) GetLinkedPackages(pkg *Package) []*Package {
	contracts.RequireNotNil(pkg, "pkg")

	if pkg.LinkedGroup == "" {
		return []*Package{pkg}
	}

	components := c.LinkedGroups[pkg.LinkedGroup]
	var result []*Package
	for _, p := range c.Packages {
		for _, comp := range components {
			if p.Component == comp {
				result = append(result, p)
				break
			}
		}
	}

	// Sort by path for deterministic order
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result
}

// PackagesSortedByPath returns all packages sorted by path.
// Useful for deterministic output.
func (c *Config) PackagesSortedByPath() []*Package {
	var result []*Package
	for _, pkg := range c.Packages {
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}

// normalizePath cleans up a path for consistent comparison. The root path
// "." becomes the empty path.
func normalizePath(path string) string {
	if path == "." || path == "./" {
		return ""
	}
	// Remove leading ./
	path = strings.TrimPrefix(path, "./")
	// Remove trailing /
	path = strings.TrimSuffix(path, "/")
	// Remove leading /
	path = strings.TrimPrefix(path, "/")
	return path
}

// DefaultTagSeparator is the conventional separator between a component name
// and its version in a tag: "engine" + "-" + "v1.2.3".
const DefaultTagSeparator = "-"

// resolveTagSeparator picks the effective separator for one package: its own
// value if set, else the repo-wide default, else "-".
//
// Both are pointers so an explicitly-configured empty string ("component
// immediately followed by the version") is distinguishable from "not
// configured".
func resolveTagSeparator(repoWide, perPackage *string) string {
	if perPackage != nil {
		return *perPackage
	}
	if repoWide != nil {
		return *repoWide
	}
	return DefaultTagSeparator
}

// TagName returns this package's tag for a version:
// component + separator + "v" + version.
//
// A zero-valued TagSeparator means the Package was built without going through
// Load (a test fixture, or a caller constructing one by hand), so it falls back
// to the default rather than silently producing "enginev1.2.3". A config that
// genuinely wants no separator is an edge case not worth the ambiguity here.
func (p *Package) TagName(version string) string {
	if p.OmitComponentFromTag {
		return "v" + version
	}
	sep := p.TagSeparator
	if sep == "" {
		sep = DefaultTagSeparator
	}
	return p.Component + sep + "v" + version
}

// resolveIncludeComponent applies release-please's include-component-in-tag:
// the package value wins, then the repo-wide value, then true.
func resolveIncludeComponent(repoWide, perPackage *bool) bool {
	if perPackage != nil {
		return *perPackage
	}
	if repoWide != nil {
		return *repoWide
	}
	return true
}

// resolveTypes returns the allowed commit types. "feat!" means the same as
// "feat": the breaking marker is a property of feat, not a separate type.
func resolveTypes(configured []string) ([]string, error) {
	if len(configured) == 0 {
		return append([]string(nil), DefaultTypes...), nil
	}
	seen := make(map[string]bool, len(configured))
	var types []string
	for _, t := range configured {
		name := strings.TrimSuffix(t, "!")
		if name == "" {
			return nil, fmt.Errorf("types: %q is not a commit type", t)
		}
		if !seen[name] {
			seen[name] = true
			types = append(types, name)
		}
	}
	return types, nil
}

// RepoPath returns a path relative to this package as a path relative to the
// repo root, using forward slashes.
func (p *Package) RepoPath(rel string) string {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if p.Path == "" {
		return rel
	}
	return p.Path + "/" + rel
}

// VersionFile returns the repo-relative path of this package's VERSION file.
func (p *Package) VersionFile() string { return p.RepoPath("VERSION") }

// ChangelogFile returns the repo-relative path of this package's changelog.
func (p *Package) ChangelogFile() string { return p.RepoPath(p.ChangelogPath) }

// IsRoot reports whether this is the root package (config key ".").
func (p *Package) IsRoot() bool { return p.Path == "" }
