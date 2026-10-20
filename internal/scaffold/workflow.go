package scaffold

import "strings"

// DefaultActionRef is the release-damnit action a scaffolded workflow pins.
const DefaultActionRef = "dsswift/release-damnit@v1.4.0"

// releaseWorkflow is the standard release workflow. USES is the action reference.
const releaseWorkflow = `name: Release

on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: write

concurrency:
  group: release
  cancel-in-progress: false

jobs:
  version:
    if: ${{ !contains(github.event.head_commit.message, '[skip ci]') }}
    runs-on: ubuntu-latest
    outputs:
      released: ${{ steps.release.outputs.released }}
      tag: ${{ steps.release.outputs.tag }}
      version: ${{ steps.release.outputs.version }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - id: release
        uses: USES
        with:
          since-last-release: 'true'
          publish: 'true'

  build:
    needs: version
    if: needs.version.outputs.released == 'true'
    uses: ./.github/workflows/build.yml
    with:
      tag: ${{ needs.version.outputs.tag }}
    secrets: inherit
`

// ReleaseWorkflow returns the release workflow using the release-damnit
// action at uses: a pinned "owner/repo@ref", or "./" for the action's own repo.
func ReleaseWorkflow(uses string) string {
	return strings.Replace(releaseWorkflow, "USES", uses, 1)
}
