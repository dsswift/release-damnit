// Package version handles semantic versioning operations.
// It provides parsing, bumping, and comparison of semver versions.
package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dsswift/release-damnit/pkg/contracts"
)

// BumpType represents the type of version bump.
type BumpType int

const (
	// None indicates no version bump is needed.
	None BumpType = iota
	// Patch indicates a patch version bump (bug fixes).
	Patch
	// Minor indicates a minor version bump (new features).
	Minor
	// Major indicates a major version bump (breaking changes).
	Major
)

// String returns the string representation of a BumpType.
func (b BumpType) String() string {
	switch b {
	case None:
		return "none"
	case Patch:
		return "patch"
	case Minor:
		return "minor"
	case Major:
		return "major"
	default:
		contracts.Unreachable("unknown BumpType: %d", b)
		return ""
	}
}

// Version represents a parsed semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string // Optional prerelease suffix (e.g., "alpha.1")
	Build      string // Optional build metadata (e.g., "build.123")
}

// semverRegex parses semantic version strings.
// Supports: 1.2.3, 1.2.3-alpha.1, 1.2.3+build.123, 1.2.3-alpha.1+build.123
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-.]+))?(?:\+([0-9A-Za-z-.]+))?$`)

// Parse parses a version string into a Version struct.
// Accepts versions with or without 'v' prefix.
func Parse(s string) (*Version, error) {
	contracts.RequireNotEmpty(s, "version string")

	matches := semverRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid semver: %s", s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
	}, nil
}

// String returns the version as a string without 'v' prefix.
func (v *Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// IsPrerelease returns true if this is a prerelease version.
func (v *Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

// IsPreMajor returns true if this is a pre-1.0.0 version.
func (v *Version) IsPreMajor() bool {
	return v.Major == 0
}

// Bump returns a new Version with the specified bump applied.
// For pre-1.0 versions with treatPreMajorAsMinor=true, minor bumps become patch.
func (v *Version) Bump(bumpType BumpType, treatPreMajorAsMinor bool) *Version {
	contracts.RequireOneOf(bumpType, []BumpType{None, Patch, Minor, Major}, "invalid bump type: %v", bumpType)

	if bumpType == None {
		return &Version{
			Major: v.Major,
			Minor: v.Minor,
			Patch: v.Patch,
		}
	}

	// For pre-1.0 versions, treat minor as patch if configured
	if v.IsPreMajor() && treatPreMajorAsMinor && bumpType == Minor {
		bumpType = Patch
	}

	var newVersion Version
	switch bumpType {
	case Patch:
		newVersion = Version{
			Major: v.Major,
			Minor: v.Minor,
			Patch: v.Patch + 1,
		}
	case Minor:
		newVersion = Version{
			Major: v.Major,
			Minor: v.Minor + 1,
			Patch: 0,
		}
	case Major:
		newVersion = Version{
			Major: v.Major + 1,
			Minor: 0,
			Patch: 0,
		}
	default:
		contracts.Unreachable("unhandled bump type: %v", bumpType)
	}

	contracts.Ensure(newVersion.Major >= 0, "major version must be non-negative")
	contracts.Ensure(newVersion.Minor >= 0, "minor version must be non-negative")
	contracts.Ensure(newVersion.Patch >= 0, "patch version must be non-negative")

	return &newVersion
}

// Compare compares two versions.
// Returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v *Version) Compare(other *Version) int {
	contracts.RequireNotNil(other, "other")

	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}

	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}

	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Prerelease versions have lower precedence
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease < other.Prerelease {
		return -1
	}
	if v.Prerelease > other.Prerelease {
		return 1
	}

	return 0
}

// CommitTypeToBump maps a conventional commit type to a version bump.
// Breaking changes should be detected separately.
func CommitTypeToBump(commitType string) BumpType {
	switch strings.ToLower(commitType) {
	case "feat":
		return Minor
	case "fix", "perf":
		return Patch
	case "chore", "docs", "style", "refactor", "test", "build", "ci":
		return None
	default:
		return None
	}
}

// MaxBump returns the higher-priority bump type.
func MaxBump(a, b BumpType) BumpType {
	if a > b {
		return a
	}
	return b
}

// ParseVersionFile reads a VERSION file and extracts the version string.
// VERSION files typically contain just a version number, optionally with a comment.
// Format: "0.1.119 # x-release-please-version" or just "0.1.119"
func ParseVersionFile(content string) (string, error) {
	contracts.RequireNotEmpty(content, "content")

	// Split on # to remove comment
	parts := strings.SplitN(content, "#", 2)
	version := strings.TrimSpace(parts[0])

	if version == "" {
		return "", fmt.Errorf("empty version in file")
	}

	// Validate it's a valid semver
	if _, err := Parse(version); err != nil {
		return "", err
	}

	return version, nil
}

// FormatVersionFile formats a version for writing to a VERSION file.
// Preserves the x-release-please-version marker if present in the original.
func FormatVersionFile(version string, originalContent string) string {
	// Check if original had the marker
	if strings.Contains(originalContent, "x-release-please-version") {
		return fmt.Sprintf("%s # x-release-please-version\n", version)
	}
	return version + "\n"
}
