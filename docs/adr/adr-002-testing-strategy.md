# ADR-002: Testing Strategy

## Status

Accepted

## Context

release-damnit is release infrastructure. Failures in production are costly:
- Wrong versions published
- Missing changelog entries
- Broken builds due to incorrect package detection

We need a testing strategy that provides high confidence with reasonable effort.

## Decision

We will use a three-tier test pyramid plus contract assertions.

### Test Pyramid

```
                    ┌─────────────────┐
                    │   E2E Tests     │  ← Real GitHub repo (mock--gitops-playground)
                    │   (few, slow)   │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │ Integration     │  ← Local git repos (temp dirs with origins)
                    │ Tests           │
                    └────────┬────────┘
                             │
                    ┌────────┴────────┐
                    │ Unit/Simulation │  ← Mocked git output, no actual git calls
                    │ Tests (many)    │
                    └─────────────────┘
```

### Test Categories

| Category | What It Tests | How |
|----------|---------------|-----|
| **Unit tests** | Individual functions (parsing, path matching, version bumping) | Pure Go, mocked inputs |
| **Simulation tests** | Logic flows with mocked git output | Inject fake git responses |
| **Contract tests** | Assertions throughout code that validate assumptions | `require()` functions that panic with context |
| **Integration tests** | Full tool against local git repos | Temp dirs with `git init`, local "origin" repos |
| **E2E tests** | Full tool against real GitHub repo | `mock--gitops-playground` repo |

### Contract Assertions

Sprinkled throughout the code to catch invalid states early:

```go
// In version/bump.go
func BumpVersion(current string, bumpType BumpType) string {
    contracts.Require(current != "", "current version cannot be empty")
    contracts.Require(semver.IsValid("v"+current), "current version must be valid semver: %s", current)
    contracts.RequireOneOf(bumpType, []BumpType{Patch, Minor, Major}, "invalid bump type: %v", bumpType)

    // ... implementation

    contracts.Ensure(result != "", "result cannot be empty")
    contracts.Ensure(semver.IsValid("v"+result), "result must be valid semver: %s", result)
    return result
}
```

Contracts are always active (not compiled out) because this tool runs briefly and correctness matters more than performance.

### Mock Repository

A dedicated GitHub repository (`spraguehouse/mock--gitops-playground`) provides:
- Deterministic structure that can be reset
- Real GitHub API interactions for E2E tests
- Isolated from production repositories

Setup and reset scripts manage this repository's state.

### Test Execution

```bash
# Unit tests only (fast, no git needed)
go test ./internal/... -short

# Integration tests (creates temp git repos)
go test ./internal/... -run Integration

# E2E tests (requires mock--gitops-playground access)
go test ./e2e/... -tags=e2e

# All tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### CI Pipeline

Unit and integration tests run on every push. E2E tests run on PRs and main.

## Consequences

**Positive:**
- High confidence from multiple test layers
- Fast feedback from unit tests
- Contract assertions catch bugs at the point of failure
- E2E tests verify real GitHub integration

**Negative:**
- More test infrastructure to maintain
- Mock repository requires access management
- E2E tests are slower and can be flaky

**Neutral:**
- Balance between test coverage and development velocity
- Contract assertions add some code verbosity
