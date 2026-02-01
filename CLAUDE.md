# release-damnit

## Project Overview

A drop-in replacement for Release Please that correctly traverses merge commits.

Release Please has a fundamental bug (#1533): GitHub's API returns commits in chronological order, not topological order. When you merge a feature branch to main with `--no-ff`, RP only sees the merge commit tip, missing all the commits that came in through the merge.

release-damnit uses your existing Release Please configuration but reads git history directly using `git log` instead of relying on GitHub's broken API.

## Key Behaviors

- Reads `release-please-config.json` and `release-please-manifest.json` unchanged
- Updates VERSION files and CHANGELOG.md in the same format
- Creates GitHub releases with identical naming (`{component}-v{version}`)
- Correctly traverses merge commits to find all conventional commits

## Testing

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