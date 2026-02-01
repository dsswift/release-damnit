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
}

// Package represents a single package's configuration.
type Package struct {
	// Path is the relative path from repo root (e.g., "workloads/jarvis").
	Path string

	// Component is the package name used in releases (e.g., "jarvis").
	Component string

	// ChangelogPath is the relative path to changelog from package root.
	// Defaults to "CHANGELOG.md".
	ChangelogPath string

	// CurrentVersion is the current version from the manifest.
	CurrentVersion string

	// LinkedGroup is the name of the linked-versions group, if any.
	LinkedGroup string
}

// releasePleaseConfig represents the JSON structure of release-please-config.json.
type releasePleaseConfig struct {
	Packages map[string]packageConfig `json:"packages"`
	Plugins  []pluginConfig           `json:"plugins"`
}

type packageConfig struct {
	Component     string `json:"component"`
	ChangelogPath string `json:"changelog-path"`
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

	configPath := filepath.Join(absRoot, "release-please-config.json")
	manifestPath := filepath.Join(absRoot, "release-please-manifest.json")

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

	// Build config
	config := &Config{
		Packages:     make(map[string]*Package),
		LinkedGroups: make(map[string][]string),
		RepoRoot:     absRoot,
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

		pkg := &Package{
			Path:           path,
			Component:      pkgConfig.Component,
			ChangelogPath:  pkgConfig.ChangelogPath,
			CurrentVersion: manifest[path],
			LinkedGroup:    componentToGroup[pkgConfig.Component],
		}

		// Default changelog path
		if pkg.ChangelogPath == "" {
			pkg.ChangelogPath = "CHANGELOG.md"
		}

		// Validate
		if pkg.Component == "" {
			return nil, fmt.Errorf("package %s missing component name", path)
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
	var bestMatchLen int

	for path, pkg := range c.Packages {
		// Check if the file path starts with this package path
		if strings.HasPrefix(filePath, path+"/") || filePath == path {
			if len(path) > bestMatchLen {
				bestMatch = pkg
				bestMatchLen = len(path)
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

// normalizePath cleans up a path for consistent comparison.
func normalizePath(path string) string {
	// Remove leading ./
	path = strings.TrimPrefix(path, "./")
	// Remove trailing /
	path = strings.TrimSuffix(path, "/")
	// Remove leading /
	path = strings.TrimPrefix(path, "/")
	return path
}
