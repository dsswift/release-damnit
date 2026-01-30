# Plan: release-damnit - Drop-in Release Please Fix

## Problem

Release Please has a fundamental bug (#1533): GitHub's API returns commits in **chronological** order, not **topological** order. When you merge a feature branch to main with `--no-ff`, RP only sees the merge commit tip, missing all the commits that came in through the merge.

This forces the painful per-scope cherry-pick workflow documented in ADR-009.

## Solution

Build `release-damnit` - a **drop-in replacement** that uses your existing Release Please configuration but reads git history directly instead of relying on GitHub's broken API.

**What stays the same:**
- `release-please-config.json` - unchanged, used as-is
- `release-please-manifest.json` - unchanged, updated the same way
- VERSION file format - identical (`0.1.119 # x-release-please-version`)
- CHANGELOG format - identical (conventional-changelog style)
- GitHub Release format - identical (`jarvis-v0.1.120`)
- Action outputs - identical (`jarvis--release_created`, etc.)

**What changes:**
- Commit analysis uses `git log` instead of GitHub API
- Merge commits are traversed correctly

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

1. Detect merge commit: `git rev-parse HEAD^2` succeeds
2. Find merge base: `git merge-base HEAD^1 HEAD^2` = M1
3. Get all merged commits: `git log M1..D` = A, B, C, D
4. For each commit:
   - Parse conventional commit type (feat, fix, chore, etc.)
   - Get changed files: `git diff-tree --name-only -r <sha>`
   - Map files to packages (path-based, deepest match wins)
5. Per package: highest-priority commit type determines bump
6. Update VERSION files for affected packages only
7. Commit version bumps
8. Create GitHub releases (optional)

## Architecture Decision Records

The release-damnit project uses ADRs to document significant decisions.

**ADR-000: Use Architecture Decision Records**
- Decision to use ADRs for this project
- Format: Markdown files in `docs/adr/`
- Naming: `adr-NNN-title.md`

**ADR-001: Implementation Language**
- Evaluates Rust, Go, C#, Python, TypeScript, Bash
- Decision: Go
- Rationale documented below

**Future ADRs as needed:**
- ADR-002: Config format compatibility (release-please-config.json)
- ADR-003: Testing strategy
- ADR-004: GitHub Action distribution model

## Language Choice: Go

**Why Go:**
| Requirement | How Go Addresses It |
|-------------|---------------------|
| **Bulletproof error handling** | Every function returns `(result, error)`. Must handle explicitly. No hidden exceptions. |
| **Single binary distribution** | `go build` produces one file. No runtime, no dependencies. |
| **Cross-platform** | `GOOS=linux/darwin/windows` cross-compilation built-in |
| **Testing** | `go test` with table-driven tests, built-in coverage |
| **Contract assertions** | Easy to add `require()` functions that panic with detailed context |
| **DevOps ecosystem** | gh CLI, Terraform, kubectl are all Go. Familiar patterns. |
| **Git integration** | Shell out to git (simple) or use go-git library (more control) |

**Why not Rust:** Overkill. Rust's memory safety doesn't add value here (CLI tool, not long-running process). Learning curve would slow development and verification.

**Why not C#:** Exception-based error handling is implicit. Errors can throw anywhere and be caught (or not) anywhere else. Less predictable for "bulletproof" requirement.

## Distribution: Shared GitHub Action

Repo created at `/Users/Shared/source/spraguehouse/release-damnit`:

```
release-damnit/
├── cmd/
│   └── release-damnit/
│       └── main.go         # CLI entrypoint
├── internal/
│   ├── config/             # Config parsing (release-please-config.json)
│   ├── git/                # Git operations (merge detection, commit traversal)
│   ├── version/            # Version bumping logic
│   ├── changelog/          # Changelog generation
│   └── release/            # GitHub release creation
├── pkg/
│   └── contracts/          # Contract assertion helpers
├── docs/
│   └── adr/
│       ├── adr-000-use-adrs.md
│       ├── adr-001-implementation-language.md
│       └── ...
├── scripts/
│   ├── setup-mock-repo.sh  # Initialize test repo
│   └── reset-mock-repo.sh  # Reset to baseline
├── action.yml              # GitHub Action wrapper
├── go.mod
├── go.sum
├── Makefile                # Build targets
├── README.md               # Generic usage documentation
├── LICENSE                 # MIT
└── VERSION                 # Tool's own version
```

**README.md scope (generic, reusable):**
- What the tool does
- Installation/usage
- Config format (compatible with release-please-config.json)
- CLI options
- GitHub Action inputs/outputs
- Examples

**NOT in README.md (project-specific):**
- Git branching strategy (goes in consumer repo's docs)
- Feature→main workflow details
- Team processes

**Usage in any repo:**
```yaml
- uses: spraguehouse/release-damnit@v1
  with:
    token: ${{ secrets.GITHUB_TOKEN }}
```

**Versioning:** Semver tags (`v1.0.0`), consumers pin to major (`@v1`).

**In sh-monorepo, update:**
| File | Change |
|------|--------|
| `.github/workflows/release.yml` | Replace release-please-action with release-damnit |

No other changes needed - existing config/manifest/VERSION files work as-is.

## Config: Uses Your Existing Files

No migration. Reads your existing config directly:

| File | How It's Used |
|------|---------------|
| `release-please-config.json` | Package paths, components, changelog paths, linked-versions plugin |
| `release-please-manifest.json` | Current versions (reads and updates) |
| `*/VERSION` | Updated with new versions |
| `*/CHANGELOG.md` | Updated with conventional-changelog entries |

Same config, same manifest, same output files. Just correct merge traversal.

## CLI Interface

```bash
# Dry run - see what would be bumped
./scripts/release-damnit.sh --dry-run

# Run versioning
./scripts/release-damnit.sh

# With GitHub releases
./scripts/release-damnit.sh --create-releases
```

**Output example:**
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

## Workflow Changes

**Before (current painful flow):**
1. Cherry-pick commits by scope into `merge/<scope>` branches
2. Create separate PR per scope
3. Merge each PR individually
4. Wait for RP to process each one

**After (feature→main flow):**
1. Feature branch merges to main via PR (`--no-ff`) when approved
2. release-damnit analyzes the merge commit
3. All affected packages versioned in one commit
4. Targeted builds run for changed packages only

## Integration with Existing Pipeline

The downstream pipeline stays the same:
- VERSION file diffs trigger targeted builds (existing logic)
- Docker builds read VERSION files (existing logic)
- Manifest updates use versions (existing logic)
- ArgoCD syncs (existing logic)

Only the versioning step changes.

## Edge Cases Handled

| Case | Behavior |
|------|----------|
| Squash merge (non-merge commit) | Falls back to `HEAD~1..HEAD` |
| No conventional commits | No bumps, "No releasable changes" |
| Linked versions (ma-observe) | Both packages bump together |
| Pre-1.0 packages | `feat` treated as patch (config flag) |
| Multiple scopes in one merge | Each package bumped independently |

## Implementation Workflow

### Step 1: Establish repository documentation

Move all knowledge from this plan into permanent docs in the `release-damnit` repo:

**Create these files:**
- `docs/adr/adr-000-use-adrs.md` - Decision to use ADRs
- `docs/adr/adr-001-implementation-language.md` - Go choice with full analysis
- `docs/adr/adr-002-testing-strategy.md` - Test pyramid, contracts, mock repo
- `docs/adr/adr-003-config-compatibility.md` - release-please-config.json format
- `docs/README.md` - Overview pointing to ADRs
- `README.md` - Generic tool documentation

**Create `docs/blog--creating-release-damnit.md`:**

A casual, narrative blog post documenting the journey:

1. **The Problem** - Release Please bug #1533, merge commits don't work
2. **Failed Attempts** - Squashing, rebasing, cherry-pick workflows, the pain
3. **The Decision** - Build a custom tool in the era of code commoditization
4. **The Design** - Language choice (Go), testing strategy, config compatibility
5. **The Build** - Updated at milestones with learnings, surprises, failures
6. **The Result** - Production deployment, what worked, what didn't

Tone: Casual, first-person, honest about failures and trade-offs.

**Create `CLAUDE.md`:**
```markdown
# release-damnit

Check `./todo.md` for the implementation roadmap. Update each item's status:
- `[ ]` pending
- `[~]` in progress
- `[x]` complete

Add brief notes under items ONLY for unexpected issues or deviations.
Do not add notes for successful completions.

If interrupted with urgent work:
1. Note current progress on the active item
2. Clear in-progress marker
3. Insert new item at current position (not at end)
4. Complete inserted item first
5. Resume original sequence
```

### Step 2: Create implementation roadmap

Create `./todo.md` at repo root - the master checklist.

### Step 3: Execute iteratively

Work through todo.md, updating status as items complete. Restart/retry until validated.

---

## Implementation Roadmap (becomes `./todo.md`)

### Phase 1: Documentation & Foundation
- [ ] Create docs/adr/adr-000-use-adrs.md
- [ ] Create docs/adr/adr-001-implementation-language.md
- [ ] Create docs/adr/adr-002-testing-strategy.md
- [ ] Create docs/adr/adr-003-config-compatibility.md
- [ ] Create docs/README.md
- [ ] Create docs/blog--creating-release-damnit.md (initial draft: the problem, failed attempts)
- [ ] Create README.md
- [ ] Create CLAUDE.md
- [ ] Initialize Go module (`go mod init`)
- [ ] Create Makefile with build/test targets
- [ ] Create .gitignore

### Phase 2: Test Infrastructure
- [ ] Create pkg/contracts - assertion helpers
- [ ] Create scripts/setup-mock-repo.sh
- [ ] Create scripts/reset-mock-repo.sh
- [ ] Initialize mock--gitops-playground with baseline structure
- [ ] Verify mock repo can be reset deterministically
- [ ] **BLOG**: Update with test infrastructure decisions and any surprises

### Phase 3: Core (test-first)
- [ ] internal/config - parse release-please-config.json
- [ ] internal/config - unit tests
- [ ] internal/git - merge detection (is HEAD a merge commit?)
- [ ] internal/git - commit traversal (get all commits in merge)
- [ ] internal/git - integration tests with temp repos
- [ ] internal/version - semver parsing
- [ ] internal/version - bump logic (patch, minor, major)
- [ ] internal/version - unit tests
- [ ] internal/changelog - generate changelog entries
- [ ] internal/changelog - unit tests
- [ ] **BLOG**: Update with core implementation learnings, any tricky git edge cases

### Phase 4: CLI Integration
- [ ] cmd/release-damnit/main.go - CLI entrypoint
- [ ] Wire up config → git → version → changelog
- [ ] --dry-run flag
- [ ] Integration tests against temp repos
- [ ] E2E tests against mock--gitops-playground
- [ ] **BLOG**: Update with CLI design decisions, test results

### Phase 5: GitHub Integration
- [ ] internal/release - create GitHub releases via gh CLI
- [ ] internal/release - tests
- [ ] action.yml - composite action wrapper
- [ ] E2E test: create actual GitHub release in mock repo
- [ ] **BLOG**: Update with GitHub Action packaging experience

### Phase 6: sh-monorepo Integration
- [ ] Create sh-monorepo/docs/standards/gitops.md
- [ ] Update ADR-009
- [ ] Update .github/workflows/release.yml
- [ ] Test with real feature→main merge
- [ ] Verify targeted builds work

### Phase 7: Release
- [ ] Final testing pass
- [ ] **BLOG**: Final update - production results, lessons learned, what worked/didn't
- [ ] Tag release-damnit v1.0.0
- [ ] Document in sh-monorepo README

## Testing Strategy

### Test Pyramid

```
                    ┌─────────────────┐
                    │   E2E Tests     │  ← Real GitHub repo (mock--gitops-playground)
                    │   (few, slow)   │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │ Integration     │  ← Local git repos (temp dirs with origins)
                    │ Tests           │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │ Unit/Simulation │  ← Mocked git output, no actual git calls
                    │ Tests (many)    │
                    └─────────────────┘
```

### Test Categories

| Category | What It Tests | How |
|----------|---------------|-----|
| **Unit tests** | Individual functions (parsing, path matching, version bumping) | Pure Go, mocked inputs |
| **Simulation tests** | Logic flows with mocked git output | Inject fake git responses |
| **Contract tests** | Assertions throughout code that validate assumptions | `require()` functions that panic with context |
| **Integration tests** | Full tool against local git repos | Temp dirs with `git init`, local "origin" repos |
| **E2E tests** | Full tool against real GitHub repo | `mock--gitops-playground` repo |

### Contract Assertions

Sprinkled throughout the code to catch invalid states early:

```go
// In version/bump.go
func BumpVersion(current string, bumpType BumpType) string {
    contracts.Require(current != "", "current version cannot be empty")
    contracts.Require(semver.IsValid("v"+current), "current version must be valid semver: %s", current)
    contracts.RequireOneOf(bumpType, []BumpType{Patch, Minor, Major}, "invalid bump type: %v", bumpType)

    // ... implementation

    contracts.Ensure(semver.IsValid("v"+result), "result must be valid semver: %s", result)
    return result
}
```

Contracts are **active in dev/test**, can be compiled out for production if needed.

### Mock Repository: `mock--gitops-playground`

**Location:** `git@github.com:spraguehouse/mock--gitops-playground.git`

**Purpose:** Deterministic test environment that can be reset and recreated.

**Structure (created by setup script):**
```
mock--gitops-playground/
├── workloads/
│   ├── service-a/
│   │   ├── VERSION           # 0.1.0
│   │   ├── CHANGELOG.md
│   │   └── src/
│   ├── service-b/
│   │   ├── VERSION           # 0.1.0
│   │   ├── CHANGELOG.md
│   │   └── src/
│   └── service-c/
│       ├── VERSION           # 0.1.0
│       ├── CHANGELOG.md
│       └── src/
├── release-please-config.json
├── release-please-manifest.json
└── README.md
```

**Branches (created by setup):**
- `main` - Production baseline
- `dev` - Test environment
- `uat` - UAT environment
- `feature/service-a-update` - Sample feature branch
- `feature/service-b-fix` - Sample feature branch

**Setup Script (`scripts/setup-mock-repo.sh`):**
```bash
#!/bin/bash
# Creates deterministic mock repo structure
# Can be run to reset repo to known state

REPO_URL="git@github.com:spraguehouse/mock--gitops-playground.git"
WORK_DIR=$(mktemp -d)

# Clone or init
git clone "$REPO_URL" "$WORK_DIR" || git init "$WORK_DIR"
cd "$WORK_DIR"

# Create structure...
# Force push to reset to known state
git push --force origin main dev uat
```

**Test Scenario Scripts:**

| Script | Creates State | Tests |
|--------|---------------|-------|
| `scenario-single-feature.sh` | One feature branch merged to main | Basic merge detection |
| `scenario-multi-package.sh` | Feature touching multiple packages | Multi-package bump |
| `scenario-parallel-features.sh` | Two features merged in sequence | Independent versioning |
| `scenario-linked-versions.sh` | Change to linked package | Linked bump behavior |
| `scenario-no-releasable.sh` | Only chore commits | No bump behavior |

**Reset Script (`scripts/reset-mock-repo.sh`):**
```bash
#!/bin/bash
# Resets mock repo to baseline state
# Safe to run after any test failure

cd "$WORK_DIR"
git fetch origin
git checkout main && git reset --hard origin/main
git checkout dev && git reset --hard origin/dev
git checkout uat && git reset --hard origin/uat
```

### Local Integration Testing

For tests that don't need GitHub:

```go
func TestMergeDetection(t *testing.T) {
    // Create temp dir with git repo and local "origin"
    origin := t.TempDir()
    local := t.TempDir()

    // Init origin as bare repo
    exec.Command("git", "init", "--bare", origin).Run()

    // Clone to local
    exec.Command("git", "clone", origin, local).Run()

    // Create commits, branches, merges...
    // Run release-damnit against local repo
    // Assert results
}
```

### Test Execution

```bash
# Unit tests (fast, no git needed)
go test ./internal/... -short

# Integration tests (creates temp git repos)
go test ./internal/... -run Integration

# E2E tests (requires mock--gitops-playground access)
go test ./e2e/... -tags=e2e

# All tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### CI Pipeline

```yaml
test:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.22'

    # Unit + Integration tests
    - run: go test ./... -v -coverprofile=coverage.out

    # E2E tests (separate job with repo access)
e2e:
  runs-on: ubuntu-latest
  needs: test
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - run: |
        # Setup mock repo access
        go test ./e2e/... -tags=e2e -v
```

## Verification

1. Create feature branch from main with commits touching multiple packages
2. Merge feature branch to dev (triggers Test deployment)
3. Merge feature branch to main with `--no-ff`
4. Run `release-damnit --dry-run`
5. Verify correct packages detected with correct bump types
6. Run without dry-run, verify VERSION files updated
7. Verify pipeline builds only changed packages

## Merge Conflict Expectations

| Merge Direction | Conflict Risk | Notes |
|-----------------|---------------|-------|
| feature → dev | **Normal** | Multiple features, expected |
| feature → main | **Low** | Feature branched from main, minimal drift |
| main → dev (sync) | **Trivial** | VERSION/CHANGELOG only, take main's |

**Key insight:** Features branch from main, so feature→main conflicts are minimal. The only drift is:
- Other features that shipped since you branched
- VERSION/CHANGELOG updates from those releases

If feature was branched from dev (for dependencies), rebase onto main before merging.

The sync-dev workflow handles VERSION file merges automatically.

## Rollback Plan

Since we use the exact same config/manifest/VERSION files, rollback is trivial:
1. Revert `.github/workflows/release.yml` to use `google-github-actions/release-please-action`
2. Go back to cherry-pick workflow

No data migration, no config changes, no VERSION file format changes.

## Git Workflow Documentation

The following section will be documented in `/Users/Shared/source/spraguehouse/sh-monorepo/docs/standards/gitops.md` (not in release-damnit, which stays generic).

---

## New Git Workflow (At-Scale Model)

### Core Principle

**Features branch from main, test on dev, merge to main when approved.**

This inverts the traditional GitFlow model where dev is a staging area for main. Instead:
- `main` is the source of truth for production-ready code
- `dev` is a testing environment, not a promotion queue
- Features are independent units that ship when THEY are ready

### The Basic Flow (Single Feature)

```
TIME →

main:    M1 ───────────────────────────── M2 ────────────→
          │                                ↑
          │ branch                         │ merge (--no-ff)
          ↓                                │ when approved
feature:  ●───●───●───────────────────────●┘
                   \
                    \ merge (--no-ff)
                     ↓
dev:      ●─────────●─────────────────────────────────────→
                    │
                    ↓ CI deploys
              Test Environment
                    │
                    ↓ QA validates
```

**Step by step:**
1. Developer creates `feature/jarvis-calendar` from `main` at M1
2. Developer works on feature: `●───●───●`
3. Feature ready for testing → PR to `dev` (--no-ff merge)
4. CI deploys dev to Test environment automatically
5. QA validates the feature in Test
6. QA approves → PR from `feature/jarvis-calendar` to `main` (--no-ff merge)
7. release-damnit runs, bumps affected packages
8. Targeted builds run for changed packages
9. Production deployment

### Parallel Features (Multiple Teams)

```
TIME →

main:    M1 ────────────────────── M2 ─────────── M3 ─────→
          │                         ↑              ↑
          ├─────────────────────────┤              │
          │ feature/A (Team 1)      │              │
          │                         │              │
          ├───────────────────────────────────────-┤
          │ feature/B (Team 2)                     │
          │                                        │
dev:      ●───────●───────●────────────────────────────────→
               merge A  merge B
               ↓        ↓
             Test env validates both
```

**Key behaviors:**
- Features A and B branch from same point (M1)
- Both merge to dev for testing (can be concurrent)
- Feature A approved first → merges to main at M2
- Feature B approved later → merges to main at M3
- **No dependency between them** - they ship independently
- release-damnit runs on each merge, bumps only that feature's packages

### Why Features Branch from Main

**Problem with branching from dev:**
```
main:    M1 ────────────────────────────────────────────→
          │
dev:      ●───●───●───●───────────────────────────────────→
                   │
feature:           └───●───●───●
                       ↑
                       Contains unapproved code from dev!
```

If you branch from dev, your feature includes all unapproved work on dev. When you try to merge to main, you either:
- Bring unapproved code with you (bad)
- Have to cherry-pick/rebase to exclude it (painful)

**Solution: Branch from main:**
```
main:    M1 ────────────────────────────────────────────→
          │
          └───────●───●───●  ← Only your feature code
                   \
dev:      ●─────────●───────────────────────────────────→
                    ↑
                    Your feature merged for testing
```

Your feature contains ONLY your changes. When you merge to main, you bring ONLY your feature.

### Keeping Feature Branches Current

As main advances (other features ship), your feature branch may drift:

```
main:    M1 ───────────── M2 ───────────────────────────→
          │                │
          └───●───●        │ (M2 = another feature shipped)
                           │
                           ↓
              Need to integrate M2's changes
```

**Options:**
1. **Merge main into feature:** `git merge main` - preserves history, creates merge commit
2. **Rebase feature onto main:** `git rebase main` - linear history, rewrites commits
3. **Do nothing:** If no conflicts, merge will handle it

Recommendation: Merge main into feature periodically for long-lived branches. Your feature branch is yours - rebase if you prefer, but the merge to main is always --no-ff.

### Handling Dependencies

Sometimes features depend on each other:

```
main:    M1 ─────────────────────────────────────────────→
          │
feature/A ├───●───●───●
          │            \
feature/B │             └───●───●───●
          │                          ↑
          │                          Depends on A
```

**Scenario:** Feature B needs code from Feature A, but A isn't approved yet.

**Solutions:**

| Approach | When to use | How |
|----------|-------------|-----|
| **Wait** | A will be approved soon | Approve A first, then B |
| **Branch from A** | Tight coupling | B branches from A, ships together |
| **Feature train** | Planned bundle | Both reviewed/approved together, merge A then B |

**Branching from A:**
```
main:    M1 ───────────────────────── M2 ─── M3 ─────────→
          │                            ↑     ↑
feature/A ├───●───●───●────────────────┤     │
                       \                     │
feature/B              └───●───●────────────-┤
```

When A ships (M2), B rebases onto main to resolve, then ships (M3).

### At-Scale: Managing Many Features

With 5+ teams and 20+ features in flight:

**1. Feature naming convention:**
```
feature/{product}-{description}
feature/jarvis-calendar-sync
feature/sandbox-auth-refresh
feature/ma-observe-export
```

**2. Dev branch becomes busy:**
```
dev: ●──●──●──●──●──●──●──●──●──●──●──●──●──●──●──●──●──→
      A  B  C  D  A  E  F  A  B  G  ...
```

This is fine. Dev is a testing ground, not a promotion queue. Features come and go.

**3. Main stays clean:**
```
main: M1 ─────── M2 ─────── M3 ─────── M4 ─────── M5 ────→
       release   release   release   release   release
       A         B         C         D         E
```

Each merge to main is one approved feature. History is clear.

**4. Version management:**

| Feature | Packages affected | Version bump |
|---------|-------------------|--------------|
| jarvis-calendar | jarvis, jarvis-web | jarvis: 0.1.119→0.1.120, jarvis-web: 0.1.10→0.1.11 |
| sandbox-auth | sandbox-portal | sandbox-portal: 0.1.46→0.1.47 |
| jarvis-notifications | jarvis, jarvis-android | jarvis: 0.1.120→0.1.121, jarvis-android: 0.1.3→0.1.4 |

Each feature bumps only its packages. No coordination needed.

### Multiple Test Environments: dev and uat

Some projects need multiple pre-production environments:

| Branch | Environment | Purpose | Sync Frequency |
|--------|-------------|---------|----------------|
| `dev` | Test | Continuous testing, automated QA | After each feature ships (automated) |
| `uat` | UAT | User acceptance, stakeholder demos | Monthly or on-demand (manual) |
| `main` | Production | Live | Source of truth |

**Feature testing flow:**
```
                                    ┌──→ dev (Test env)
                                    │
feature/calendar ───●───●───●───────┼──→ uat (UAT env, if needed)
                                    │
                                    └──→ main (Production, when approved)
```

Features can merge to dev, uat, or both depending on what testing is needed:
- **dev**: Always merge here for automated testing
- **uat**: Merge here when stakeholder validation is needed
- **main**: Merge here when fully approved

### Keeping Test Environments Stable

**Problem:** Over time, dev and uat accumulate features. Some ship to production, others don't. The environments drift from production.

**Solution:** Periodic sync from main to reset the baseline.

```
main:    M1 ─── M2 ─── M3 ─── M4 ─── M5 ─── M6 ─── M7 ────→
          │           │                     │
          │           │ sync (weekly)       │ sync (weekly)
          ↓           ↓                     ↓
dev:     ●───●───●────●───●───●───●───●────●───●───●─────→
         features     reset   features      reset   features

main:    M1 ─── M2 ─── M3 ─── M4 ─── M5 ─── M6 ─── M7 ────→
          │                                 │
          │                                 │ sync (monthly)
          ↓                                 ↓
uat:     ●───●───●───●───●───●───●───●─────●───●───●─────→
         features for UAT                  reset   new features
```

### Sync Workflows

**sync-dev (automated, after each feature ships):**
```yaml
on:
  push:
    branches: [main]

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: dev
          fetch-depth: 0
      - run: |
          git merge origin/main --no-edit
          git push
```

Triggers automatically when any feature ships to main. VERSION/CHANGELOG updates flow to dev immediately.

**sync-uat (manual or scheduled):**
```yaml
on:
  workflow_dispatch:  # Manual trigger
  schedule:
    - cron: '0 0 1 * *'  # First of each month

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: uat
          fetch-depth: 0
      - run: |
          git merge origin/main --no-edit
          git push
```

Runs monthly or on-demand. Resets UAT to production baseline.

### Handling Sync Conflicts

When syncing main→dev or main→uat, conflicts may occur:

**Common conflict: Feature on dev that never shipped**
```
dev has: feature X commits (never approved, still testing)
main has: features A, B, C (shipped)

Sync brings A, B, C to dev
→ Conflict if X touched same files as A, B, or C
```

**Resolution strategies:**

| Strategy | When to use |
|----------|-------------|
| **Resolve manually** | Feature X is still active, conflicts are minor |
| **Drop feature X** | Feature X was abandoned, remove its branch from dev |
| **Recreate dev from main** | Major reset, re-merge active features |

For UAT (less frequent sync), conflicts are more likely. Consider:
1. Announcing sync windows so teams finish active UAT testing
2. Having a "UAT freeze" period before sync
3. Recreating UAT from main and re-merging only active UAT features

### The Full Environment Picture

```
                    ┌─────────────────────────────────────┐
                    │           PRODUCTION                 │
                    │         (main branch)                │
                    └─────────────────────────────────────┘
                           ↑              │
         merge (--no-ff)   │              │ sync (automated/scheduled)
         when approved     │              ↓
                    ┌──────┴────────────────────────┐
                    │        FEATURE BRANCHES        │
                    │    (branch from main)          │
                    └──────┬────────────────────────┘
                           │
         merge (--no-ff)   │ merge (--no-ff)
         for testing       │ for UAT
                           │
              ┌────────────┴────────────┐
              ↓                         ↓
┌─────────────────────────┐  ┌─────────────────────────┐
│    TEST ENVIRONMENT     │  │    UAT ENVIRONMENT      │
│      (dev branch)       │  │      (uat branch)       │
│                         │  │                         │
│  • Automated QA         │  │  • Stakeholder demos    │
│  • Continuous testing   │  │  • User acceptance      │
│  • Sync: after each     │  │  • Sync: monthly or     │
│    feature ships        │  │    on-demand            │
└─────────────────────────┘  └─────────────────────────┘
```

### Branch Protection Settings

| Branch | Setting | Value |
|--------|---------|-------|
| `main` | Require PR | Yes |
| `main` | Require approvals | 1+ (QA or senior dev) |
| `main` | Require status checks | CI passes |
| `main` | Allow merge commits | Yes (`--no-ff`) |
| `main` | Allow squash | No |
| `main` | Allow rebase | No |
| `dev` | Require PR | Yes |
| `dev` | Require approvals | Optional (team preference) |
| `dev` | Allow merge commits | Yes (`--no-ff`) |
| `dev` | Allow squash | No |
| `dev` | Allow rebase | No |
| `uat` | Require PR | Yes |
| `uat` | Require approvals | Optional (team preference) |
| `uat` | Allow merge commits | Yes (`--no-ff`) |
| `uat` | Allow squash | No |
| `uat` | Allow rebase | No |

### The Complete Picture

```
                    ┌─────────────────────────────────────┐
                    │           PRODUCTION                 │
                    │    (main branch deployments)         │
                    └─────────────────────────────────────┘
                                    ↑
                                    │ Merge (--no-ff) when approved
                                    │ release-damnit bumps versions
                                    │ Targeted builds run
                    ┌───────────────┴───────────────┐
                    │         FEATURE BRANCHES       │
                    │   (branch from main, owned by  │
                    │    developer/team)             │
                    ├────────────────────────────────┤
                    │ feature/jarvis-calendar        │
                    │ feature/sandbox-auth           │
                    │ feature/ma-observe-export      │
                    └───────────────┬───────────────┘
                                    │ Merge (--no-ff) for testing
                                    ↓
                    ┌─────────────────────────────────────┐
                    │           TEST ENVIRONMENT           │
                    │    (dev branch deployments)          │
                    │    QA validates features here        │
                    └─────────────────────────────────────┘
```

### Why This Works

| Property | How this model achieves it |
|----------|----------------------------|
| **Selective promotion** | Only approved features merge to main |
| **History preserved** | --no-ff merges everywhere, no squash on shared branches |
| **Independent versioning** | release-damnit bumps only packages touched by merged feature |
| **Parallel development** | Multiple features test on dev/uat simultaneously |
| **No all-or-nothing** | Each feature ships independently when ready |
| **Clear audit trail** | Main shows exactly which features shipped when |
| **Low conflict risk** | Features branch from main, minimal drift |
| **Stable test envs** | Periodic sync from main resets baseline |
| **Flexible UAT** | Features can target dev, uat, or both |

### What Changes from Today

| Today (painful) | After this model |
|-----------------|------------------|
| Cherry-pick by scope into merge branches | Merge feature branch directly to main |
| Multiple PRs to main per release | One PR per feature |
| Manual scope tracking | Automatic path-based detection |
| Wait for all scopes to be ready | Ship features independently |
| Complex ancestry-path filtering | Simple merge-base traversal |
| Risk of missing commits | All commits in merge are analyzed |
