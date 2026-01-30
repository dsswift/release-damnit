# release-damnit

Check `./todo.md` for the implementation roadmap. Update each item's status:
- `[ ]` pending
- `[~]` in progress
- `[x]` complete

Add brief notes under items ONLY for unexpected issues or deviations.
Do not add notes for successful completions.

If interrupted with urgent work:
1. Note current progress on the active item
2. Clear in-progress marker
3. Insert new item at current position (not at end)
4. Complete inserted item first
5. Resume original sequence

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