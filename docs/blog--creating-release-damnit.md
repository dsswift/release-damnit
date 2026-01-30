# Creating release-damnit

A casual narrative documenting the journey from Release Please frustration to building a replacement.

---

## The Problem

Release Please has a bug. Not a minor inconvenience, but a fundamental flaw in how it reads git history.

GitHub's API returns commits in **chronological** order, not **topological** order. When you merge a feature branch to main with `--no-ff`, Release Please only sees the merge commit tip. All the commits inside that merge? Gone. Invisible.

Here's what I mean:

```
main:    M1 ─────────────────── M2 (merge commit = HEAD)
          │                      ↑
          │ branch               │ merge (--no-ff)
          ↓                      │
feature:  ●───●───●───●─────────●┘
          A   B   C   D   (HEAD^2)
```

Release Please sees: M2 (maybe)
What actually shipped: A, B, C, D

This is [issue #1533](https://github.com/googleapis/release-please/issues/1533). It's been open. It's a known limitation. The fix would require Release Please to clone the repo and use actual git commands instead of GitHub's API.

## Failed Attempts

### Attempt 1: "Just squash merges"

The obvious workaround: squash your feature branches so each merge becomes one commit.

Problem: We have a monorepo. A feature might touch `jarvis`, `jarvis-web`, and `sandbox-portal`. Squashing makes it one commit with one scope. But Release Please uses commit scope to determine which packages to bump. One scope = one package bump. The others get missed.

### Attempt 2: "Just rebase"

Rebase feature branches onto main before merging. Then the commits are directly on main's history.

Problem: This destroys merge commit semantics. We can't see "this feature shipped" as a single event. Also, rebasing changes commit SHAs, which breaks any references to those commits (PR links, issue links, etc.).

### Attempt 3: The Cherry-Pick Workflow (ADR-009)

This is what we actually do. It's painful:

1. For each commit scope touched in the feature branch:
   - Create a `merge/<scope>` branch from main
   - Cherry-pick commits with that scope
   - Create a PR for that scope
   - Merge the PR
   - Wait for Release Please to process it

2. Repeat for every scope.

3. Run `sync-dev` workflow to merge main back to dev.

This works. It's also tedious, error-prone, and doesn't scale. Every feature that touches multiple packages becomes 3-5 PRs instead of 1.

### Attempt 4: Accept Defeat

Maybe Release Please is just not for monorepos with merge commits. Maybe we should:
- Use a different tool (none fit our config format)
- Write version bumps manually (error-prone)
- Accept that CI is a second job

No. The problem is well-defined. The solution is straightforward. Building a focused tool is faster than working around a broken one.

## The Decision

Build `release-damnit` - a drop-in replacement that:
- Uses our existing `release-please-config.json` unchanged
- Updates the same `release-please-manifest.json`
- Writes the same VERSION and CHANGELOG formats
- Creates identical GitHub releases

The only change: read git history correctly.

### Why Go?

See [ADR-001](./adr/adr-001-implementation-language.md) for the full analysis, but the short version:
- Explicit error handling (`result, err` everywhere)
- Single binary distribution
- Cross-compile with environment variables
- The DevOps ecosystem speaks Go (gh, terraform, kubectl)

Rust would be overkill. C#/.NET exceptions are implicit. Python/TypeScript need runtimes.

### Why Build Instead of Fix?

I could submit a PR to Release Please. But:
- The fix requires fundamental changes to how they read commits
- Their architecture assumes GitHub API, not git
- Waiting for PR review/merge/release could take months
- I need this working now

---

## The Build

*This section will be updated as implementation progresses.*

### Phase 1: Documentation & Foundation

Starting with documentation forces clarity. Writing ADRs for language choice and testing strategy before writing code ensures I actually think through the decisions instead of coding by instinct.

The todo.md file is the master checklist. Each item gets marked in progress, then complete. If something unexpected happens, it gets a note. Otherwise, just check the box and move on.

### Phase 2: Test Infrastructure

The testing strategy has three tiers:
1. **Unit tests** - Pure Go, mocked inputs, fast
2. **Integration tests** - Create real git repos in temp dirs
3. **E2E tests** - A dedicated GitHub repo (`mock--gitops-playground`)

**The contract assertions turned out to be valuable.** The `pkg/contracts` package provides `Require()`, `Ensure()`, and `Invariant()` functions that panic with detailed context. Unlike assertions that get compiled out, these are always active. For release infrastructure, catching bugs early matters more than microseconds of performance.

**The mock repo setup script needed iteration.** First attempt: clone existing repo, `git rm -rf .`, add new files. Problem: if the new files are identical to the old, git sees no changes. The commit fails. Second attempt: always start with `git init`, build from scratch, force-push. This creates a truly deterministic state - the tree hash is identical after every reset.

**Verification matters.** Running the setup script twice and comparing tree hashes (not commit hashes - those include timestamps) confirmed the reset is deterministic: `27dab00976668188366d17b2fce1638acd1eb1d0` every time.

### Phase 3: Core Implementation

**The git package is the heart of the tool.** Two key functions:
- `AnalyzeHead()` - Detect if HEAD is a merge commit by checking if `HEAD^2` exists
- `GetCommitsInRange(base, head)` - Get all commits between two points

The merge detection logic:
```go
// Try to get second parent (HEAD^2) - if this fails, it's not a merge commit
mergeHead, err := runGit(repoPath, "rev-parse", "HEAD^2")
if err != nil {
    // Not a merge commit - fall back to HEAD~1..HEAD
    info.IsMerge = false
    return info, nil
}
```

**Conventional commit parsing uses a single regex.** The pattern handles all the variations:
- `feat: add feature`
- `fix(auth): handle null`
- `feat!: breaking change`
- `feat(api)!: breaking API change`

```go
var conventionalCommitRegex = regexp.MustCompile(
    `^(\w+)(?:\(([^)]+)\))?(!)?\s*:\s*(.+)$`
)
```

**The config package taught me about deepest-match-wins.** With nested packages like `workloads/jarvis` and `workloads/jarvis/clients/web`, a file at `workloads/jarvis/clients/web/src/App.tsx` should match `jarvis-web`, not `jarvis`. The solution: iterate all packages, track the longest matching path prefix.

**The version package handles pre-1.0 packages specially.** By default, `feat` triggers minor bump (0.1.0 → 0.2.0). But many teams want pre-1.0 versions to stay pre-1.0 longer, treating `feat` as patch (0.1.0 → 0.1.1). The `TreatPreMajorAsMinor` flag controls this.

**Edge case: linked versions.** Some packages should always have the same version (like `ma-observe-client` and `ma-observe-server`). The config tracks these groups, and when calculating bumps, we find the highest bump across all linked packages and apply it to all of them.

### Phase 4: CLI Integration & Testing

**The CLI wiring was straightforward.** The `cmd/release-damnit/main.go` file parses flags, calls `Analyze()`, prints results, and calls `Apply()` if not in dry-run mode. The `--dry-run` flag is essential for CI debugging.

**Integration tests uncovered two edge cases:**

1. **Single-commit repos fail on `HEAD~1`.** When a repo has only one commit, `git log HEAD~1..HEAD` fails because `HEAD~1` doesn't exist. Fixed by catching the error and returning empty commits instead of failing.

2. **Linked packages with no direct commits.** When service-a and service-b are linked, and only service-a is touched, service-b still needs to bump but has no commits. The original code tried to generate a changelog with zero commits, which panicked. Fixed by skipping changelog generation for packages with no commits - they still get version bumps, just no changelog entries.

**E2E tests against the real mock repo caught real issues.** The test suite:
- Clones `mock--gitops-playground` to a temp dir
- Merges feature branches with `--no-ff`
- Runs analysis and verifies commit counts, bump types, version numbers
- Tests `Apply()` and verifies VERSION files, CHANGELOGs, and manifest are updated correctly
- Verifies dry-run makes no changes

**The test pyramid worked.** Unit tests caught logic bugs fast. Integration tests (with temp git repos) verified git traversal. E2E tests (with the real GitHub repo) verified the full flow including linked versions across multiple packages.

### Phase 5: GitHub Integration

**GitHub release creation uses the gh CLI.** Rather than implementing OAuth flows and GitHub API calls directly, we shell out to `gh release create`. The gh CLI handles authentication, retries, and all the edge cases. This is a deliberate choice - the gh CLI is already installed in most CI environments and handles auth via GITHUB_TOKEN automatically.

**Release notes are generated from commits.** The `BuildReleaseNotes()` function categorizes commits by type (Features, Bug Fixes, Performance Improvements) and formats them as markdown. Each commit includes a link to its SHA on GitHub.

```go
// Build release notes from commits
features := filterCommitsByType(rel.Commits, "feat")
fixes := filterCommitsByType(rel.Commits, "fix")
perfs := filterCommitsByType(rel.Commits, "perf")
```

**Linked packages need special handling in release notes.** When service-a and service-b are linked but only service-a was touched, service-b gets a version bump but has no commits to document. The release notes for service-b are essentially empty (just the header). This is correct - the version bump is for synchronization, not because something changed in service-b.

**E2E testing for actual release creation is tricky.** The original plan was to create real GitHub releases in the mock repo during E2E tests. But this fails because:
1. Local commits in a temp clone don't exist on the remote
2. GitHub's API rejects `--target <sha>` when the SHA doesn't exist on the remote
3. Creating real releases would pollute the mock repo with test artifacts

The solution: test release data building thoroughly (unit tests), test the full analysis-to-release-prep flow (E2E with dry-run), and trust the gh CLI for the actual API call. The gh CLI is well-tested - we don't need to verify it creates releases correctly.

**The GitHub Action wrapper was simple.** `action.yml` is a composite action that:
1. Sets up Go 1.22
2. Builds the binary from source
3. Runs with the provided flags

Building from source in CI means no binary distribution headaches (which architecture? which OS?). Go compiles fast enough that this doesn't noticeably impact CI times.

### Phase 6: sh-monorepo Integration

**Documentation first, migration second.** Before switching the production workflow, I created:
- `docs/standards/gitops.md` - The new simplified workflow documentation
- ADR-009 update - Documenting release-damnit as an alternative to the cherry-pick workflow

**The workflow change is additive.** Rather than replacing `release-please.yml`, I created `release-damnit.yml` as a parallel workflow. This allows testing the new tool without disrupting the existing release pipeline. The workflow has:
- Same path triggers as the existing workflow
- Same build jobs for each package
- Same manifest update and ArgoCD sync steps
- A `dry_run` input for safe testing

**The integration point is clean.** The release-damnit action:
1. Analyzes HEAD for merge commits
2. Updates VERSION/CHANGELOG files
3. Creates GitHub releases
4. Outputs which packages were released

The downstream build jobs check these outputs (`jarvis--release_created`, etc.) to decide whether to build.

**Real testing is still needed.** The documentation and workflow are ready, but the actual proof is merging a feature branch to main and verifying all packages are detected correctly. This requires:
1. Temporarily disabling `release-please.yml`
2. Creating a feature branch with changes to multiple packages
3. Merging to main with `--no-ff`
4. Verifying VERSION bumps are correct

This is a manual validation step that I'll do before Phase 7 (final release).

---

*Final update coming in Phase 7 after production testing...*
