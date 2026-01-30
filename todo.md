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
- [ ] Integration tests against temp repos
- [ ] E2E tests against mock--gitops-playground
- [ ] **BLOG**: Update with CLI design decisions, test results

## Phase 5: GitHub Integration
- [ ] internal/release - create GitHub releases via gh CLI
- [ ] internal/release - tests
- [x] action.yml - composite action wrapper
- [ ] E2E test: create actual GitHub release in mock repo
- [ ] **BLOG**: Update with GitHub Action packaging experience

## Phase 6: sh-monorepo Integration
- [ ] Create sh-monorepo/docs/standards/gitops.md
- [ ] Update ADR-009
- [ ] Update .github/workflows/release.yml
- [ ] Test with real feature→main merge
- [ ] Verify targeted builds work

## Phase 7: Release
- [ ] Final testing pass
- [ ] **BLOG**: Final update - production results, lessons learned, what worked/didn't
- [ ] Tag release-damnit v1.0.0
- [ ] Document in sh-monorepo README
