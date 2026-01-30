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

---

*More updates to come as the build progresses...*
