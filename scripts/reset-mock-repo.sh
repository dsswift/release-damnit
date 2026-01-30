#!/bin/bash
# Resets the mock--gitops-playground repository to the baseline state.
# Use this after running tests that modify the repository.
#
# Usage:
#   ./scripts/reset-mock-repo.sh              # Reset from GitHub
#   ./scripts/reset-mock-repo.sh /path/to/local  # Reset local repo
#
# This is a lightweight reset that just runs setup again.
# For local testing, provide the path to the local repo.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ $# -gt 0 ]; then
    # Local mode - just re-run setup
    echo "Re-running setup for local reset..."
    "$SCRIPT_DIR/setup-mock-repo.sh" --local
else
    # GitHub mode
    echo "Re-running setup for GitHub reset..."
    "$SCRIPT_DIR/setup-mock-repo.sh"
fi

echo ""
echo "Repository reset to baseline state."
