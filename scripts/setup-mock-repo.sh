#!/bin/bash
# Creates or resets the mock--gitops-playground repository to a deterministic state.
# This provides a controlled environment for E2E testing of release-damnit.
#
# Usage:
#   ./scripts/setup-mock-repo.sh              # Full setup (creates repo structure)
#   ./scripts/setup-mock-repo.sh --local      # Local mode (creates in /tmp, no GitHub)
#
# The repository structure mirrors the production monorepo with:
# - Nested workloads (beacon, beacon/clients/web, etc.)
# - Platform packages (sandbox, infrastructure)
# - Linked versions (comet-client and comet-server)
# - release-please-config.json and manifest
# - .github/workflows/release-damnit.yml (actual CI pipeline)
#
# Feature branches created for testing various scenarios:
# - feature/single-feat: Single feat commit to one package
# - feature/multi-package: Commits touching multiple packages
# - feature/breaking-change: Breaking change (major bump)
# - feature/linked-versions: Change to linked package
# - feature/stacked-commits: Multiple commits of varying severity
# - feature/nested-package: Change to nested package (beacon-web inside beacon)

set -euo pipefail

REPO_URL="${MOCK_REPO_URL:-git@github.com:dsswift/mock--gitops-playground.git}"
LOCAL_MODE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --local)
            LOCAL_MODE=true
            shift
            ;;
        *)
            echo "Unknown argument: $1"
            exit 1
            ;;
    esac
done

# Create work directory
WORK_DIR=$(mktemp -d)
if [ "$LOCAL_MODE" = true ]; then
    echo "Local mode: working in $WORK_DIR"
else
    echo "Working in temporary directory: $WORK_DIR"
fi

cleanup() {
    if [ "$LOCAL_MODE" = false ] && [ -d "$WORK_DIR" ]; then
        rm -rf "$WORK_DIR"
    fi
}
trap cleanup EXIT

cd "$WORK_DIR"

# Initialize fresh repo (always start clean for deterministic state)
git init --initial-branch=main
git config user.email "release-damnit-test@dsswift.io"
git config user.name "release-damnit Test"

if [ "$LOCAL_MODE" = true ]; then
    # Create a "remote" for local testing
    mkdir -p "$WORK_DIR-origin"
    git init --bare "$WORK_DIR-origin"
    git remote add origin "$WORK_DIR-origin"
else
    git remote add origin "$REPO_URL"
fi

# =============================================================================
# Directory Structure (mirrors the production monorepo)
# =============================================================================

# Workloads
mkdir -p workloads/beacon/backend/src
mkdir -p workloads/beacon/clients/web/src
mkdir -p workloads/beacon/clients/discord/src
mkdir -p workloads/comet/client/src
mkdir -p workloads/comet/server/src

# Platforms
mkdir -p platforms/sandbox/portal/src
mkdir -p platforms/sandbox/images/claude-code

# Infrastructure
mkdir -p infrastructure/terraform/modules

# GitHub workflows
mkdir -p .github/workflows

# =============================================================================
# VERSION files (all start at 0.1.0)
# =============================================================================

for dir in \
    workloads/beacon \
    workloads/beacon/clients/web \
    workloads/beacon/clients/discord \
    workloads/comet/client \
    workloads/comet/server \
    platforms/sandbox/portal \
    platforms/sandbox/images/claude-code \
    infrastructure/terraform
do
    echo "0.1.0 # x-release-please-version" > "$dir/VERSION"
done

# =============================================================================
# CHANGELOG files
# =============================================================================

for dir in \
    workloads/beacon \
    workloads/beacon/clients/web \
    workloads/beacon/clients/discord \
    workloads/comet/client \
    workloads/comet/server \
    platforms/sandbox/portal \
    platforms/sandbox/images/claude-code \
    infrastructure/terraform
do
    cat > "$dir/CHANGELOG.md" << 'CHANGELOG_EOF'
# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - Initial Release

Initial release.
CHANGELOG_EOF
done

# =============================================================================
# Source files (placeholders)
# =============================================================================

echo "// Beacon backend service" > workloads/beacon/backend/src/main.go
echo "// Beacon web client" > workloads/beacon/clients/web/src/App.tsx
echo "// Beacon Discord bot" > workloads/beacon/clients/discord/src/bot.ts
echo "// MA-Observe client" > workloads/comet/client/src/client.go
echo "// MA-Observe server" > workloads/comet/server/src/server.go
echo "// Sandbox portal" > platforms/sandbox/portal/src/main.py
echo "FROM ubuntu:22.04" > platforms/sandbox/images/claude-code/Dockerfile
echo "# Terraform modules" > infrastructure/terraform/modules/README.md

# =============================================================================
# release-please-config.json (mirrors the structure of a production monorepo)
# =============================================================================

cat > release-please-config.json << 'CONFIG_EOF'
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "release-type": "simple",
  "bump-minor-pre-major": true,
  "bump-patch-for-minor-pre-major": true,
  "include-component-in-tag": true,
  "tag-separator": "-",
  "packages": {
    "workloads/beacon": {
      "component": "beacon",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/beacon/clients/web": {
      "component": "beacon-web",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/beacon/clients/discord": {
      "component": "beacon-discord",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/comet/client": {
      "component": "comet-client",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/comet/server": {
      "component": "comet-server",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "platforms/sandbox/portal": {
      "component": "sandbox-portal",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "platforms/sandbox/images/claude-code": {
      "component": "sandbox-image-claude-code",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "infrastructure/terraform": {
      "component": "infrastructure",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    }
  },
  "plugins": [
    {
      "type": "linked-versions",
      "groupName": "comet",
      "components": ["comet-client", "comet-server"]
    }
  ]
}
CONFIG_EOF

# =============================================================================
# release-please-manifest.json
# =============================================================================

cat > release-please-manifest.json << 'MANIFEST_EOF'
{
  "workloads/beacon": "0.1.0",
  "workloads/beacon/clients/web": "0.1.0",
  "workloads/beacon/clients/discord": "0.1.0",
  "workloads/comet/client": "0.1.0",
  "workloads/comet/server": "0.1.0",
  "platforms/sandbox/portal": "0.1.0",
  "platforms/sandbox/images/claude-code": "0.1.0",
  "infrastructure/terraform": "0.1.0"
}
MANIFEST_EOF

# =============================================================================
# .github/workflows/release-damnit.yml (the actual CI pipeline)
# =============================================================================

cat > .github/workflows/release-damnit.yml << 'WORKFLOW_EOF'
name: Release (release-damnit)

on:
  push:
    branches: [main]
  workflow_dispatch:
    inputs:
      dry_run:
        description: 'Dry run mode (no changes)'
        required: false
        default: 'false'
        type: boolean

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    outputs:
      releases_created: ${{ steps.release.outputs.releases_created }}
      beacon_release_created: ${{ steps.release.outputs.beacon_release_created }}
      beacon_web_release_created: ${{ steps.release.outputs.beacon-web_release_created }}
      beacon_discord_release_created: ${{ steps.release.outputs.beacon-discord_release_created }}
      ma_observe_client_release_created: ${{ steps.release.outputs.comet-client_release_created }}
      ma_observe_server_release_created: ${{ steps.release.outputs.comet-server_release_created }}
      sandbox_portal_release_created: ${{ steps.release.outputs.sandbox-portal_release_created }}
      sandbox_image_claude_code_release_created: ${{ steps.release.outputs.sandbox-image-claude-code_release_created }}
      infrastructure_release_created: ${{ steps.release.outputs.infrastructure_release_created }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run release-damnit
        id: release
        uses: dsswift/release-damnit@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          dry_run: ${{ inputs.dry_run || 'false' }}
          create_releases: 'true'

      - name: Show results
        run: |
          echo "Releases created: ${{ steps.release.outputs.releases_created }}"
          echo "---"
          echo "beacon: ${{ steps.release.outputs.beacon_release_created }}"
          echo "beacon-web: ${{ steps.release.outputs.beacon-web_release_created }}"
          echo "beacon-discord: ${{ steps.release.outputs.beacon-discord_release_created }}"
          echo "comet-client: ${{ steps.release.outputs.comet-client_release_created }}"
          echo "comet-server: ${{ steps.release.outputs.comet-server_release_created }}"
          echo "sandbox-portal: ${{ steps.release.outputs.sandbox-portal_release_created }}"
          echo "sandbox-image-claude-code: ${{ steps.release.outputs.sandbox-image-claude-code_release_created }}"
          echo "infrastructure: ${{ steps.release.outputs.infrastructure_release_created }}"

  # Example downstream build jobs (would be real builds in the production monorepo)
  build-beacon:
    needs: release
    if: needs.release.outputs.beacon_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building beacon..."

  build-beacon-web:
    needs: release
    if: needs.release.outputs.beacon_web_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building beacon-web..."

  build-comet:
    needs: release
    if: needs.release.outputs.ma_observe_client_release_created == 'true' || needs.release.outputs.ma_observe_server_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building comet (client and/or server)..."
WORKFLOW_EOF

# =============================================================================
# README.md
# =============================================================================

cat > README.md << 'README_EOF'
# mock--gitops-playground

Test repository for release-damnit E2E tests. **DO NOT USE FOR PRODUCTION.**

This repository is periodically reset by the release-damnit test suite.

## Structure

```
workloads/
├── beacon/                    # beacon (backend)
│   └── clients/
│       ├── web/               # beacon-web (nested under beacon)
│       └── discord/           # beacon-discord (nested under beacon)
└── comet/
    ├── client/                # comet-client (linked with server)
    └── server/                # comet-server (linked with client)
platforms/
└── sandbox/
    ├── portal/                # sandbox-portal
    └── images/
        └── claude-code/       # sandbox-image-claude-code
infrastructure/
└── terraform/                 # infrastructure
```

## Package Behaviors

| Package | Behavior |
|---------|----------|
| beacon | Standalone - changes to beacon/ (but not clients/) |
| beacon-web | Nested - changes to beacon/clients/web/ |
| beacon-discord | Nested - changes to beacon/clients/discord/ |
| comet-client | Linked - bumps together with comet-server |
| comet-server | Linked - bumps together with comet-client |
| sandbox-portal | Standalone |
| sandbox-image-claude-code | Standalone |
| infrastructure | Standalone |

## Feature Branches

These branches test different release scenarios:

| Branch | Scenario |
|--------|----------|
| feature/single-feat | One feat commit to beacon |
| feature/multi-package | Commits to beacon, sandbox-portal, infrastructure |
| feature/breaking-change | feat!: breaking change (major bump) |
| feature/linked-versions | Change to comet-client (triggers both) |
| feature/stacked-commits | Multiple commits with different severities |
| feature/nested-package | Change to beacon-web (nested inside beacon) |
README_EOF

# =============================================================================
# Initial commit
# =============================================================================

git add -A
git commit -m "chore: initial repository structure"

INITIAL_SHA=$(git rev-parse HEAD)
echo "Initial commit: $INITIAL_SHA"

# Create dev branch (copy of main)
git checkout -b dev
git checkout main

# =============================================================================
# Feature branch: single-feat
# One feat commit to beacon
# =============================================================================

git checkout -b feature/single-feat

echo "// New calendar integration" >> workloads/beacon/backend/src/main.go
git add workloads/beacon/backend/src/main.go
git commit -m "feat(beacon): add calendar integration"

git checkout main

# =============================================================================
# Feature branch: multi-package
# Commits touching beacon, sandbox-portal, and infrastructure
# =============================================================================

git checkout -b feature/multi-package

echo "// Recipe improvements" >> workloads/beacon/backend/src/main.go
git add workloads/beacon/backend/src/main.go
git commit -m "feat(beacon): improve recipe search"

echo "// Dashboard redesign" >> platforms/sandbox/portal/src/main.py
git add platforms/sandbox/portal/src/main.py
git commit -m "feat(sandbox-portal): redesign dashboard"

echo "# Add monitoring module" >> infrastructure/terraform/modules/README.md
git add infrastructure/terraform/modules/README.md
git commit -m "feat(infrastructure): add monitoring module"

git checkout main

# =============================================================================
# Feature branch: breaking-change
# Breaking change requiring major bump
# =============================================================================

git checkout -b feature/breaking-change

echo "// BREAKING: New API structure" >> platforms/sandbox/portal/src/main.py
git add platforms/sandbox/portal/src/main.py
git commit -m "feat(sandbox-portal)!: redesign API with breaking changes"

git checkout main

# =============================================================================
# Feature branch: linked-versions
# Change to comet-client (should bump both client and server)
# =============================================================================

git checkout -b feature/linked-versions

echo "// New metrics collector" >> workloads/comet/client/src/client.go
git add workloads/comet/client/src/client.go
git commit -m "feat(comet-client): add metrics collector"

git checkout main

# =============================================================================
# Feature branch: stacked-commits
# Multiple commits with different severities to same package
# Expected: highest severity wins (feat > fix > chore)
# =============================================================================

git checkout -b feature/stacked-commits

echo "// Chore: cleanup" >> workloads/beacon/clients/discord/src/bot.ts
git add workloads/beacon/clients/discord/src/bot.ts
git commit -m "chore(beacon-discord): cleanup old code"

echo "// Fix: handle rate limits" >> workloads/beacon/clients/discord/src/bot.ts
git add workloads/beacon/clients/discord/src/bot.ts
git commit -m "fix(beacon-discord): handle rate limits properly"

echo "// Feature: slash commands" >> workloads/beacon/clients/discord/src/bot.ts
git add workloads/beacon/clients/discord/src/bot.ts
git commit -m "feat(beacon-discord): add slash commands support"

echo "// Another fix" >> workloads/beacon/clients/discord/src/bot.ts
git add workloads/beacon/clients/discord/src/bot.ts
git commit -m "fix(beacon-discord): fix command parsing"

git checkout main

# =============================================================================
# Feature branch: nested-package
# Change to beacon-web (nested inside beacon)
# Should only bump beacon-web, not beacon (deepest match wins)
# =============================================================================

git checkout -b feature/nested-package

echo "// New React component" >> workloads/beacon/clients/web/src/App.tsx
git add workloads/beacon/clients/web/src/App.tsx
git commit -m "feat(beacon-web): add notification component"

git checkout main

# =============================================================================
# Feature branch: complex-merge (the "kitchen sink" test)
# Tests ALL package types in a single merge:
# - Simple scopes: beacon, sandbox-portal, infrastructure, sandbox-image-claude-code
# - Linked scopes: comet-client (triggers comet-server)
# - Nested scopes: beacon-web, beacon-discord (both nested inside beacon)
# - Multiple commit types: feat, fix, chore, feat! (breaking)
# - Stacked commits: Multiple commits to same package
#
# Expected results (8 packages total):
# - beacon: 0.1.0 → 0.1.1 (feat)
# - beacon-web: 0.1.0 → 0.1.1 (feat wins over fix)
# - beacon-discord: 0.1.0 → 0.1.1 (fix)
# - comet-client: 0.1.0 → 0.1.1 (feat)
# - comet-server: 0.1.0 → 0.1.1 (linked, no direct commits)
# - sandbox-portal: 0.1.0 → 1.0.0 (feat! breaking)
# - sandbox-image-claude-code: 0.1.0 → 0.1.1 (feat)
# - infrastructure: 0.1.0 → 0.1.1 (fix)
# =============================================================================

git checkout -b feature/complex-merge

# --- beacon (simple scope, multiple commits) ---
echo "// Add recipe feature" >> workloads/beacon/backend/src/main.go
git add workloads/beacon/backend/src/main.go
git commit -m "feat(beacon): add recipe recommendation engine"

echo "// Fix recipe bug" >> workloads/beacon/backend/src/main.go
git add workloads/beacon/backend/src/main.go
git commit -m "fix(beacon): handle missing ingredients gracefully"

# --- beacon-web (nested scope, stacked commits - feat wins) ---
echo "// Fix button styling" >> workloads/beacon/clients/web/src/App.tsx
git add workloads/beacon/clients/web/src/App.tsx
git commit -m "fix(beacon-web): fix button alignment on mobile"

echo "// Add dark mode" >> workloads/beacon/clients/web/src/App.tsx
git add workloads/beacon/clients/web/src/App.tsx
git commit -m "feat(beacon-web): add dark mode support"

echo "// Cleanup imports" >> workloads/beacon/clients/web/src/App.tsx
git add workloads/beacon/clients/web/src/App.tsx
git commit -m "chore(beacon-web): cleanup unused imports"

# --- beacon-discord (nested scope, single fix) ---
echo "// Handle rate limit" >> workloads/beacon/clients/discord/src/bot.ts
git add workloads/beacon/clients/discord/src/bot.ts
git commit -m "fix(beacon-discord): handle Discord API rate limits"

# --- comet-client (linked scope - should trigger server bump too) ---
echo "// Add new metric type" >> workloads/comet/client/src/client.go
git add workloads/comet/client/src/client.go
git commit -m "feat(comet-client): add CPU temperature metric"

echo "// Fix metric parsing" >> workloads/comet/client/src/client.go
git add workloads/comet/client/src/client.go
git commit -m "fix(comet-client): fix metric timestamp parsing"

# --- sandbox-portal (breaking change - major bump) ---
echo "// BREAKING: New API v2" >> platforms/sandbox/portal/src/main.py
git add platforms/sandbox/portal/src/main.py
git commit -m "feat(sandbox-portal)!: redesign REST API with v2 schema

BREAKING CHANGE: All API endpoints now use /api/v2 prefix.
Old /api/v1 endpoints are removed."

# --- sandbox-image-claude-code (simple scope) ---
echo "RUN apt-get update" >> platforms/sandbox/images/claude-code/Dockerfile
git add platforms/sandbox/images/claude-code/Dockerfile
git commit -m "feat(sandbox-image-claude-code): add system package updates"

# --- infrastructure (simple scope, fix only) ---
echo "# Fix module path" >> infrastructure/terraform/modules/README.md
git add infrastructure/terraform/modules/README.md
git commit -m "fix(infrastructure): correct module source path"

git checkout main

# =============================================================================
# Push all branches
# =============================================================================

echo ""
echo "Pushing to origin..."
git push --force origin main
git push --force origin dev
git push --force origin feature/single-feat
git push --force origin feature/multi-package
git push --force origin feature/breaking-change
git push --force origin feature/linked-versions
git push --force origin feature/stacked-commits
git push --force origin feature/nested-package
git push --force origin feature/complex-merge

if [ "$LOCAL_MODE" = true ]; then
    echo ""
    echo "Local repository setup complete: $WORK_DIR"
    echo "Origin: $WORK_DIR-origin"
    trap - EXIT
else
    echo ""
    echo "Repository setup complete: $REPO_URL"
fi

echo ""
echo "=== Setup Summary ==="
echo ""
echo "Initial commit: $INITIAL_SHA"
echo ""
echo "Packages (8 total):"
echo "  - workloads/beacon (beacon)"
echo "  - workloads/beacon/clients/web (beacon-web) - nested"
echo "  - workloads/beacon/clients/discord (beacon-discord) - nested"
echo "  - workloads/comet/client (comet-client) - linked"
echo "  - workloads/comet/server (comet-server) - linked"
echo "  - platforms/sandbox/portal (sandbox-portal)"
echo "  - platforms/sandbox/images/claude-code (sandbox-image-claude-code)"
echo "  - infrastructure/terraform (infrastructure)"
echo ""
echo "Feature branches (7 scenarios):"
echo "  - feature/single-feat: 1 commit (feat) → beacon minor bump"
echo "  - feature/multi-package: 3 commits → beacon, sandbox-portal, infrastructure minor bumps"
echo "  - feature/breaking-change: 1 commit (feat!) → sandbox-portal MAJOR bump"
echo "  - feature/linked-versions: 1 commit → comet-client AND comet-server minor bumps"
echo "  - feature/stacked-commits: 4 commits (chore, fix, feat, fix) → beacon-discord minor bump (feat wins)"
echo "  - feature/nested-package: 1 commit → beacon-web minor bump (not beacon)"
echo "  - feature/complex-merge: 12 commits → ALL 8 packages bumped (the \"kitchen sink\" test)"
echo "      Simple: beacon, sandbox-portal (MAJOR!), sandbox-image-claude-code, infrastructure"
echo "      Linked: comet-client + comet-server (server has no direct commits)"
echo "      Nested: beacon-web (feat wins), beacon-discord"
echo ""
echo "CI Pipeline: .github/workflows/release-damnit.yml"
