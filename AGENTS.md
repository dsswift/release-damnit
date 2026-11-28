# release-damnit

## Project Overview

A drop-in replacement for Release Please that correctly traverses merge commits.

Release Please has a fundamental bug (#1533): GitHub's API returns commits in chronological order, not topological order. When you merge a feature branch to main with `--no-ff`, RP only sees the merge commit tip, missing all the commits that came in through the merge.

release-damnit uses your existing Release Please configuration but reads git history directly using `git log` instead of relying on GitHub's broken API.

## Key Behaviors

- Reads `release-please-config.json` and `release-please-manifest.json` unchanged
- Updates VERSION files and CHANGELOG.md in the same format
- Creates GitHub releases with Release Please's tag naming (`{component}-v{version}`, or `v{version}` with `include-component-in-tag: false`)
- Commands beside the default release run: `check-commit`, `check-commits`, `commit`, `land`, `verify`, `sync`, `hooks`, `publish`, `init` (see README.md)
- `land` runs the config's `check` on exactly the tree that lands and signs a landing proof (a git note under `refs/notes/release-damnit`, SSH signature namespace `release-damnit-landing`), pushed atomically with main; `verify` checks it in the pipeline against the estate's allowed signers (README, "The landing proof", "Verifying")
- Correctly traverses merge commits to find all conventional commits
- Checks every commit, in the hook and on `land`, against the commit rules: a conventional header, a scope that names the one package it touches, and, when the repo's `vantage.yaml` names a `work:` kind, a `Work: <KIND>-<n>` trailer in its last paragraph naming an item of that kind or the `roadmap:` kind (README, "Work Trailer")

## Testing

`make land` runs `release-damnit land`, which runs the config's `check` (`make check`: vet, gofmt, every test) and signs its proof.

```bash
# Unit tests (fast, no git needed)
go test ./internal/... -short

# Integration tests (creates temp git repos)
go test ./internal/... -run Integration

# All tests with coverage
go test ./... -coverprofile=coverage.out
```

## Architecture

Decisions live in the Vantage registry `vantage.yaml` names (`RD`). Read them before changing what they govern; `RD-5004` is the landing proof.

## Install

After a release lands, install it on this machine: `go install github.com/dsswift/release-damnit/cmd/release-damnit@latest`.