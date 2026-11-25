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

func TestCheck_Work(t *testing.T) {
	tracked := RulesFor(nil)
	tracked.Work = Tracking{Kind: "APPW", Roadmap: "APP"}
	workOnly := RulesFor(nil)
	workOnly.Work = Tracking{Kind: "APPW"}

	tests := []struct {
		name    string
		rules   Rules
		message string
		// want is a substring of each problem, in order; none means the message passes.
		want []string
	}{
		{name: "off: no trailer passes", rules: RulesFor(nil), message: "feat: add a page\n"},
		{name: "off: any trailer passes", rules: RulesFor(nil), message: "feat: add a page\n\nWork: whatever\n"},
		{name: "work item", rules: tracked, message: "feat: add a page\n\nWork: APPW-15\n"},
		{name: "roadmap item", rules: tracked, message: "feat: add a page\n\nWork: APP-4\n"},
		{name: "among other trailers", rules: tracked, message: "feat: add a page\n\nBody text.\n\nWork: APPW-15\nCo-Authored-By: A User <user@example.com>\n"},
		{name: "key in any case", rules: tracked, message: "feat: add a page\n\nwork: APPW-15\n"},
		{name: "comma separated", rules: tracked, message: "feat: add a page\n\nWork: APPW-15, APP-4,APPW-16\n"},
		{name: "several lines", rules: tracked, message: "feat: add a page\n\nWork: APPW-15\nWork: APPW-16\n"},
		{name: "comments and scissors ignored", rules: tracked, message: "feat: add a page\n\nWork: APPW-15\n# Please enter the commit message\n# ------------------------ >8 ------------------------\n\nWork: nope\n"},

		{name: "missing", rules: tracked, message: "feat: add a page\n\nSome body.\n", want: []string{`no "Work:" trailer; end the message with a line like "Work: APPW-15" naming the APPW or APP item this commit is part of (vantage.yaml)`}},
		{name: "header only", rules: tracked, message: "feat: add a page\n", want: []string{`no "Work:" trailer`}},
		{name: "missing names only the work kind", rules: workOnly, message: "feat: add a page\n", want: []string{`naming the APPW item`}},
		{name: "not the last paragraph", rules: tracked, message: "feat: add a page\n\nWork: APPW-15\n\nCo-Authored-By: A User <user@example.com>\n", want: []string{"must be in the last paragraph"}},
		{name: "in the header paragraph", rules: tracked, message: "feat: add a page\nWork: APPW-15\n", want: []string{"must be in the last paragraph"}},
		{name: "another kind", rules: tracked, message: "feat: add a page\n\nWork: WEB-3\n", want: []string{`"Work: WEB-3": WEB-3 is of kind WEB; name the APPW or APP item this commit is part of`}},
		{name: "roadmap kind refused when not named", rules: workOnly, message: "feat: add a page\n\nWork: APP-4\n", want: []string{"APP-4 is of kind APP; name the APPW item"}},
		{name: "kind is case sensitive", rules: tracked, message: "feat: add a page\n\nWork: appw-15\n", want: []string{"is of kind appw"}},
		{name: "no number", rules: tracked, message: "feat: add a page\n\nWork: APPW15\n", want: []string{`"APPW15" is not an item key like "APPW-15"`}},
		{name: "empty", rules: tracked, message: "feat: add a page\n\nWork:\n", want: []string{`"" is not an item key`}},
		{name: "a url", rules: tracked, message: "feat: add a page\n\nWork: https://hub.example.com/APPW-15\n", want: []string{"is not an item key"}},
		{name: "one bad item among good", rules: tracked, message: "feat: add a page\n\nWork: APPW-15, WEB-3\n", want: []string{"WEB-3 is of kind WEB"}},
		{name: "every bad item", rules: tracked, message: "feat: add a page\n\nWork: WEB-3\nWork: APPW\n", want: []string{"WEB-3 is of kind WEB", `"APPW" is not an item key`}},
		{name: "a bad header still checks work", rules: tracked, message: "Add a page\n", want: []string{"must look like", `no "Work:" trailer`}},
		{name: "merge skipped", rules: tracked, message: "Merge branch 'x'\n"},
		{name: "release commit skipped", rules: tracked, message: "chore: release v1.2.0 [skip ci]\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Check(tc.rules, tc.message, nil).Problems
			if len(got) != len(tc.want) {
				t.Fatalf("problems = %q, want %d containing %q", got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestTracking_Trailer(t *testing.T) {
	tracked := Tracking{Kind: "APPW", Roadmap: "APP"}
	if got, err := tracked.Trailer([]string{"APPW-15", "APP-4"}); err != nil || got != "\n\nWork: APPW-15, APP-4" {
		t.Errorf("Trailer = %q, %v", got, err)
	}
	if _, err := tracked.Trailer([]string{"WEB-3"}); err == nil || !strings.Contains(err.Error(), "is of kind WEB") {
		t.Errorf("another kind: err = %v", err)
	}
	if _, err := tracked.Trailer(nil); err == nil {
		t.Error("no items gave no error")
	}
	if got, err := (Tracking{}).Trailer([]string{"WEB-3"}); err != nil || got != "\n\nWork: WEB-3" {
		t.Errorf("off: Trailer = %q, %v", got, err)
	}
	if _, err := (Tracking{}).Trailer([]string{"WEB3"}); err == nil {
		t.Error("off: a malformed item gave no error")
	}
}

func TestReadTracking(t *testing.T) {
	tests := []struct {
		name    string
		file    string // empty means no vantage.yaml
		want    Tracking
		wantErr string
	}{
		{name: "no file"},
		{name: "work and roadmap", file: "hub: https://hub.example.com\nworkspace: app\nroadmap: APP\nwork: APPW\nwatch: [src/]\n", want: Tracking{Kind: "APPW", Roadmap: "APP"}},
		{name: "comments stripped", file: "# the program's pointer\nroadmap: APPR          # the kind its roadmap items are\nwork: APPW # its work items\n", want: Tracking{Kind: "APPW", Roadmap: "APPR"}},
		{name: "quoted", file: "work: \"APPW\"\nroadmap: 'APP'\n", want: Tracking{Kind: "APPW", Roadmap: "APP"}},
		{name: "roadmap alone is off", file: "roadmap: APP\n"},
		{name: "indented keys ignored", file: "diagrams:\n  work: APPW\n  roadmap: APP\n"},
		{name: "empty work is off", file: "work:\nroadmap: APP\n"},
		{name: "digits in a kind", file: "work: APP2W\n", want: Tracking{Kind: "APP2W"}},
		{name: "not a kind", file: "work: APP W\n", wantErr: `work "APP W" must be a kind key`},
		{name: "a list is not a kind", file: "work: [APPW]\n", wantErr: "must be a kind key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				write(t, dir, VantageFile, tc.file)
			}
			got, err := ReadTracking(dir)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("ReadTracking = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
