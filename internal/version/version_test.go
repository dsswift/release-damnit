package version

import (
	"testing"
)

func TestParse_ValidVersions(t *testing.T) {
	tests := []struct {
		input string
		want  Version
	}{
		{"1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
		{"0.1.0", Version{Major: 0, Minor: 1, Patch: 0}},
		{"10.20.30", Version{Major: 10, Minor: 20, Patch: 30}},
		{"1.2.3-alpha.1", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha.1"}},
		{"1.2.3+build.123", Version{Major: 1, Minor: 2, Patch: 3, Build: "build.123"}},
		{"1.2.3-beta.2+build.456", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.2", Build: "build.456"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse(%s) failed: %v", tc.input, err)
			}
			if got.Major != tc.want.Major || got.Minor != tc.want.Minor || got.Patch != tc.want.Patch {
				t.Errorf("Parse(%s) = %d.%d.%d, want %d.%d.%d",
					tc.input, got.Major, got.Minor, got.Patch,
					tc.want.Major, tc.want.Minor, tc.want.Patch)
			}
			if got.Prerelease != tc.want.Prerelease {
				t.Errorf("Parse(%s) prerelease = %s, want %s", tc.input, got.Prerelease, tc.want.Prerelease)
			}
			if got.Build != tc.want.Build {
				t.Errorf("Parse(%s) build = %s, want %s", tc.input, got.Build, tc.want.Build)
			}
		})
	}
}

func TestParse_InvalidVersions(t *testing.T) {
	tests := []string{
		"1",
		"1.2",
		"a.b.c",
		"1.2.3.4",
		"not-a-version",
	}

	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			_, err := Parse(tc)
			if err == nil {
				t.Errorf("Parse(%s) should have failed", tc)
			}
		})
	}
}

func TestParse_EmptyString_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty string")
		}
	}()
	Parse("")
}

func TestVersion_String(t *testing.T) {
	tests := []struct {
		v    Version
		want string
	}{
		{Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{Version{Major: 0, Minor: 1, Patch: 0}, "0.1.0"},
		{Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"}, "1.2.3-alpha"},
		{Version{Major: 1, Minor: 2, Patch: 3, Build: "123"}, "1.2.3+123"},
		{Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta", Build: "456"}, "1.2.3-beta+456"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.v.String()
			if got != tc.want {
				t.Errorf("String() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestVersion_IsPreMajor(t *testing.T) {
	tests := []struct {
		v    Version
		want bool
	}{
		{Version{Major: 0, Minor: 1, Patch: 0}, true},
		{Version{Major: 0, Minor: 99, Patch: 99}, true},
		{Version{Major: 1, Minor: 0, Patch: 0}, false},
		{Version{Major: 2, Minor: 0, Patch: 0}, false},
	}

	for _, tc := range tests {
		t.Run(tc.v.String(), func(t *testing.T) {
			got := tc.v.IsPreMajor()
			if got != tc.want {
				t.Errorf("IsPreMajor() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVersion_Bump(t *testing.T) {
	tests := []struct {
		name            string
		v               Version
		bump            BumpType
		treatPreMajor   bool
		want            string
	}{
		{"patch", Version{1, 2, 3, "", ""}, Patch, false, "1.2.4"},
		{"minor", Version{1, 2, 3, "", ""}, Minor, false, "1.3.0"},
		{"major", Version{1, 2, 3, "", ""}, Major, false, "2.0.0"},
		{"none", Version{1, 2, 3, "", ""}, None, false, "1.2.3"},
		{"patch from 0", Version{0, 1, 0, "", ""}, Patch, false, "0.1.1"},
		{"minor pre-1.0 no treat", Version{0, 1, 0, "", ""}, Minor, false, "0.2.0"},
		{"minor pre-1.0 treat", Version{0, 1, 0, "", ""}, Minor, true, "0.1.1"},
		{"major pre-1.0 treat", Version{0, 1, 0, "", ""}, Major, true, "1.0.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.v.Bump(tc.bump, tc.treatPreMajor)
			if got.String() != tc.want {
				t.Errorf("Bump(%v, %v) = %s, want %s", tc.bump, tc.treatPreMajor, got.String(), tc.want)
			}
		})
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		a    string
		b    string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
	}

	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			va, _ := Parse(tc.a)
			vb, _ := Parse(tc.b)
			got := va.Compare(vb)
			if got != tc.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCommitTypeToBump(t *testing.T) {
	tests := []struct {
		commitType string
		want       BumpType
	}{
		{"feat", Minor},
		{"FEAT", Minor},
		{"fix", Patch},
		{"perf", Patch},
		{"chore", None},
		{"docs", None},
		{"style", None},
		{"refactor", None},
		{"test", None},
		{"build", None},
		{"ci", None},
		{"unknown", None},
		{"", None},
	}

	for _, tc := range tests {
		t.Run(tc.commitType, func(t *testing.T) {
			got := CommitTypeToBump(tc.commitType)
			if got != tc.want {
				t.Errorf("CommitTypeToBump(%s) = %v, want %v", tc.commitType, got, tc.want)
			}
		})
	}
}

func TestMaxBump(t *testing.T) {
	tests := []struct {
		a, b BumpType
		want BumpType
	}{
		{None, None, None},
		{None, Patch, Patch},
		{Patch, None, Patch},
		{Patch, Minor, Minor},
		{Minor, Patch, Minor},
		{Minor, Major, Major},
		{Major, Minor, Major},
	}

	for _, tc := range tests {
		t.Run(tc.a.String()+"_"+tc.b.String(), func(t *testing.T) {
			got := MaxBump(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("MaxBump(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestParseVersionFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"simple", "1.2.3", "1.2.3", false},
		{"with newline", "1.2.3\n", "1.2.3", false},
		{"with marker", "0.1.119 # x-release-please-version\n", "0.1.119", false},
		{"with whitespace", "  1.2.3  ", "1.2.3", false},
		{"invalid", "not-a-version", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseVersionFile(tc.content)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseVersionFile() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			if got != tc.want {
				t.Errorf("ParseVersionFile() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestFormatVersionFile(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		original string
		want     string
	}{
		{"no marker", "1.2.4", "1.2.3\n", "1.2.4\n"},
		{"with marker", "0.1.120", "0.1.119 # x-release-please-version\n", "0.1.120 # x-release-please-version\n"},
		{"empty original", "1.0.0", "", "1.0.0\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatVersionFile(tc.version, tc.original)
			if got != tc.want {
				t.Errorf("FormatVersionFile() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBumpType_String(t *testing.T) {
	tests := []struct {
		bump BumpType
		want string
	}{
		{None, "none"},
		{Patch, "patch"},
		{Minor, "minor"},
		{Major, "major"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := tc.bump.String()
			if got != tc.want {
				t.Errorf("String() = %s, want %s", got, tc.want)
			}
		})
	}
}
