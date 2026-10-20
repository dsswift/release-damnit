package commitcheck

import (
	"strings"
	"testing"

	"github.com/dsswift/release-damnit/internal/config"
)

func monorepo() *config.Config {
	return &config.Config{
		Packages: map[string]*config.Package{
			"apps/web": {Path: "apps/web", Component: "web"},
			"apps/api": {Path: "apps/api", Component: "api"},
			"sdk/go":   {Path: "sdk/go", Component: "sdk/go", CommitScope: "sdk"},
		},
		Scopes:           []string{"repo", "docs", "ci"},
		Types:            config.DefaultTypes,
		SubjectMaxLength: 50,
	}
}

func singlePackage() *config.Config {
	return &config.Config{
		Packages:         map[string]*config.Package{"": {Path: "", Component: "tool"}},
		Scopes:           []string{"repo"},
		Types:            config.DefaultTypes,
		SubjectMaxLength: 50,
	}
}

func TestCheck(t *testing.T) {
	mono, single, none := RulesFor(monorepo()), RulesFor(singlePackage()), RulesFor(nil)

	tests := []struct {
		name    string
		rules   Rules
		message string
		files   []string
		// want is a substring of the only problem; empty means the message passes.
		want    string
		skipped bool
	}{
		// Skipped messages.
		{name: "merge", rules: mono, message: "Merge branch 'x'", files: []string{"apps/web/a", "apps/api/b"}, skipped: true},
		{name: "revert", rules: mono, message: `Revert "feat(web): x"`, skipped: true},
		{name: "fixup", rules: mono, message: "fixup! feat(web): x", skipped: true},
		{name: "squash", rules: mono, message: "squash! feat(web): x", skipped: true},
		{name: "release commit", rules: mono, message: "chore: release web v1.2.0 [skip ci]", files: []string{"apps/web/VERSION", "apps/api/VERSION"}, skipped: true},
		{name: "release-please style release commit", rules: mono, message: "chore(release): 1.3.0", skipped: true},

		// Header rules.
		{name: "commit-scope replaces the component", rules: mono, message: "fix(sdk): retry", files: []string{"sdk/go/client.go"}},
		{name: "component rejected when commit-scope set", rules: mono, message: "fix(sdk/go): retry", files: []string{"sdk/go/client.go"}, want: `use "sdk"`},
		{name: "good scoped header", rules: mono, message: "feat(web): add a page", files: []string{"apps/web/page.tsx"}},
		{name: "comments and blank lines ignored", rules: mono, message: "\n# Please enter the commit message\nfix(api): handle nil\n\nbody\n# comment\n", files: []string{"apps/api/x.go"}},
		{name: "not conventional", rules: none, message: "Add a page", want: "must look like type(scope): description"},
		{name: "no space after colon", rules: none, message: "feat:add", want: "must look like"},
		{name: "two spaces after colon", rules: none, message: "feat:  add", want: "exactly one space"},
		{name: "unknown type", rules: none, message: "refactor: tidy", want: `type "refactor" is not allowed`},
		{name: "uppercase type", rules: none, message: "Feat: add", want: `type "Feat" is not allowed`},
		{name: "breaking feat", rules: none, message: "feat!: drop the v1 api"},
		{name: "breaking feat with scope", rules: mono, message: "feat(web)!: drop it", files: []string{"apps/web/a"}},
		{name: "breaking fix", rules: none, message: "fix!: change it", want: "allowed only on feat"},
		{name: "empty description", rules: none, message: "feat: ", want: "is empty"},
		{name: "blank description", rules: none, message: "feat:  ", want: "is empty"},
		{name: "uppercase description", rules: none, message: "feat: Add a page", want: "lowercase"},
		{name: "trailing period", rules: none, message: "feat: add a page.", want: "period"},
		{name: "fifty characters pass", rules: none, message: "feat: " + strings.Repeat("a", 50)},
		{name: "fifty-one characters fail", rules: none, message: "feat: " + strings.Repeat("a", 51), want: "51 characters; the limit is 50"},
		{name: "empty message", rules: none, message: "# only a comment\n\n", want: "empty"},
		{name: "empty parentheses", rules: none, message: "feat(): add", want: "scope is empty"},

		// No config: any scope.
		{name: "no config accepts any scope", rules: none, message: "fix(anything): repair", files: []string{"x/y"}},

		// One package touched.
		{name: "scope must match the package", rules: mono, message: "feat(api): add", files: []string{"apps/web/a"}, want: `use "web"`},
		{name: "missing scope in a monorepo", rules: mono, message: "feat: add", files: []string{"apps/web/a"}, want: `add the scope "web"`},
		{name: "repo scope on package files", rules: mono, message: "chore(repo): tidy", files: []string{"apps/web/a"}, want: `use "web"`},
		{name: "outside files ride along", rules: mono, message: "feat(web): add", files: []string{"apps/web/a", "README.md", ".github/x.yml"}},
		{name: "single package allows no scope", rules: single, message: "feat: add", files: []string{"main.go"}},
		{name: "single package allows its component", rules: single, message: "feat(tool): add", files: []string{"main.go"}},
		{name: "single package rejects another scope", rules: single, message: "feat(repo): add", files: []string{"main.go"}, want: `use "tool"`},

		// Several packages touched.
		{name: "two packages", rules: mono, message: "feat(web): add", files: []string{"apps/web/a", "apps/api/b", "README.md"}, want: "split into one commit per package"},

		// No package touched.
		{name: "repo scope", rules: mono, message: "docs(docs): explain", files: []string{"README.md"}},
		{name: "unknown repo scope", rules: mono, message: "docs(guide): explain", files: []string{"README.md"}, want: `scope "guide" is not a repo scope`},
		{name: "package scope on repo files", rules: mono, message: "docs(web): explain", files: []string{"README.md"}, want: "not a repo scope"},
		{name: "scope required in a monorepo", rules: mono, message: "docs: explain", files: []string{"README.md"}, want: "needs a repo scope; use one of: repo, docs, ci"},
		{name: "empty commit in a single repo", rules: single, message: "chore: nothing", files: nil},
		{name: "no scopes configured", rules: RulesFor(&config.Config{Packages: monorepo().Packages, Types: config.DefaultTypes, SubjectMaxLength: 50}), message: "chore: x", files: []string{"README.md"}, want: `add one to "scopes"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(tc.rules, tc.message, tc.files)
			if got.Skipped != tc.skipped {
				t.Errorf("Skipped = %v, want %v", got.Skipped, tc.skipped)
			}
			if tc.want == "" {
				if !got.OK() {
					t.Errorf("problems = %q, want none", got.Problems)
				}
				return
			}
			if len(got.Problems) != 1 || !strings.Contains(got.Problems[0], tc.want) {
				t.Errorf("problems = %q, want one containing %q", got.Problems, tc.want)
			}
		})
	}
}

func TestCheck_MultiplePackagesListsEachPackageWithItsFiles(t *testing.T) {
	got := Check(RulesFor(monorepo()), "feat(web): add", []string{"apps/web/a", "apps/api/b", "apps/api/c"})
	want := "this commit changes 2 packages; split into one commit per package:\n  api: apps/api/b, apps/api/c\n  web: apps/web/a"
	if len(got.Problems) != 1 || got.Problems[0] != want {
		t.Errorf("problems = %q, want %q", got.Problems, want)
	}
}

func TestCheck_ReportsEveryProblem(t *testing.T) {
	got := Check(RulesFor(monorepo()), "fix!: Add it.", []string{"apps/web/a"})
	if len(got.Problems) != 4 {
		t.Errorf("problems = %q, want breaking, lowercase, period and scope", got.Problems)
	}
}
