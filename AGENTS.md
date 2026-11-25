# release-damnit

## Project Overview

A drop-in replacement for Release Please that correctly traverses merge commits.

Release Please has a fundamental bug (#1533): GitHub's API returns commits in chronological order, not topological order. When you merge a feature branch to main with `--no-ff`, RP only sees the merge commit tip, missing all the commits that came in through the merge.

release-damnit uses your existing Release Please configuration but reads git history directly using `git log` instead of relying on GitHub's broken API.

## Key Behaviors

- Reads `release-please-config.json` and `release-please-manifest.json` unchanged
- Updates VERSION files and CHANGELOG.md in the same format
- Creates GitHub releases with Release Please's tag naming (`{component}-v{version}`, or `v{version}` with `include-component-in-tag: false`)
- Commands beside the default release run: `check-commit`, `check-commits`, `commit`, `land`, `sync`, `hooks`, `publish`, `init` (see README.md)
- Correctly traverses merge commits to find all conventional commits
- Checks every commit, in the hook and on `land`, against the commit rules: a conventional header, a scope that names the one package it touches, and, when the repo's `vantage.yaml` names a `work:` kind, a `Work: <KIND>-<n>` trailer in its last paragraph naming an item of that kind or the `roadmap:` kind (README, "Work Trailer")

## Testing

`make land` runs `make check` (vet, gofmt, every test) and then `release-damnit land`.

```bash
# Unit tests (fast, no git needed)
go test ./internal/... -short

# Integration tests (creates temp git repos)
go test ./internal/... -run Integration

# All tests with coverage
go test ./... -coverprofile=coverage.out
```

## Architecture

See `docs/adr/` for architectural decisions.