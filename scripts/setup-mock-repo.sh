#!/bin/bash
# Creates or resets the mock--gitops-playground repository to a deterministic state.
# This provides a controlled environment for E2E testing of release-damnit.
#
# Usage:
#   ./scripts/setup-mock-repo.sh              # Full setup (creates repo structure)
#   ./scripts/setup-mock-repo.sh --local      # Local mode (creates in /tmp, no GitHub)
#
# The repository structure mirrors sh-monorepo with:
# - Nested workloads (jarvis, jarvis/clients/web, etc.)
# - Platform packages (sandbox, infrastructure)
# - Linked versions (ma-observe-client and ma-observe-server)
# - release-please-config.json and manifest
# - .github/workflows/release-damnit.yml (actual CI pipeline)
#
# Feature branches created for testing various scenarios:
# - feature/single-feat: Single feat commit to one package
# - feature/multi-package: Commits touching multiple packages
# - feature/breaking-change: Breaking change (major bump)
# - feature/linked-versions: Change to linked package
# - feature/stacked-commits: Multiple commits of varying severity
# - feature/nested-package: Change to nested package (jarvis-web inside jarvis)

set -euo pipefail

REPO_URL="${MOCK_REPO_URL:-git@github.com:spraguehouse/mock--gitops-playground.git}"
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
git config user.email "release-damnit-test@spraguehouse.io"
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
# Directory Structure (mirrors sh-monorepo)
# =============================================================================

# Workloads
mkdir -p workloads/jarvis/backend/src
mkdir -p workloads/jarvis/clients/web/src
mkdir -p workloads/jarvis/clients/discord/src
mkdir -p workloads/ma-observe/client/src
mkdir -p workloads/ma-observe/server/src

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
    workloads/jarvis \
    workloads/jarvis/clients/web \
    workloads/jarvis/clients/discord \
    workloads/ma-observe/client \
    workloads/ma-observe/server \
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
    workloads/jarvis \
    workloads/jarvis/clients/web \
    workloads/jarvis/clients/discord \
    workloads/ma-observe/client \
    workloads/ma-observe/server \
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

echo "// Jarvis backend service" > workloads/jarvis/backend/src/main.go
echo "// Jarvis web client" > workloads/jarvis/clients/web/src/App.tsx
echo "// Jarvis Discord bot" > workloads/jarvis/clients/discord/src/bot.ts
echo "// MA-Observe client" > workloads/ma-observe/client/src/client.go
echo "// MA-Observe server" > workloads/ma-observe/server/src/server.go
echo "// Sandbox portal" > platforms/sandbox/portal/src/main.py
echo "FROM ubuntu:22.04" > platforms/sandbox/images/claude-code/Dockerfile
echo "# Terraform modules" > infrastructure/terraform/modules/README.md

# =============================================================================
# release-please-config.json (mirrors sh-monorepo structure)
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
    "workloads/jarvis": {
      "component": "jarvis",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/jarvis/clients/web": {
      "component": "jarvis-web",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/jarvis/clients/discord": {
      "component": "jarvis-discord",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/ma-observe/client": {
      "component": "ma-observe-client",
      "changelog-path": "CHANGELOG.md",
      "extra-files": [{"type": "generic", "path": "VERSION"}]
    },
    "workloads/ma-observe/server": {
      "component": "ma-observe-server",
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
      "groupName": "ma-observe",
      "components": ["ma-observe-client", "ma-observe-server"]
    }
  ]
}
CONFIG_EOF

# =============================================================================
# release-please-manifest.json
# =============================================================================

cat > release-please-manifest.json << 'MANIFEST_EOF'
{
  "workloads/jarvis": "0.1.0",
  "workloads/jarvis/clients/web": "0.1.0",
  "workloads/jarvis/clients/discord": "0.1.0",
  "workloads/ma-observe/client": "0.1.0",
  "workloads/ma-observe/server": "0.1.0",
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
      jarvis_release_created: ${{ steps.release.outputs.jarvis_release_created }}
      jarvis_web_release_created: ${{ steps.release.outputs.jarvis-web_release_created }}
      jarvis_discord_release_created: ${{ steps.release.outputs.jarvis-discord_release_created }}
      ma_observe_client_release_created: ${{ steps.release.outputs.ma-observe-client_release_created }}
      ma_observe_server_release_created: ${{ steps.release.outputs.ma-observe-server_release_created }}
      sandbox_portal_release_created: ${{ steps.release.outputs.sandbox-portal_release_created }}
      sandbox_image_claude_code_release_created: ${{ steps.release.outputs.sandbox-image-claude-code_release_created }}
      infrastructure_release_created: ${{ steps.release.outputs.infrastructure_release_created }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run release-damnit
        id: release
        uses: spraguehouse/release-damnit@main
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          dry_run: ${{ inputs.dry_run || 'false' }}
          create_releases: 'true'

      - name: Show results
        run: |
          echo "Releases created: ${{ steps.release.outputs.releases_created }}"
          echo "---"
          echo "jarvis: ${{ steps.release.outputs.jarvis_release_created }}"
          echo "jarvis-web: ${{ steps.release.outputs.jarvis-web_release_created }}"
          echo "jarvis-discord: ${{ steps.release.outputs.jarvis-discord_release_created }}"
          echo "ma-observe-client: ${{ steps.release.outputs.ma-observe-client_release_created }}"
          echo "ma-observe-server: ${{ steps.release.outputs.ma-observe-server_release_created }}"
          echo "sandbox-portal: ${{ steps.release.outputs.sandbox-portal_release_created }}"
          echo "sandbox-image-claude-code: ${{ steps.release.outputs.sandbox-image-claude-code_release_created }}"
          echo "infrastructure: ${{ steps.release.outputs.infrastructure_release_created }}"

  # Example downstream build jobs (would be real builds in sh-monorepo)
  build-jarvis:
    needs: release
    if: needs.release.outputs.jarvis_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building jarvis..."

  build-jarvis-web:
    needs: release
    if: needs.release.outputs.jarvis_web_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building jarvis-web..."

  build-ma-observe:
    needs: release
    if: needs.release.outputs.ma_observe_client_release_created == 'true' || needs.release.outputs.ma_observe_server_release_created == 'true'
    runs-on: ubuntu-latest
    steps:
      - run: echo "Building ma-observe (client and/or server)..."
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
├── jarvis/                    # jarvis (backend)
│   └── clients/
│       ├── web/               # jarvis-web (nested under jarvis)
│       └── discord/           # jarvis-discord (nested under jarvis)
└── ma-observe/
    ├── client/                # ma-observe-client (linked with server)
    └── server/                # ma-observe-server (linked with client)
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
| jarvis | Standalone - changes to jarvis/ (but not clients/) |
| jarvis-web | Nested - changes to jarvis/clients/web/ |
| jarvis-discord | Nested - changes to jarvis/clients/discord/ |
| ma-observe-client | Linked - bumps together with ma-observe-server |
| ma-observe-server | Linked - bumps together with ma-observe-client |
| sandbox-portal | Standalone |
| sandbox-image-claude-code | Standalone |
| infrastructure | Standalone |

## Feature Branches

These branches test different release scenarios:

| Branch | Scenario |
|--------|----------|
| feature/single-feat | One feat commit to jarvis |
| feature/multi-package | Commits to jarvis, sandbox-portal, infrastructure |
| feature/breaking-change | feat!: breaking change (major bump) |
| feature/linked-versions | Change to ma-observe-client (triggers both) |
| feature/stacked-commits | Multiple commits with different severities |
| feature/nested-package | Change to jarvis-web (nested inside jarvis) |
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
# One feat commit to jarvis
# =============================================================================

git checkout -b feature/single-feat

echo "// New calendar integration" >> workloads/jarvis/backend/src/main.go
git add workloads/jarvis/backend/src/main.go
git commit -m "feat(jarvis): add calendar integration"

git checkout main

# =============================================================================
# Feature branch: multi-package
# Commits touching jarvis, sandbox-portal, and infrastructure
# =============================================================================

git checkout -b feature/multi-package

echo "// Recipe improvements" >> workloads/jarvis/backend/src/main.go
git add workloads/jarvis/backend/src/main.go
git commit -m "feat(jarvis): improve recipe search"

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
# Change to ma-observe-client (should bump both client and server)
# =============================================================================

git checkout -b feature/linked-versions

echo "// New metrics collector" >> workloads/ma-observe/client/src/client.go
git add workloads/ma-observe/client/src/client.go
git commit -m "feat(ma-observe-client): add metrics collector"

git checkout main

# =============================================================================
# Feature branch: stacked-commits
# Multiple commits with different severities to same package
# Expected: highest severity wins (feat > fix > chore)
# =============================================================================

git checkout -b feature/stacked-commits

echo "// Chore: cleanup" >> workloads/jarvis/clients/discord/src/bot.ts
git add workloads/jarvis/clients/discord/src/bot.ts
git commit -m "chore(jarvis-discord): cleanup old code"

echo "// Fix: handle rate limits" >> workloads/jarvis/clients/discord/src/bot.ts
git add workloads/jarvis/clients/discord/src/bot.ts
git commit -m "fix(jarvis-discord): handle rate limits properly"

echo "// Feature: slash commands" >> workloads/jarvis/clients/discord/src/bot.ts
git add workloads/jarvis/clients/discord/src/bot.ts
git commit -m "feat(jarvis-discord): add slash commands support"

echo "// Another fix" >> workloads/jarvis/clients/discord/src/bot.ts
git add workloads/jarvis/clients/discord/src/bot.ts
git commit -m "fix(jarvis-discord): fix command parsing"

git checkout main

# =============================================================================
# Feature branch: nested-package
# Change to jarvis-web (nested inside jarvis)
# Should only bump jarvis-web, not jarvis (deepest match wins)
# =============================================================================

git checkout -b feature/nested-package

echo "// New React component" >> workloads/jarvis/clients/web/src/App.tsx
git add workloads/jarvis/clients/web/src/App.tsx
git commit -m "feat(jarvis-web): add notification component"

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
echo "  - workloads/jarvis (jarvis)"
echo "  - workloads/jarvis/clients/web (jarvis-web) - nested"
echo "  - workloads/jarvis/clients/discord (jarvis-discord) - nested"
echo "  - workloads/ma-observe/client (ma-observe-client) - linked"
echo "  - workloads/ma-observe/server (ma-observe-server) - linked"
echo "  - platforms/sandbox/portal (sandbox-portal)"
echo "  - platforms/sandbox/images/claude-code (sandbox-image-claude-code)"
echo "  - infrastructure/terraform (infrastructure)"
echo ""
echo "Feature branches (6 scenarios):"
echo "  - feature/single-feat: 1 commit (feat) → jarvis minor bump"
echo "  - feature/multi-package: 3 commits → jarvis, sandbox-portal, infrastructure minor bumps"
echo "  - feature/breaking-change: 1 commit (feat!) → sandbox-portal MAJOR bump"
echo "  - feature/linked-versions: 1 commit → ma-observe-client AND ma-observe-server minor bumps"
echo "  - feature/stacked-commits: 4 commits (chore, fix, feat, fix) → jarvis-discord minor bump (feat wins)"
echo "  - feature/nested-package: 1 commit → jarvis-web minor bump (not jarvis)"
echo ""
echo "CI Pipeline: .github/workflows/release-damnit.yml"
