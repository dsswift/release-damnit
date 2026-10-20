# release-damnit

A drop-in replacement for Release Please that correctly traverses merge commits, plus the commit tooling around it: a commit-msg check, a landing command, and a publish step that tags the commit that carries the new version.

## Why?

Release Please has a [known bug](https://github.com/googleapis/release-please/issues/1533): GitHub's API returns commits in chronological order, not topological order. When you merge a feature branch to main with `--no-ff`, Release Please only sees the merge commit tip, missing all the commits that came through the merge.

release-damnit uses your existing Release Please configuration but reads git history directly using `git log` instead of relying on GitHub's API.

## The Flow

Every repo, monorepo or single, works the same way:

1. Work on `main`. Each commit touches one package, and its scope is that package. The commit-msg hook (`release-damnit hooks`) checks this on every commit.
2. `release-damnit land` puts the commits on the remote main. It rebases onto whatever landed meanwhile, checks the commits again, and pushes.
3. On the push, CI runs the action with `publish: 'true'`. It bumps the versions, commits them, tags that commit, pushes, and creates the releases.

New single repos get all of this from `release-damnit init`.

## Installation

### As a GitHub Action

```yaml
- uses: dsswift/release-damnit@v1.4.0
  with:
    token: ${{ secrets.GITHUB_TOKEN }}
    # Release every commit since each package's last release.
    since-last-release: 'true'
```

Both `since-last-release` and `since` read history, so check out with
`fetch-depth: 0`. `since: ${{ github.event.before }}` releases only the
commits in the push instead; a commit a failed or skipped run missed is then
never released. `since-last-release` reads from each package's last release
tag, so nothing after a release is ever missed.

### As a CLI

```bash
# Download binary (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64.exe)
curl -sSL https://github.com/dsswift/release-damnit/releases/latest/download/release-damnit-linux-amd64 -o release-damnit
chmod +x release-damnit

# Or build from source
go install github.com/dsswift/release-damnit/cmd/release-damnit@latest
```

## Usage

```bash
# Dry run - see what would be bumped
release-damnit --dry-run

# Run versioning (updates VERSION files, extra-files, manifests, changelogs)
release-damnit

# Also create GitHub releases
release-damnit --create-releases

# Analyze every commit after a given one, e.g. the branch tip before a push
release-damnit --since 1a2b3c4 --dry-run

# Analyze each package from its last release tag
release-damnit --since-last-release --dry-run

# Write the release report for a later step (publish reads it)
release-damnit --since 1a2b3c4 --report-file report.json
```

With no command, release-damnit runs the release analysis above. Its flags and output are unchanged. The first argument can instead name a command:

| Command | What it does |
|---------|--------------|
| `check-commit <msgfile>` | Checks a commit message against the staged files. This is the commit-msg hook. |
| `check-commits <range>` | Checks every non-merge commit in a revision range, e.g. `origin/main..HEAD`. |
| `land [branch]` | Lands main's local commits, or a branch as one `--no-ff` merge, and pushes. |
| `hooks` | Installs the commit-msg hook. |
| `publish` | Commits the version files, tags that commit, pushes, and creates the releases. |
| `init` | Scaffolds a single GitHub repo: config, manifest, workflow, hook. |

`release-damnit <command> -h` prints a command's options.

### Output Example

```
Analyzing merge commit abc1234...
Merge range: def5678..ghi9012 (5 commits)

Package analysis:
  jarvis         → minor (feat) [2 commits]
  jarvis-web     → patch (fix)  [1 commit]
  sandbox-portal → patch (fix)  [1 commit]

Updating:
  workloads/jarvis/VERSION: 0.1.119 → 0.2.0
  workloads/jarvis/clients/web/VERSION: 0.1.10 → 0.1.11
  platforms/sandbox/portal/VERSION: 0.1.46 → 0.1.47
```

## Commit Rules

`check-commit` and `check-commits` share one set of rules. Each problem prints as one line starting with `release-damnit check-commit:`, and the exit code is 1.

The header is `type(scope)!: description`. Scope and `!` are optional.

- The type must be in `types` (default `feat`, `fix`, `chore`, `docs`).
- `!` marks a breaking change and is allowed only on feat: `feat!: drop v1` or `feat(api)!: drop v1`.
- The description is not empty, starts with a lowercase letter, has no trailing period, and is at most `subject-max-length` characters (default 50).

The scope must match the files:

| The commit's files touch | Scope |
|--------------------------|-------|
| One package | That package's `commit-scope`, else its `component`. Empty is fine when the config has only one package. |
| More than one package | Fails. Split it into one commit per package. |
| No package | One of `scopes`. Required when the config has more than one package. |

Files outside every package can ride along with one package's files. Deletions and both sides of a rename count. Merge, `Revert "`, `fixup!`, `squash!` and release commits (`chore: release ...`, `chore(release): ...`) pass without a check. A repo with no `release-please-config.json` gets only the header rules, and any scope.

```text
$ git commit -m "feat(web): add the login page"
release-damnit check-commit: this commit changes 2 packages; split into one commit per package:
                               api: apps/api/session.go
                               web: apps/web/login.tsx
```

### The Hook

```bash
release-damnit hooks
```

In a repo with a `.husky/` directory, this makes `.husky/commit-msg` run `release-damnit check-commit "$1"` and removes any commitlint line. Other lines stay. Anywhere else, it writes a `commit-msg` git hook (linked worktrees included). That hook tells you how to install release-damnit when the binary is missing. Running it again changes nothing. It refuses to replace a commit-msg hook it did not write; `--force` replaces it.

## Landing

```bash
release-damnit land            # on main: land main's local commits
release-damnit land feature-x  # land a branch as one --no-ff merge
```

Options: `--remote` (default `origin`), `--main` (default `main`), `--attempts` (default 5).

On main, land fetches, rebases the local commits onto the remote main, runs the commit rules on them, and pushes. On a branch, it builds the `--no-ff` merge in a temporary worktree, so none of your working trees is checked out or reset. Then it pushes and moves local main to the merge: a ref update when main is not checked out, a fast-forward where it is checked out and clean, and a note when that worktree has changes.

When the push is rejected because main moved on the remote (a CI version commit, someone else's landing), land starts over on the new main. When the push is refused and main did not move, land stops and points you at the hook or server output. Land never force pushes and never skips hooks. It refuses a tree with uncommitted changes to tracked files, and on a branch it refuses a local main holding commits that are neither on the remote nor copies of the branch's commits.

## Publishing

`--create-releases` creates the tags at the analyzed commit, before the version commit exists, so the tag points at code that still has the old version. `publish` fixes the order:

```bash
release-damnit --since "$BEFORE" --report-file report.json   # bump and write files
release-damnit publish --report-file report.json --add package-lock.json
```

1. It commits exactly the files the release wrote (VERSION, changelog, extra-files, manifest) plus each `--add` path, as `chore: release web v1.1.0, api v0.3.1 [skip ci]`. A single package with bare tags gets `chore: release v1.4.0 [skip ci]`.
2. It makes an annotated tag per release on that commit.
3. It pushes the commit, then the tags. When the branch moved, it rebases the version commit onto the new tip, moves the tags, and retries.
4. It creates the GitHub or GitLab releases on the pushed tags.

A report with no releases does nothing. Options: `--report-file` (required), `--remote` (`origin`), `--branch` (`main`), `--provider` (`auto`, `github`, `gitlab`), `--gitlab-token` (or `GITLAB_RELEASE_TOKEN`), `--attempts` (5), `--add <path>` (repeatable), `--skip-ci` (`message` or `push-option`).

`--skip-ci push-option` leaves `[skip ci]` out of the message and pushes the version commit with `git push -o ci.skip` instead. Use it on GitLab: there the marker skips every pipeline for that commit, so the tag pipeline that builds the release would never run.

When `GITHUB_OUTPUT` is set, publish writes `released`, `tag` and `version` (of the first release), `tags` (space-separated), and `<component>--tag_name` and `<component>--version` per release.

## Scaffolding a Repo

```bash
release-damnit init
release-damnit init --component web-app --extra-file 'json:package.json:$.version'
```

For a single repo, init writes:

- `release-please-config.json`: one root package with bare `v1.2.3` tags.
- `release-please-manifest.json`: the current version.
- `VERSION`, unless an extra file carries the version.
- `CHANGELOG.md`, when missing.
- On GitHub (`--forge github`, the default): `.github/workflows/release.yml`, which versions and publishes on push to main, then calls `.github/workflows/build.yml` with the new tag. `build.yml` is the repo's own build: a `workflow_call` workflow that takes a string input `tag` and checks out that tag.
- On GitLab (`--forge gitlab`): `.gitlab-ci.yml`, whose `release` job downloads the release-damnit binary, versions every package since its last release, and publishes with `--provider gitlab --skip-ci push-option`. The new tag starts a tag pipeline that runs `.gitlab/build.yml`, the repo's own build (`$CI_COMMIT_TAG` is the release tag). init writes a starting `.gitlab/build.yml` only when the repo has none.

Then it installs the hook.

A GitLab repo needs one masked CI/CD variable, `GITLAB_RELEASE_TOKEN`: a project access token with the `api` and `write_repository` scopes and a role allowed to push to the default branch. The job pushes the version commit and tags with it and creates the release through the API.

Options: `--component` (default: the repo directory name), `--version` (default: the highest `v*` tag that is not a prerelease, else `package.json`'s version, else 0.0.0), `--extra-file` (repeatable; `json:<path>:<jsonpath>` or `generic:<path>`), `--force` (overwrite existing files; without it init lists them and stops), `--forge` (`github` or `gitlab`).

## Configuration

release-damnit uses your existing Release Please configuration files unchanged:

### release-please-config.json

```json
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "scopes": ["repo", "docs", "ci", "deps"],
  "packages": {
    "workloads/jarvis": {
      "component": "jarvis",
      "changelog-path": "CHANGELOG.md"
    },
    "workloads/jarvis/clients/web": {
      "component": "jarvis-web",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [
        {"type": "json", "path": "package.json", "jsonpath": "$.version"}
      ]
    }
  },
  "plugins": [
    {
      "type": "linked-versions",
      "groupName": "ma-observe",
      "components": ["ma-observe-client", "ma-observe-server"]
    }
  ]
}
```

| Key | Where | Meaning |
|-----|-------|---------|
| `packages` | top | Map of path to package. A file belongs to the deepest package whose path contains it. |
| `component` | package | The package name in tags, releases, and commit scopes. |
| `commit-scope` | package | The commit scope when it should differ from `component`, e.g. component `sdk/go` committed as `sdk`. |
| `changelog-path` | package | Changelog path relative to the package (default `CHANGELOG.md`). |
| `tag-separator` | top or package | Between component and version in the tag (default `-`, so `jarvis-v1.2.3`). |
| `include-component-in-tag` | top or package | `false` makes tags bare: `v1.2.3`. Default `true`. The package value wins. |
| `extra-files` | package | More files that carry the version. See below. |
| `plugins` | top | `linked-versions` bumps a group of components together. |
| `scopes` | top | Repo-level commit scopes, for commits that touch no package. |
| `types` | top | Allowed commit types (default `["feat","fix","chore","docs"]`). `"feat!"` means the same as `"feat"`. |
| `subject-max-length` | top | Longest commit description (default 50). |

Unknown keys are ignored. Every tag, compare link and release name comes from the same tag rule, so they always match.

#### Single repo: the root package

A package keyed `"."` owns every file that no deeper package owns. Its `VERSION` and changelog live at the repo root, and the manifest keeps the `"."` key:

```json
{
  "include-component-in-tag": false,
  "packages": {
    ".": {"component": "release-damnit", "changelog-path": "CHANGELOG.md"}
  }
}
```

#### Extra files

Paths are relative to the package. Three forms, as in Release Please:

```json
"extra-files": [
  "src/version.go",
  {"type": "generic", "path": "chart/Chart.yaml"},
  {"type": "json", "path": "package-lock.json", "jsonpath": "$.packages[\"\"].version"}
]
```

- **generic** (a plain string or `"type": "generic"`): replaces the first version on every line that contains `x-release-please-version`, and on every line between `x-release-please-start-version` and `x-release-please-end`.
- **json**: replaces the string at `jsonpath`. Paths look like `$.a.b`, `$["a"].b`, `$.list[0]`, or `$.packages[""].version`.

Only the version text changes. The rest of the file keeps its exact formatting. A missing file, marker, or path fails the release and names the file and path.

#### VERSION files

A package's `VERSION` is written when it already exists or when the package has no extra-files. So a Node package whose `package.json` is the version of record does not grow a second version file.

```
0.1.119 # x-release-please-version
```

### release-please-manifest.json

```json
{
  "workloads/jarvis": "0.1.119",
  "workloads/jarvis/clients/web": "0.1.10"
}
```

This file is read to get current versions and updated with new versions.

## GitHub Action

### Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `token` | GitHub token for creating releases | `${{ github.token }}` |
| `dry-run` | Only show what would change | `false` |
| `create-releases` | Create GitHub releases (ignored when `publish` is on) | `true` |
| `repo-url` | Repository URL | auto-detected |
| `since` | Analyze every commit after this SHA, normally `github.event.before`. With `since-last-release`, only packages with no tag use it. | `''` |
| `since-last-release` | Analyze each package from its last release tag | `false` |
| `verbose` | Detailed analysis output | `false` |
| `publish` | Commit the version files, tag that commit, push, then create releases | `false` |
| `before-publish` | Shell command run after the versions are written and before the version commit, only when something is released. `RELEASE_REPORT` points at the report JSON. | `''` |
| `publish-add` | Space-separated paths `before-publish` changed that belong in the version commit | `''` |

With `publish` on, the action runs the analysis with `--report-file`, sets the git identity to `github-actions[bot]`, runs `before-publish`, then runs `release-damnit publish` with one `--add` per `publish-add` path. The job needs `permissions: contents: write`.

### Outputs

| Output | Description |
|--------|-------------|
| `releases_created` | Whether any releases were computed |
| `release_report` | JSON release report (components, versions, tags, files) |
| `analysis_input` | JSON of the data the release decision used |
| `released` | `publish` only: whether a version commit and tags were pushed |
| `tag` | `publish` only: tag of the first release |
| `version` | `publish` only: version of the first release |
| `tags` | `publish` only: every pushed tag, space-separated |
| `{component}--release_created` | Whether this component was released |
| `{component}--version` | New version for this component |
| `{component}--tag_name` | Git tag name for this component |

`release-damnit --version` in the action reports the action's ref.

### Example: publish

A trimmed copy of the workflow `release-damnit init` writes:

```yaml
name: Release

on:
  push:
    branches: [main]

permissions:
  contents: write

concurrency:
  group: release
  cancel-in-progress: false

jobs:
  version:
    if: ${{ !contains(github.event.head_commit.message, '[skip ci]') }}
    runs-on: ubuntu-latest
    outputs:
      released: ${{ steps.release.outputs.released }}
      tag: ${{ steps.release.outputs.tag }}
    steps:
      # The branch tip, not the pushed commit: a run queued behind another
      # release must see that release's version commit and tags.
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.ref_name }}
          fetch-depth: 0
      - id: release
        uses: dsswift/release-damnit@v1.4.0
        with:
          since-last-release: 'true'
          publish: 'true'
          # A Node repo refreshes its lock file in the same commit:
          # before-publish: npm install --package-lock-only
          # publish-add: package-lock.json

  build:
    needs: version
    if: needs.version.outputs.released == 'true'
    uses: ./.github/workflows/build.yml
    with:
      tag: ${{ needs.version.outputs.tag }}
    secrets: inherit
```

### Example: per-component jobs

```yaml
jobs:
  release:
    runs-on: ubuntu-latest
    outputs:
      jarvis--release_created: ${{ steps.release.outputs.jarvis--release_created }}
      jarvis--version: ${{ steps.release.outputs.jarvis--version }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Required for git history analysis

      - uses: dsswift/release-damnit@v1
        id: release
        with:
          token: ${{ secrets.GITHUB_TOKEN }}

  build-jarvis:
    needs: release
    if: needs.release.outputs.jarvis--release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building jarvis ${{ needs.release.outputs.jarvis--version }}"
```

## How It Works

When a feature branch merges to main:

```
main:    M1 ─────────────────── M2 (merge commit = HEAD)
          │                      ↑
          │ branch               │ merge (--no-ff)
          ↓                      │
feature:  ●───●───●───●─────────●┘
          A   B   C   D   (HEAD^2)
```

1. **Detect merge commit**: `git rev-parse HEAD^2` succeeds
2. **Find merge base**: `git merge-base HEAD^1 HEAD^2` = M1
3. **Get all merged commits**: `git log M1..D` = A, B, C, D
4. **For each commit**:
   - Parse conventional commit type (feat, fix, chore, etc.)
   - Get changed files: `git diff-tree --name-only -r <sha>`
   - Map files to packages (path-based, deepest match wins)
5. **Per package**: highest-priority commit type determines bump
6. **Update files**: VERSION, extra-files, CHANGELOG, manifest
7. **Create releases**: (optional) `--create-releases`, or `publish` to tag the version commit

## Bump Priority

| Commit Type | Bump | Priority |
|-------------|------|----------|
| feat | minor | 2 |
| fix | patch | 1 |
| perf | patch | 1 |
| chore | none | 0 |
| docs | none | 0 |
| style | none | 0 |
| refactor | none | 0 |
| test | none | 0 |

Breaking changes (a `!` after the type, as in `feat!:`) always trigger a major bump. A `BREAKING CHANGE:` footer is not read.

For pre-1.0 packages, `feat` triggers patch instead of minor.

## Edge Cases

| Case | Behavior |
|------|----------|
| `--since` names an ancestor of HEAD | Analyzes `since..HEAD`: every commit in the push, merges included |
| `--since` missing, all zeros, or not an ancestor (new branch, force push) | Falls back to the rules below |
| Squash merge (non-merge commit) | Falls back to `HEAD~1..HEAD` |
| No conventional commits | No bumps, "No releasable changes" |
| Linked versions | All linked packages bump together |
| Pre-1.0 packages | `feat` treated as patch |
| Multiple scopes in one merge | Each package bumped independently |

## Comparison to Release Please

| Feature | Release Please | release-damnit |
|---------|----------------|----------------|
| Config format | ✓ | ✓ (same files) |
| Merge commit support | ✗ | ✓ |
| GitHub API dependency | Required | Optional |
| Monorepo support | ✓ | ✓ |
| Linked versions | ✓ | ✓ |
| Single binary | ✗ (Node.js) | ✓ |

## License

MIT
