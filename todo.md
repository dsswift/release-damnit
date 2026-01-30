# Implementation Roadmap

## Phase 1: Documentation & Foundation
- [x] Create docs/adr/adr-000-use-adrs.md
- [x] Create docs/adr/adr-001-implementation-language.md
- [x] Create docs/adr/adr-002-testing-strategy.md
- [x] Create docs/adr/adr-003-config-compatibility.md
- [x] Create docs/README.md
- [x] Create docs/blog--creating-release-damnit.md (initial draft: the problem, failed attempts)
- [x] Create README.md
- [x] Initialize Go module (`go mod init`)
- [x] Create Makefile with build/test targets
- [x] Create .gitignore

## Phase 2: Test Infrastructure
- [x] Create pkg/contracts - assertion helpers
- [x] Create scripts/setup-mock-repo.sh
- [x] Create scripts/reset-mock-repo.sh
- [x] Initialize mock--gitops-playground with baseline structure
- [x] Verify mock repo can be reset deterministically
  - Tree hash verified: 27dab00976668188366d17b2fce1638acd1eb1d0
- [x] **BLOG**: Update with test infrastructure decisions and any surprises

## Phase 3: Core (test-first)
- [x] internal/config - parse release-please-config.json
- [x] internal/config - unit tests
- [x] internal/git - merge detection (is HEAD a merge commit?)
- [x] internal/git - commit traversal (get all commits in merge)
- [x] internal/git - integration tests with temp repos
- [x] internal/version - semver parsing
- [x] internal/version - bump logic (patch, minor, major)
- [x] internal/version - unit tests
- [x] internal/changelog - generate changelog entries
- [x] internal/changelog - unit tests
- [x] **BLOG**: Update with core implementation learnings, any tricky git edge cases

## Phase 4: CLI Integration
- [x] cmd/release-damnit/main.go - CLI entrypoint
- [x] Wire up config → git → version → changelog
- [x] --dry-run flag
- [x] Integration tests against temp repos
  - TestAnalyze_NonMergeCommit, TestAnalyze_MergeCommit, TestAnalyze_MultiplePackages
  - TestAnalyze_LinkedVersions, TestAnalyze_NoReleasableCommits, TestApply_UpdatesFiles
- [x] E2E tests against mock--gitops-playground
  - TestE2E_AnalyzeMockRepo, TestE2E_MergeFeatureBranch, TestE2E_MergeMultiServiceBranch
  - TestE2E_ApplyAndVerify, TestE2E_DryRunMakesNoChanges
- [x] **BLOG**: Update with CLI design decisions, test results
  - Documented edge cases: single-commit repos, linked packages with no commits

## Phase 5: GitHub Integration
- [x] internal/release - create GitHub releases via gh CLI
  - internal/release/github.go: BuildGitHubRelease, CreateGitHubReleases, BuildReleaseNotes
- [x] internal/release - tests
  - internal/release/github_test.go: comprehensive unit tests
- [x] action.yml - composite action wrapper
- [x] E2E test: GitHub release flow (real releases)
  - TestE2E_FullGitHubReleaseFlow, TestE2E_MultiServiceGitHubReleases
  - Tests push to timestamped branches on origin, create real releases, verify via gh CLI, cleanup
- [x] **BLOG**: Update with GitHub Action packaging experience

## Phase 6: sh-monorepo Integration
- [x] Create sh-monorepo/docs/standards/gitops.md
  - Documents simplified workflow with release-damnit
- [x] Update ADR-009
  - Added "Alternative: release-damnit" section
- [x] Create .github/workflows/release-damnit.yml (draft)
  - Parallel workflow to test without disrupting existing release-please.yml
  - Uses spraguehouse/release-damnit@main action
- [ ] Test with real feature→main merge
  - Requires disabling release-please.yml temporarily
  - Manual testing step
- [ ] Verify targeted builds work

## Phase 7: Release
- [ ] Final testing pass
- [ ] **BLOG**: Final update - production results, lessons learned, what worked/didn't
- [ ] Tag release-damnit v1.0.0
- [ ] Document in sh-monorepo README
