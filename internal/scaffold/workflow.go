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
      # The branch tip, not the pushed commit: a run queued behind another
      # release must see that release's version commit and tags.
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.ref_name }}
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

// DefaultReleaseDamnitVersion is the release-damnit binary a scaffolded
// GitLab pipeline downloads.
const DefaultReleaseDamnitVersion = "v1.5.0"

// GitLabCIPath and GitLabBuildPath are where the GitLab pipeline goes.
const (
	GitLabCIPath    = ".gitlab-ci.yml"
	GitLabBuildPath = ".gitlab/build.yml"
)

// gitlabCI is the standard GitLab pipeline. {{version}} is the release-damnit tag.
const gitlabCI = `# Every push to the default branch versions every feat and fix since the
# last release with release-damnit, commits and tags the version, and creates
# the GitLab release. The tag pipeline then runs the build in .gitlab/build.yml.
#
# Needs one masked CI/CD variable, GITLAB_RELEASE_TOKEN: a project access
# token with the api and write_repository scopes and a role allowed to push
# to the default branch.

stages:
  - release
  - build

include:
  - local: .gitlab/build.yml

release:
  stage: release
  image: alpine:3.22
  rules:
    - if: $CI_PIPELINE_SOURCE == "push" && $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  resource_group: release
  variables:
    GIT_DEPTH: "0"
    RELEASE_DAMNIT_VERSION: {{version}}
  script:
    - apk add --no-cache git curl
    - ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
    - curl -fsSL -o /usr/local/bin/release-damnit "https://github.com/dsswift/release-damnit/releases/download/${RELEASE_DAMNIT_VERSION}/release-damnit-linux-${ARCH}"
    - chmod +x /usr/local/bin/release-damnit
    - git config user.name release-damnit
    - git config user.email "release-damnit@${CI_SERVER_HOST}"
    - git remote set-url origin "$(echo "$CI_REPOSITORY_URL" | sed "s|//[^@]*@|//release-damnit:${GITLAB_RELEASE_TOKEN}@|")"
    # The branch tip, not the pushed commit: a run queued behind another
    # release must see that release's version commit and tags.
    - git fetch --tags --force origin "$CI_DEFAULT_BRANCH"
    - git checkout -B "$CI_DEFAULT_BRANCH" "origin/$CI_DEFAULT_BRANCH"
    - release-damnit --since-last-release --report-file release-report.json
    # A [skip ci] marker would skip the tag pipeline too; the push option skips
    # only the version commit's own push pipeline.
    - release-damnit publish --report-file release-report.json --branch "$CI_DEFAULT_BRANCH" --provider gitlab --skip-ci push-option
`

// gitlabBuild is the starting point for the repo's own build.
const gitlabBuild = `# The repo's own build. It runs in the tag pipeline each release starts, and
# $CI_COMMIT_TAG is the release tag (v1.2.3). Replace the script with the real
# build and publish steps.

build:
  stage: build
  image: alpine:3.22
  rules:
    - if: $CI_COMMIT_TAG =~ /^v\d+\.\d+\.\d+$/
  script:
    - echo "Build and publish $CI_COMMIT_TAG here."
`

// GitLabCI returns the GitLab pipeline that downloads release-damnit at version.
func GitLabCI(version string) string {
	return strings.Replace(gitlabCI, "{{version}}", version, 1)
}

// GitLabBuild returns the starting build file.
func GitLabBuild() string { return gitlabBuild }
