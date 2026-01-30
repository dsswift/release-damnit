#!/bin/bash
# Creates or resets the mock--gitops-playground repository to a deterministic state.
# This provides a controlled environment for E2E testing of release-damnit.
#
# Usage:
#   ./scripts/setup-mock-repo.sh              # Full setup (creates repo structure)
#   ./scripts/setup-mock-repo.sh --local      # Local mode (creates in /tmp, no GitHub)
#
# The repository structure mirrors a typical monorepo with:
# - Multiple packages at different paths
# - release-please-config.json and manifest
# - VERSION and CHANGELOG files per package
# - Linked versions configuration
#
# Branches created:
# - main: Production baseline
# - dev: Test environment (syncs from main)
# - feature/service-a-update: Sample feature branch with commits

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

# Create directory structure
mkdir -p workloads/service-a/src
mkdir -p workloads/service-b/src
mkdir -p workloads/service-c/src
mkdir -p platforms/shared

# Create VERSION files
echo "0.1.0 # x-release-please-version" > workloads/service-a/VERSION
echo "0.1.0 # x-release-please-version" > workloads/service-b/VERSION
echo "0.1.0 # x-release-please-version" > workloads/service-c/VERSION
echo "0.1.0 # x-release-please-version" > platforms/shared/VERSION

# Create empty CHANGELOG files
for dir in workloads/service-a workloads/service-b workloads/service-c platforms/shared; do
    cat > "$dir/CHANGELOG.md" << 'CHANGELOG_EOF'
# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - Initial Release

Initial release.
CHANGELOG_EOF
done

# Create placeholder source files
echo "// Service A" > workloads/service-a/src/main.go
echo "// Service B" > workloads/service-b/src/main.go
echo "// Service C" > workloads/service-c/src/main.go
echo "// Shared utilities" > platforms/shared/lib.go

# Create release-please-config.json
cat > release-please-config.json << 'CONFIG_EOF'
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "packages": {
    "workloads/service-a": {
      "component": "service-a",
      "changelog-path": "CHANGELOG.md"
    },
    "workloads/service-b": {
      "component": "service-b",
      "changelog-path": "CHANGELOG.md"
    },
    "workloads/service-c": {
      "component": "service-c",
      "changelog-path": "CHANGELOG.md"
    },
    "platforms/shared": {
      "component": "shared",
      "changelog-path": "CHANGELOG.md"
    }
  },
  "plugins": [
    {
      "type": "linked-versions",
      "groupName": "services-ab",
      "components": ["service-a", "service-b"]
    }
  ]
}
CONFIG_EOF

# Create release-please-manifest.json
cat > release-please-manifest.json << 'MANIFEST_EOF'
{
  "workloads/service-a": "0.1.0",
  "workloads/service-b": "0.1.0",
  "workloads/service-c": "0.1.0",
  "platforms/shared": "0.1.0"
}
MANIFEST_EOF

# Create README
cat > README.md << 'README_EOF'
# mock--gitops-playground

Test repository for release-damnit E2E tests.

**DO NOT USE FOR PRODUCTION**

This repository is periodically reset to a baseline state by the release-damnit
test suite. Any manual changes will be overwritten.

## Structure

```
workloads/
├── service-a/     # Component: service-a (linked with service-b)
├── service-b/     # Component: service-b (linked with service-a)
└── service-c/     # Component: service-c (standalone)
platforms/
└── shared/        # Component: shared (standalone)
```

## Linked Versions

service-a and service-b are linked. When one is bumped, both are bumped.
README_EOF

# Initial commit
git add -A
git commit -m "chore: initial repository structure"

# Store the initial commit SHA for verification
INITIAL_SHA=$(git rev-parse HEAD)
echo "Initial commit: $INITIAL_SHA"

# Create dev branch (copy of main)
git checkout -b dev
git checkout main

# Create feature branch with conventional commits
git checkout -b feature/service-a-update

# Add a feat commit touching service-a
echo "// Feature: Add new endpoint" >> workloads/service-a/src/main.go
git add workloads/service-a/src/main.go
git commit -m "feat(service-a): add new endpoint for users"

# Add a fix commit touching service-a
echo "// Fix: Handle null input" >> workloads/service-a/src/main.go
git add workloads/service-a/src/main.go
git commit -m "fix(service-a): handle null input gracefully"

# Add a chore commit (should not trigger bump)
echo "// Updated comment" >> workloads/service-a/src/main.go
git add workloads/service-a/src/main.go
git commit -m "chore(service-a): update code comments"

git checkout main

# Create another feature branch touching multiple services
git checkout -b feature/multi-service

# Touch service-b
echo "// New feature" >> workloads/service-b/src/main.go
git add workloads/service-b/src/main.go
git commit -m "feat(service-b): add batch processing"

# Touch service-c
echo "// Bug fix" >> workloads/service-c/src/main.go
git add workloads/service-c/src/main.go
git commit -m "fix(service-c): fix memory leak"

# Touch shared
echo "// Utility function" >> platforms/shared/lib.go
git add platforms/shared/lib.go
git commit -m "feat(shared): add utility function"

git checkout main

# Push all branches (force push to reset remote state)
echo "Pushing to origin..."
git push --force origin main
git push --force origin dev
git push --force origin feature/service-a-update
git push --force origin feature/multi-service

if [ "$LOCAL_MODE" = true ]; then
    echo ""
    echo "Local repository setup complete: $WORK_DIR"
    echo "Origin: $WORK_DIR-origin"
    # Don't cleanup in local mode
    trap - EXIT
else
    echo ""
    echo "Repository setup complete: $REPO_URL"
fi

echo ""
echo "Branches:"
git branch -a

echo ""
echo "Initial commit SHA: $INITIAL_SHA"

echo ""
echo "Packages:"
echo "  - workloads/service-a (service-a) - linked with service-b"
echo "  - workloads/service-b (service-b) - linked with service-a"
echo "  - workloads/service-c (service-c) - standalone"
echo "  - platforms/shared (shared) - standalone"

echo ""
echo "Feature branches:"
echo "  - feature/service-a-update: 3 commits (feat, fix, chore) touching service-a"
echo "  - feature/multi-service: 3 commits touching service-b, service-c, shared"
