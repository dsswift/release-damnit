// Package publish turns an applied release into a version commit with its
// tags on it, pushes both, and then creates the provider releases. Tags point
// at the commit that carries the new version, never at the commit before it.
package publish

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/git"
	"github.com/dsswift/release-damnit/internal/release"
	"github.com/dsswift/release-damnit/internal/version"
)

// Prefix starts every line publish prints.
const Prefix = "release-damnit publish: "

// Options configures a publish.
type Options struct {
	// Dir is any directory inside the repository; relative Add paths are
	// resolved against it.
	Dir string

	// ReportFile is the release report the analysis wrote (--report-file).
	ReportFile string

	// Remote and Branch name where the version commit goes.
	Remote string
	Branch string

	// Attempts is how many pushes to try when the branch keeps moving.
	Attempts int

	// Add are further files a repo step changed that ride in the version commit.
	Add []string

	// RepoURL is the browsable repository URL used in release notes.
	RepoURL string

	// CreateReleases makes the provider releases once the tags are pushed.
	// result.MergeInfo.HeadSHA is the version commit. Nil creates none.
	CreateReleases func(result *release.AnalysisResult) error

	// Out receives progress lines and git's own output.
	Out io.Writer

	// SkipCI picks how the version commit's push stays out of CI: SkipCIMessage
	// (the default) puts "[skip ci]" in the message; SkipCIPushOption pushes
	// the commit with "-o ci.skip" instead. GitLab skips every pipeline for a
	// commit whose message carries the marker, tag pipelines included, so a
	// repo that builds from tag pipelines needs the push option.
	SkipCI SkipCI
}

// SkipCI is how a version commit's push avoids running CI again.
type SkipCI string

const (
	SkipCIMessage    SkipCI = "message"
	SkipCIPushOption SkipCI = "push-option"
)

// Tag is one pushed release tag.
type Tag struct {
	Name      string
	Component string
	Version   string
}

// Outcome is what a publish did.
type Outcome struct {
	// Released is false when the report had nothing to release.
	Released bool

	// Commit is the pushed version commit.
	Commit string

	// Tags are in report order.
	Tags []Tag
}

// Run publishes the report in opts.ReportFile.
func Run(opts Options) (Outcome, error) {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Branch == "" {
		opts.Branch = "main"
	}
	if opts.Attempts < 1 {
		opts.Attempts = 1
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	switch opts.SkipCI {
	case "":
		opts.SkipCI = SkipCIMessage
	case SkipCIMessage, SkipCIPushOption:
	default:
		return Outcome{}, fmt.Errorf("skip-ci %q: use %s or %s", opts.SkipCI, SkipCIMessage, SkipCIPushOption)
	}

	report, err := readReport(opts.ReportFile)
	if err != nil {
		return Outcome{}, err
	}
	p := &publisher{Options: opts}
	if len(report.Releases) == 0 {
		p.logf("the report has no releases; nothing to publish")
		return Outcome{}, nil
	}

	if p.root, err = git.Toplevel(opts.Dir); err != nil {
		return Outcome{}, err
	}
	cfg, err := config.Load(p.root)
	if err != nil {
		return Outcome{}, err
	}
	releases, err := packageReleases(cfg, report)
	if err != nil {
		return Outcome{}, err
	}

	tags := make([]Tag, len(releases))
	for i, rel := range releases {
		tags[i] = Tag{Name: rel.Package.TagName(rel.NewVersion), Component: rel.Package.Component, Version: rel.NewVersion}
		if _, err := p.git("rev-parse", "--verify", "--quiet", "refs/tags/"+tags[i].Name); err == nil {
			return Outcome{}, fmt.Errorf("tag %s already exists; %s %s was published before", tags[i].Name, tags[i].Component, tags[i].Version)
		}
	}

	files, err := p.files(report)
	if err != nil {
		return Outcome{}, err
	}
	if err := p.commit(files, commitMessage(releases, opts.SkipCI == SkipCIMessage)); err != nil {
		return Outcome{}, err
	}
	if err := p.tag(tags, false); err != nil {
		return Outcome{}, err
	}

	commit, err := p.pushCommit(tags)
	if err != nil {
		return Outcome{}, err
	}
	if err := p.pushTags(tags); err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{Released: true, Commit: commit, Tags: tags}
	if opts.CreateReleases != nil {
		result := &release.AnalysisResult{
			MergeInfo: &git.MergeInfo{HeadSHA: commit},
			Releases:  releases,
			Config:    cfg,
			RepoURL:   opts.RepoURL,
		}
		if err := opts.CreateReleases(result); err != nil {
			return outcome, fmt.Errorf("the version commit and tags are pushed, but creating the releases failed: %w", err)
		}
	}
	return outcome, nil
}

type publisher struct {
	Options
	root string
}

func (p *publisher) logf(format string, args ...any) {
	fmt.Fprintf(p.Out, Prefix+format+"\n", args...)
}

func (p *publisher) git(args ...string) (string, error) { return git.Output(p.root, args...) }

func readReport(path string) (*release.ReleaseReport, error) {
	if path == "" {
		return nil, errors.New("--report-file is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the release report: %w", err)
	}
	var report release.ReleaseReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse the release report %s: %w", path, err)
	}
	return &report, nil
}

// packageReleases rebuilds the analyzed releases from the report, with each
// package taken from the config so tag names follow its settings.
func packageReleases(cfg *config.Config, report *release.ReleaseReport) ([]*release.PackageRelease, error) {
	releases := make([]*release.PackageRelease, 0, len(report.Releases))
	for _, r := range report.Releases {
		pkg := cfg.Packages[r.Path]
		if pkg == nil || pkg.Component != r.Component {
			return nil, fmt.Errorf("the report releases %s at %q, which %s does not configure", r.Component, r.Path, config.ConfigFile)
		}
		commits := make([]*git.Commit, len(r.Commits))
		for i, c := range r.Commits {
			commits[i] = &git.Commit{SHA: c.SHA, ShortSHA: c.SHA[:min(len(c.SHA), 7)], Type: c.Type, Scope: c.Scope, Description: c.Description, IsBreaking: c.Breaking}
		}
		releases = append(releases, &release.PackageRelease{
			Package:    pkg,
			BumpType:   version.ParseBumpType(r.BumpType),
			OldVersion: r.OldVersion,
			NewVersion: r.NewVersion,
			Commits:    commits,
		})
	}
	return releases, nil
}

// files lists the repo-relative files of the version commit: what the
// release wrote, the manifest, and every --add path.
func (p *publisher) files(report *release.ReleaseReport) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	for _, r := range report.Releases {
		for _, f := range r.Files {
			add(f)
		}
	}
	add(config.ManifestFile)

	if len(p.Add) > 0 {
		// --show-prefix is Dir relative to the root, so relative --add paths
		// resolve the way the user typed them, symlinked temp dirs included.
		prefix, err := git.Output(p.Dir, "rev-parse", "--show-prefix")
		if err != nil {
			return nil, err
		}
		root, err := filepath.EvalSymlinks(p.root)
		if err != nil {
			return nil, err
		}
		for _, extra := range p.Add {
			rel := filepath.Join(filepath.FromSlash(prefix), extra)
			if filepath.IsAbs(extra) {
				resolved, err := filepath.EvalSymlinks(extra)
				if err != nil {
					return nil, fmt.Errorf("--add %s: %w", extra, err)
				}
				if rel, err = filepath.Rel(root, resolved); err != nil {
					return nil, fmt.Errorf("--add %s: %w", extra, err)
				}
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("--add %s is outside the repository", extra)
			}
			add(filepath.ToSlash(rel))
		}
	}
	return files, nil
}

// commit stages and commits exactly files, leaving anything else that is
// staged out of the version commit.
func (p *publisher) commit(files []string, message string) error {
	if _, err := p.git(append([]string{"add", "--"}, files...)...); err != nil {
		return fmt.Errorf("stage the version files: %w", err)
	}
	if _, err := p.git(append([]string{"diff", "--cached", "--quiet", "--"}, files...)...); err == nil {
		return fmt.Errorf("none of %s changed; run the release analysis without --dry-run first", strings.Join(files, ", "))
	}
	if _, err := p.git(append([]string{"commit", "--quiet", "-m", message, "--"}, files...)...); err != nil {
		return fmt.Errorf("commit the version files: %w", err)
	}
	sha, _ := p.git("rev-parse", "--short", "HEAD")
	p.logf("committed %q as %s with %s", message, sha, strings.Join(files, ", "))
	return nil
}

// commitMessage names every release. A lone release whose tag carries no
// component names only the version, matching its tag. marker appends
// "[skip ci]".
func commitMessage(releases []*release.PackageRelease, marker bool) string {
	var msg string
	if len(releases) == 1 && releases[0].Package.OmitComponentFromTag {
		msg = fmt.Sprintf("chore: release v%s", releases[0].NewVersion)
	} else {
		parts := make([]string, len(releases))
		for i, rel := range releases {
			parts[i] = fmt.Sprintf("%s v%s", rel.Package.Component, rel.NewVersion)
		}
		msg = "chore: release " + strings.Join(parts, ", ")
	}
	if marker {
		msg += " [skip ci]"
	}
	return msg
}

// tag creates an annotated tag per release at HEAD; move replaces tags that
// already exist locally (after a rebase).
func (p *publisher) tag(tags []Tag, move bool) error {
	for _, t := range tags {
		args := []string{"tag", "--annotate", "-m", fmt.Sprintf("%s v%s", t.Component, t.Version)}
		if move {
			args = append(args, "--force")
		}
		if _, err := p.git(append(args, t.Name, "HEAD")...); err != nil {
			return fmt.Errorf("tag %s: %w", t.Name, err)
		}
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	verb := "tagged"
	if move {
		verb = "moved tags"
	}
	sha, _ := p.git("rev-parse", "--short", "HEAD")
	p.logf("%s %s at %s", verb, strings.Join(names, " "), sha)
	return nil
}

// pushCommit pushes HEAD to the branch. When the branch moved meanwhile, it
// rebases the version commit onto the new tip, moves the tags, and retries.
func (p *publisher) pushCommit(tags []Tag) (string, error) {
	for attempt := 1; ; attempt++ {
		p.logf("attempt %d/%d: pushing the version commit to %s/%s", attempt, p.Attempts, p.Remote, p.Branch)
		push := []string{"push"}
		if p.SkipCI == SkipCIPushOption {
			push = append(push, "-o", "ci.skip")
		}
		if err := git.Stream(p.root, p.Out, append(push, p.Remote, "HEAD:refs/heads/"+p.Branch)...); err == nil {
			return p.git("rev-parse", "HEAD")
		}

		tracking := "refs/remotes/" + p.Remote + "/" + p.Branch
		if err := git.Stream(p.root, p.Out, "fetch", "--quiet", p.Remote, "+refs/heads/"+p.Branch+":"+tracking); err != nil {
			return "", err
		}
		if _, err := p.git("merge-base", "--is-ancestor", tracking, "HEAD"); err == nil {
			return "", fmt.Errorf("push refused and %s/%s did not move; read the hook/server output above", p.Remote, p.Branch)
		}
		if attempt >= p.Attempts {
			return "", fmt.Errorf("%s/%s kept moving; gave up after %d attempts", p.Remote, p.Branch, attempt)
		}
		tip, _ := p.git("rev-parse", "--short", tracking)
		p.logf("%s/%s moved (now %s); rebasing the version commit onto it, retrying (attempt %d/%d)", p.Remote, p.Branch, tip, attempt+1, p.Attempts)
		if err := p.rebase(tracking); err != nil {
			return "", err
		}
		if err := p.tag(tags, true); err != nil {
			return "", err
		}
	}
}

func (p *publisher) rebase(onto string) error {
	if _, err := p.git("rebase", "--quiet", onto); err != nil {
		files, _ := git.OutputNUL(p.root, "diff", "--name-only", "--diff-filter=U", "-z")
		_, _ = p.git("rebase", "--abort")
		return fmt.Errorf("the version commit conflicts with %s in %s; the rebase was aborted and nothing was pushed: %w",
			onto, strings.Join(files, ", "), err)
	}
	return nil
}

func (p *publisher) pushTags(tags []Tag) error {
	args := []string{"push", p.Remote}
	for _, t := range tags {
		args = append(args, "refs/tags/"+t.Name)
	}
	if err := git.Stream(p.root, p.Out, args...); err != nil {
		return fmt.Errorf("the version commit is pushed but its tags are not: %w", err)
	}
	p.logf("pushed %d tag(s)", len(tags))
	return nil
}

// WriteGitHubOutput appends the publish outputs to a GitHub Actions output file.
func WriteGitHubOutput(path string, outcome Outcome) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "released=%t\n", outcome.Released)
	if !outcome.Released {
		return nil
	}
	names := make([]string, len(outcome.Tags))
	for i, t := range outcome.Tags {
		names[i] = t.Name
	}
	fmt.Fprintf(f, "tag=%s\n", outcome.Tags[0].Name)
	fmt.Fprintf(f, "version=%s\n", outcome.Tags[0].Version)
	fmt.Fprintf(f, "tags=%s\n", strings.Join(names, " "))
	for _, t := range outcome.Tags {
		fmt.Fprintf(f, "%s--tag_name=%s\n", t.Component, t.Name)
		fmt.Fprintf(f, "%s--version=%s\n", t.Component, t.Version)
	}
	return nil
}
