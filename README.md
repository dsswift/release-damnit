# release-damnit

A drop-in replacement for Release Please that correctly traverses merge commits.

## Why?

Release Please has a [known bug](https://github.com/googleapis/release-please/issues/1533): GitHub's API returns commits in chronological order, not topological order. When you merge a feature branch to main with `--no-ff`, Release Please only sees the merge commit tip, missing all the commits that came through the merge.

release-damnit uses your existing Release Please configuration but reads git history directly using `git log` instead of relying on GitHub's API.

## Installation

### As a GitHub Action

```yaml
- uses: dsswift/release-damnit@v1
  with:
    token: ${{ secrets.GITHUB_TOKEN }}
```

### As a CLI

```bash
# Download binary
curl -sSL https://github.com/dsswift/release-damnit/releases/latest/download/release-damnit-linux-amd64 -o release-damnit
chmod +x release-damnit

# Or build from source
go install github.com/dsswift/release-damnit/cmd/release-damnit@latest
```

## Usage

```bash
# Dry run - see what would be bumped
release-damnit --dry-run

# Run versioning (updates VERSION files, manifests, changelogs)
release-damnit

# Also create GitHub releases
release-damnit --create-releases
```

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

## Configuration

release-damnit uses your existing Release Please configuration files unchanged:

### release-please-config.json

```json
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "packages": {
    "workloads/jarvis": {
      "component": "jarvis",
      "changelog-path": "CHANGELOG.md"
    },
    "workloads/jarvis/clients/web": {
      "component": "jarvis-web",
      "changelog-path": "CHANGELOG.md"
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

### release-please-manifest.json

```json
{
  "workloads/jarvis": "0.1.119",
  "workloads/jarvis/clients/web": "0.1.10"
}
```

This file is read to get current versions and updated with new versions.

### VERSION Files

Each package has a VERSION file:

```
0.1.119 # x-release-please-version
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
6. **Update files**: VERSION, CHANGELOG, manifest
7. **Create releases**: (optional) via GitHub API

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

Breaking changes (indicated by `!` or `BREAKING CHANGE:` footer) always trigger major bump.

For pre-1.0 packages, `feat` triggers patch instead of minor.

## GitHub Action

### Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `token` | GitHub token for creating releases | `${{ github.token }}` |
| `dry-run` | Only show what would change | `false` |
| `create-releases` | Create GitHub releases | `true` |

### Outputs

| Output | Description |
|--------|-------------|
| `releases_created` | Whether any releases were created |
| `{component}--release_created` | Whether this component was released |
| `{component}--version` | New version for this component |
| `{component}--tag_name` | Git tag name for this component |

### Example Workflow

```yaml
name: Release

on:
  push:
    branches: [main]

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

## Edge Cases

| Case | Behavior |
|------|----------|
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
