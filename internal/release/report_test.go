package release

import (
	"encoding/json"
	"testing"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/version"
)

func TestBuildReleaseReport_Empty(t *testing.T) {
	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{
			HeadSHA: "abc1234567890",
			IsMerge: false,
		},
		Commits:  nil,
		Releases: nil,
		Config: &config.Config{
			Packages:     make(map[string]*config.Package),
			LinkedGroups: make(map[string][]string),
		},
	}

	report := BuildReleaseReport(result, "https://github.com/test/repo")

	if len(report.Releases) != 0 {
		t.Errorf("expected 0 releases, got %d", len(report.Releases))
	}
	if len(report.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(report.Components))
	}
	if report.Summary.TotalReleases != 0 {
		t.Errorf("expected 0 total releases, got %d", report.Summary.TotalReleases)
	}

	// Verify it marshals to valid JSON
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	// Verify we can unmarshal it back
	var parsed ReleaseReport
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}
}

func TestBuildReleaseReport_SingleRelease(t *testing.T) {
	pkg := &config.Package{
		Path:          "workloads/service-a",
		Component:     "service-a",
		ChangelogPath: "CHANGELOG.md",
	}

	commits := []*git.Commit{
		{
			SHA:         "abc1234567890def",
			ShortSHA:    "abc1234",
			Type:        "feat",
			Scope:       "service-a",
			Description: "add new feature",
			IsBreaking:  false,
			Files:       []string{"workloads/service-a/src/main.go"},
		},
	}

	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{
			HeadSHA: "def7890123456abc",
			IsMerge: true,
		},
		Commits: commits,
		Releases: []*PackageRelease{
			{
				Package:    pkg,
				BumpType:   version.Minor,
				OldVersion: "0.1.0",
				NewVersion: "0.2.0",
				Commits:    commits,
			},
		},
		Config: &config.Config{
			Packages:     map[string]*config.Package{pkg.Path: pkg},
			LinkedGroups: make(map[string][]string),
		},
	}

	report := BuildReleaseReport(result, "https://github.com/test/repo")

	// Check releases
	if len(report.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(report.Releases))
	}

	rel := report.Releases[0]
	if rel.Component != "service-a" {
		t.Errorf("expected component service-a, got %s", rel.Component)
	}
	if rel.Path != "workloads/service-a" {
		t.Errorf("expected path workloads/service-a, got %s", rel.Path)
	}
	if rel.OldVersion != "0.1.0" {
		t.Errorf("expected old version 0.1.0, got %s", rel.OldVersion)
	}
	if rel.NewVersion != "0.2.0" {
		t.Errorf("expected new version 0.2.0, got %s", rel.NewVersion)
	}
	if rel.BumpType != "minor" {
		t.Errorf("expected bump type minor, got %s", rel.BumpType)
	}
	if rel.TagName != "service-a-v0.2.0" {
		t.Errorf("expected tag name service-a-v0.2.0, got %s", rel.TagName)
	}
	if rel.ReleaseURL != "https://github.com/test/repo/releases/tag/service-a-v0.2.0" {
		t.Errorf("unexpected release URL: %s", rel.ReleaseURL)
	}
	if rel.LinkedBump {
		t.Error("expected linked_bump to be false")
	}

	// Check commits in release
	if len(rel.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(rel.Commits))
	}
	if rel.Commits[0].SHA != "abc1234567890def" {
		t.Errorf("unexpected commit SHA: %s", rel.Commits[0].SHA)
	}
	if rel.Commits[0].Type != "feat" {
		t.Errorf("expected type feat, got %s", rel.Commits[0].Type)
	}
	if rel.Commits[0].Scope != "service-a" {
		t.Errorf("expected scope service-a, got %s", rel.Commits[0].Scope)
	}

	// Check components array
	if len(report.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(report.Components))
	}
	if report.Components[0] != "service-a" {
		t.Errorf("expected component service-a, got %s", report.Components[0])
	}

	// Check summary
	if report.Summary.TotalReleases != 1 {
		t.Errorf("expected 1 total release, got %d", report.Summary.TotalReleases)
	}
	if report.Summary.TotalCommits != 1 {
		t.Errorf("expected 1 total commit, got %d", report.Summary.TotalCommits)
	}
	if report.Summary.ByBumpType.Minor != 1 {
		t.Errorf("expected 1 minor bump, got %d", report.Summary.ByBumpType.Minor)
	}
}

func TestBuildReleaseReport_LinkedBump(t *testing.T) {
	pkgA := &config.Package{
		Path:        "workloads/service-a",
		Component:   "service-a",
		LinkedGroup: "services",
	}
	pkgB := &config.Package{
		Path:        "workloads/service-b",
		Component:   "service-b",
		LinkedGroup: "services",
	}

	// Only service-a has commits
	commits := []*git.Commit{
		{
			SHA:         "abc1234567890def",
			ShortSHA:    "abc1234",
			Type:        "feat",
			Description: "add feature",
			Files:       []string{"workloads/service-a/src/main.go"},
		},
	}

	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{
			HeadSHA: "def7890123456abc",
			IsMerge: false,
		},
		Commits: commits,
		Releases: []*PackageRelease{
			{
				Package:    pkgA,
				BumpType:   version.Minor,
				OldVersion: "1.0.0",
				NewVersion: "1.1.0",
				Commits:    commits, // Has commits
			},
			{
				Package:    pkgB,
				BumpType:   version.Minor,
				OldVersion: "1.0.0",
				NewVersion: "1.1.0",
				Commits:    nil, // No commits - linked bump
			},
		},
		Config: &config.Config{
			Packages: map[string]*config.Package{
				pkgA.Path: pkgA,
				pkgB.Path: pkgB,
			},
			LinkedGroups: map[string][]string{
				"services": {"service-a", "service-b"},
			},
		},
	}

	report := BuildReleaseReport(result, "")

	if len(report.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(report.Releases))
	}

	// Find each release
	var relA, relB *ComponentRelease
	for i := range report.Releases {
		switch report.Releases[i].Component {
		case "service-a":
			relA = &report.Releases[i]
		case "service-b":
			relB = &report.Releases[i]
		}
	}

	if relA == nil || relB == nil {
		t.Fatal("expected releases for both service-a and service-b")
	}

	// service-a has commits, so not a linked bump
	if relA.LinkedBump {
		t.Error("service-a should not be marked as linked bump")
	}

	// service-b has no commits but is in a linked group, so it's a linked bump
	if !relB.LinkedBump {
		t.Error("service-b should be marked as linked bump")
	}

	// Both should be in components array
	if len(report.Components) != 2 {
		t.Errorf("expected 2 components, got %d", len(report.Components))
	}
}

func TestBuildReleaseReport_AllBumpTypes(t *testing.T) {
	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{HeadSHA: "abc123"},
		Commits:   []*git.Commit{{SHA: "a"}, {SHA: "b"}, {SHA: "c"}},
		Releases: []*PackageRelease{
			{
				Package:    &config.Package{Path: "a", Component: "a"},
				BumpType:   version.Major,
				OldVersion: "1.0.0",
				NewVersion: "2.0.0",
				Commits:    []*git.Commit{{SHA: "a"}},
			},
			{
				Package:    &config.Package{Path: "b", Component: "b"},
				BumpType:   version.Minor,
				OldVersion: "1.0.0",
				NewVersion: "1.1.0",
				Commits:    []*git.Commit{{SHA: "b"}},
			},
			{
				Package:    &config.Package{Path: "c", Component: "c"},
				BumpType:   version.Patch,
				OldVersion: "1.0.0",
				NewVersion: "1.0.1",
				Commits:    []*git.Commit{{SHA: "c"}},
			},
		},
		Config: &config.Config{
			Packages:     make(map[string]*config.Package),
			LinkedGroups: make(map[string][]string),
		},
	}

	report := BuildReleaseReport(result, "")

	if report.Summary.ByBumpType.Major != 1 {
		t.Errorf("expected 1 major, got %d", report.Summary.ByBumpType.Major)
	}
	if report.Summary.ByBumpType.Minor != 1 {
		t.Errorf("expected 1 minor, got %d", report.Summary.ByBumpType.Minor)
	}
	if report.Summary.ByBumpType.Patch != 1 {
		t.Errorf("expected 1 patch, got %d", report.Summary.ByBumpType.Patch)
	}
}

func TestBuildAnalysisInput_Empty(t *testing.T) {
	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{
			HeadSHA: "abc1234567890",
			IsMerge: false,
		},
		Commits:  nil,
		Releases: nil,
		Config: &config.Config{
			Packages:     make(map[string]*config.Package),
			LinkedGroups: make(map[string][]string),
		},
	}

	input := BuildAnalysisInput(result)

	if input.Git.HeadSHA != "abc1234567890" {
		t.Errorf("expected head SHA abc1234567890, got %s", input.Git.HeadSHA)
	}
	if input.Git.IsMergeCommit {
		t.Error("expected IsMergeCommit to be false")
	}
	if len(input.CommitsAnalyzed) != 0 {
		t.Errorf("expected 0 commits, got %d", len(input.CommitsAnalyzed))
	}

	// Verify JSON marshaling
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal input: %v", err)
	}
	var parsed AnalysisInput
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal input: %v", err)
	}
}

func TestBuildAnalysisInput_MergeCommit(t *testing.T) {
	result := &AnalysisResult{
		MergeInfo: &git.MergeInfo{
			HeadSHA:   "merge123456",
			IsMerge:   true,
			MergeBase: "base123456",
			MergeHead: "head123456",
		},
		Commits: []*git.Commit{
			{
				SHA:         "commit1",
				Type:        "feat",
				Scope:       "api",
				Description: "add endpoint",
				IsBreaking:  false,
				Files:       []string{"workloads/api/src/endpoint.go"},
			},
			{
				SHA:         "commit2",
				Type:        "fix",
				Description: "fix bug",
				IsBreaking:  false,
				Files:       []string{"workloads/api/src/handler.go", "workloads/web/src/app.js"},
			},
		},
		Releases: nil,
		Config: &config.Config{
			Packages: map[string]*config.Package{
				"workloads/api": {Path: "workloads/api", Component: "api"},
				"workloads/web": {Path: "workloads/web", Component: "web"},
			},
			LinkedGroups: map[string][]string{
				"frontend": {"web", "mobile"},
			},
		},
	}

	input := BuildAnalysisInput(result)

	// Check git info
	if input.Git.HeadSHA != "merge123456" {
		t.Errorf("expected head SHA merge123456, got %s", input.Git.HeadSHA)
	}
	if !input.Git.IsMergeCommit {
		t.Error("expected IsMergeCommit to be true")
	}
	if input.Git.MergeBase != "base123456" {
		t.Errorf("expected merge base base123456, got %s", input.Git.MergeBase)
	}
	if input.Git.MergeHead != "head123456" {
		t.Errorf("expected merge head head123456, got %s", input.Git.MergeHead)
	}

	// Check commits
	if len(input.CommitsAnalyzed) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(input.CommitsAnalyzed))
	}

	commit1 := input.CommitsAnalyzed[0]
	if commit1.SHA != "commit1" {
		t.Errorf("expected SHA commit1, got %s", commit1.SHA)
	}
	if commit1.Message != "feat(api): add endpoint" {
		t.Errorf("expected message 'feat(api): add endpoint', got %s", commit1.Message)
	}
	if len(commit1.PackagesMatched) != 1 || commit1.PackagesMatched[0] != "api" {
		t.Errorf("expected packages [api], got %v", commit1.PackagesMatched)
	}

	commit2 := input.CommitsAnalyzed[1]
	if commit2.Message != "fix: fix bug" {
		t.Errorf("expected message 'fix: fix bug', got %s", commit2.Message)
	}
	// Should match both api and web
	if len(commit2.PackagesMatched) != 2 {
		t.Errorf("expected 2 packages matched, got %d", len(commit2.PackagesMatched))
	}

	// Check config summary
	if len(input.Config.Packages) != 2 {
		t.Errorf("expected 2 packages in config, got %d", len(input.Config.Packages))
	}
	if input.Config.Packages["workloads/api"] != "api" {
		t.Errorf("expected api component, got %s", input.Config.Packages["workloads/api"])
	}
	if len(input.Config.LinkedGroups["frontend"]) != 2 {
		t.Errorf("expected 2 components in frontend group, got %d", len(input.Config.LinkedGroups["frontend"]))
	}
}

func TestBuildCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		commit   *git.Commit
		expected string
	}{
		{
			name: "feat with scope",
			commit: &git.Commit{
				Type:        "feat",
				Scope:       "api",
				Description: "add endpoint",
				IsBreaking:  false,
			},
			expected: "feat(api): add endpoint",
		},
		{
			name: "fix without scope",
			commit: &git.Commit{
				Type:        "fix",
				Description: "fix bug",
				IsBreaking:  false,
			},
			expected: "fix: fix bug",
		},
		{
			name: "breaking with scope",
			commit: &git.Commit{
				Type:        "feat",
				Scope:       "api",
				Description: "remove endpoint",
				IsBreaking:  true,
			},
			expected: "feat(api)!: remove endpoint",
		},
		{
			name: "breaking without scope",
			commit: &git.Commit{
				Type:        "refactor",
				Description: "rewrite everything",
				IsBreaking:  true,
			},
			expected: "refactor!: rewrite everything",
		},
		{
			name: "non-conventional",
			commit: &git.Commit{
				Type:        "",
				Description: "Update readme",
			},
			expected: "Update readme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCommitMessage(tt.commit)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestReleaseReport_JSONRoundtrip(t *testing.T) {
	// Create a comprehensive report
	report := &ReleaseReport{
		Releases: []ComponentRelease{
			{
				Component:  "service-a",
				Path:       "workloads/service-a",
				OldVersion: "1.0.0",
				NewVersion: "1.1.0",
				BumpType:   "minor",
				TagName:    "service-a-v1.1.0",
				ReleaseURL: "https://github.com/test/repo/releases/tag/service-a-v1.1.0",
				LinkedBump: false,
				Commits: []CommitInfo{
					{
						SHA:         "abc123",
						Type:        "feat",
						Scope:       "service-a",
						Description: "add feature",
						Breaking:    false,
					},
				},
			},
		},
		Components: []string{"service-a"},
		Summary: ReleaseSummary{
			TotalReleases: 1,
			TotalCommits:  1,
			ByBumpType: BumpTypeCounts{
				Major: 0,
				Minor: 1,
				Patch: 0,
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back
	var parsed ReleaseReport
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify fields survived roundtrip
	if len(parsed.Releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(parsed.Releases))
	}
	if parsed.Releases[0].Component != "service-a" {
		t.Errorf("component mismatch")
	}
	if parsed.Releases[0].NewVersion != "1.1.0" {
		t.Errorf("version mismatch")
	}
	if len(parsed.Releases[0].Commits) != 1 {
		t.Errorf("commits mismatch")
	}
	if len(parsed.Components) != 1 || parsed.Components[0] != "service-a" {
		t.Errorf("components array mismatch")
	}
	if parsed.Summary.TotalReleases != 1 {
		t.Errorf("summary mismatch")
	}
}

func TestAnalysisInput_JSONRoundtrip(t *testing.T) {
	input := &AnalysisInput{
		Git: GitInfo{
			HeadSHA:       "abc123",
			IsMergeCommit: true,
			MergeBase:     "base123",
			MergeHead:     "head123",
		},
		CommitsAnalyzed: []AnalyzedCommit{
			{
				SHA:             "commit1",
				Message:         "feat(api): add endpoint",
				Type:            "feat",
				Scope:           "api",
				Breaking:        false,
				FilesChanged:    []string{"src/api.go"},
				PackagesMatched: []string{"api"},
			},
		},
		Config: ConfigSummary{
			Packages: map[string]string{
				"workloads/api": "api",
			},
			LinkedGroups: map[string][]string{
				"services": {"api", "web"},
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back
	var parsed AnalysisInput
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify fields survived roundtrip
	if parsed.Git.HeadSHA != "abc123" {
		t.Errorf("git head_sha mismatch")
	}
	if !parsed.Git.IsMergeCommit {
		t.Errorf("is_merge_commit mismatch")
	}
	if len(parsed.CommitsAnalyzed) != 1 {
		t.Errorf("commits_analyzed mismatch")
	}
	if parsed.Config.Packages["workloads/api"] != "api" {
		t.Errorf("config packages mismatch")
	}
}
