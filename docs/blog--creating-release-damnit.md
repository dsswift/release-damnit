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

**E2E testing requires real GitHub releases.** The mock repo exists precisely for this - a dedicated test environment where we can create real branches, push real commits, and create real releases. The key insight: clone the mock repo, create a timestamped test branch, merge feature branches, push to origin, then create releases against commits that actually exist on the remote.

```go
// Create unique test branch to avoid collisions
testBranch := fmt.Sprintf("test-release-%d", time.Now().UnixNano())
runCmd(t, dir, "git", "checkout", "-b", testBranch)
runCmd(t, dir, "git", "merge", "--no-ff", "origin/feature/service-a-update", "-m", "Merge feature")
// ... analyze and apply version bumps ...
runCmd(t, dir, "git", "push", "-u", "origin", testBranch)
// Now create real GitHub releases - commits exist on remote
releases, err := release.CreateGitHubReleases(result, opts)
```

**Cleanup is essential.** Tests run in a `defer` block that deletes all created releases and the test branch:

```go
defer func() {
    for _, tag := range createdTags {
        runCmdIgnoreError(dir, "gh", "release", "delete", tag, "--yes", "--cleanup-tag")
    }
    runCmdIgnoreError(dir, "git", "push", "origin", "--delete", testBranch)
}()
```

The mock repo stays clean between test runs. If anything goes wrong, the `reset-mock-repo.sh` script restores the baseline state.

**Verification uses the gh CLI.** After creating releases, we verify they exist:

```go
output, err := exec.Command("gh", "release", "view", tag, "-R", repoURL).Output()
if err != nil {
    t.Fatalf("Release %s not found on GitHub: %v", tag, err)
}
```

This confirms the full flow works end-to-end: merge detection → commit analysis → version bumping → file updates → GitHub release creation. No shortcuts, no mocking the final step.

**The GitHub Action wrapper was simple.** `action.yml` is a composite action that:
1. Sets up Go 1.22
2. Builds the binary from source
3. Runs with the provided flags

Building from source in CI means no binary distribution headaches (which architecture? which OS?). Go compiles fast enough that this doesn't noticeably impact CI times.

### Phase 5.5: Comprehensive E2E Testing

**The mock repo grew to mirror sh-monorepo.** The initial mock repo was simple - 4 services with basic linking. But to validate release-damnit for production use, it needed to test the same scenarios we'd face:

- **Nested packages**: `jarvis/clients/web` should match `jarvis-web`, not `jarvis` (deepest match wins)
- **Linked versions**: Change to `ma-observe-client` should bump `ma-observe-server` too
- **Breaking changes**: `feat!` should trigger major bumps
- **Stacked commits**: Multiple commits of varying severity (chore, fix, feat) should pick the highest (feat wins)
- **Multi-package changes**: A feature touching jarvis, sandbox-portal, and infrastructure should bump all three independently

**The setup script became a full simulation.** `scripts/setup-mock-repo.sh` now creates:
- 8 packages matching sh-monorepo structure
- 6 feature branches, each testing a specific scenario
- A `.github/workflows/release-damnit.yml` with conditional build jobs

**Six scenario tests validate behavior without touching GitHub:**

```go
TestE2E_Scenario_SingleFeat       // jarvis 0.1.0 → 0.1.1
TestE2E_Scenario_MultiPackage     // 3 packages bumped independently
TestE2E_Scenario_BreakingChange   // sandbox-portal 0.1.0 → 1.0.0
TestE2E_Scenario_LinkedVersions   // client+server both bump
TestE2E_Scenario_StackedCommits   // feat wins over fix and chore
TestE2E_Scenario_NestedPackage    // jarvis-web, not jarvis
```

**Three full GitHub tests create real releases:**

```go
TestE2E_FullGitHubReleaseFlow_SingleFeat      // Creates jarvis-v0.1.1
TestE2E_FullGitHubReleaseFlow_LinkedVersions  // Creates ma-observe-client-v0.1.1 + server
TestE2E_FullGitHubReleaseFlow_MultiPackage    // Creates 3 releases
```

Each test creates a timestamped branch, pushes to origin, creates releases, verifies via `gh release view`, and cleans up. The mock repo can be reset to baseline anytime with `setup-mock-repo.sh`.

**All 12 E2E tests pass consistently.** The test suite takes about 50 seconds - mostly network time for GitHub operations. Fast enough to run on every commit, comprehensive enough to catch real bugs.

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

### Phase 7: Release

**Testing complete, everything working.** All E2E tests pass. The tool correctly handles every scenario we threw at it:
- Merge commit traversal works as designed
- Nested package matching (deepest-match-wins) works
- Linked versions bump together
- Breaking changes trigger major bumps
- Multiple packages bump independently in a single merge

**Moved to personal repo and made public.** The tool is now available at [github.com/dsswift/release-damnit](https://github.com/dsswift/release-damnit). The repository ownership migration was straightforward - update module paths, import statements, and documentation references.

**The cherry-pick workflow is no longer necessary.** With release-damnit, the workflow is simply:
1. Develop on feature branch
2. Merge to main with `--no-ff`
3. release-damnit detects all commits in the merge
4. VERSION files, CHANGELOGs, and GitHub releases are created automatically

No more splitting features into per-scope PRs. No more manual cherry-picking. The tool does what Release Please should have done from the start.

---

## Lessons Learned

1. **Don't fight the tool, replace it.** When a tool has a fundamental architectural limitation, working around it creates more complexity than building a focused replacement.

2. **Contract assertions pay off.** The `Require()`/`Ensure()` functions caught several bugs during development. The runtime cost is negligible for a tool that runs in seconds.

3. **Test infrastructure is worth the investment.** The mock repo setup script took time to get right, but it enabled confident testing of every release scenario.

4. **Shell out to CLI tools for complex integrations.** Using `gh` for GitHub releases avoided reimplementing OAuth, retries, and API quirks.

5. **Deepest-match-wins is non-obvious but essential.** Nested package structures require explicit path matching logic, not just substring checks.
