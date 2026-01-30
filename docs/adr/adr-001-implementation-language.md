# ADR-001: Implementation Language

## Status

Accepted

## Context

We need to choose a programming language for release-damnit. The tool must:

1. **Be bulletproof** - This is release infrastructure. Failures are costly.
2. **Distribute as single binary** - No runtime dependencies, easy CI integration.
3. **Cross-compile** - Run on Linux (CI), macOS (dev), Windows (optional).
4. **Have good testing** - Built-in test framework with coverage.
5. **Handle errors explicitly** - Every failure case must be visible and handled.
6. **Work well with git** - Either shell out or use a library.

### Options Considered

| Language | Binary | Cross-compile | Error handling | Testing | DevOps ecosystem |
|----------|--------|---------------|----------------|---------|------------------|
| **Go** | Single | Built-in | Explicit (result, error) | Built-in | Excellent |
| **Rust** | Single | Possible | Explicit (Result<T,E>) | Built-in | Good |
| **C#** | AOT possible | Complex | Exceptions (implicit) | xUnit/NUnit | Good |
| **Python** | PyInstaller | Complex | Exceptions (implicit) | pytest | Good |
| **TypeScript** | Pkg/Deno | Complex | Exceptions (implicit) | Jest/Vitest | Good |
| **Bash** | N/A | N/A | Exit codes | bats | Excellent |

### Analysis

**Go:**
- Every function returns `(result, error)`. Must handle explicitly. No hidden exceptions.
- `go build` produces one file. No runtime, no dependencies.
- `GOOS=linux/darwin/windows` cross-compilation built-in.
- `go test` with table-driven tests, built-in coverage.
- gh CLI, Terraform, kubectl are all Go. Familiar patterns in DevOps.
- Shell out to git (simple) or use go-git library (more control).

**Rust:**
- Excellent error handling with `Result<T, E>` and `?` operator.
- Single binary, excellent performance.
- Memory safety guarantees don't add much value for a CLI tool.
- Steeper learning curve, slower development velocity.
- Overkill for this use case (not a long-running process).

**C#:**
- AOT compilation possible but complex for cross-platform.
- Exception-based error handling is implicit - errors can throw anywhere.
- Good ecosystem but less common for DevOps tooling.
- Would require .NET SDK or runtime on CI runners.

**Python/TypeScript:**
- Both require runtime or complex bundling.
- Exception-based error handling is implicit.
- Dynamically typed (or optional typing) - less safety.
- Good for prototyping, not ideal for "bulletproof" requirement.

**Bash:**
- Works well for simple scripts but error handling is painful.
- Complex logic becomes unmaintainable.
- Testing is possible but awkward.
- Good for glue code, not for application logic.

## Decision

We will implement release-damnit in **Go**.

### Rationale

1. **Explicit error handling** - Every function call forces consideration of the error case. No hidden exceptions that might bubble up unexpectedly.

2. **Single binary distribution** - `go build` produces one executable. Add it to CI, done. No runtime installation, no dependency management.

3. **Cross-platform by default** - Set `GOOS` and `GOARCH` environment variables, rebuild. Works.

4. **Testing is first-class** - `go test ./...` runs all tests. Coverage built-in. Table-driven tests are idiomatic and readable.

5. **Ecosystem alignment** - gh CLI, Terraform, kubectl, Docker, and many DevOps tools are written in Go. The patterns and idioms are familiar territory.

6. **Right-sized complexity** - Rust would be overkill (we don't need memory safety for a CLI tool that runs and exits). Python/TypeScript would under-deliver on the "bulletproof" requirement.

## Consequences

**Positive:**
- Errors cannot be ignored without explicit `_` assignment
- Easy to distribute and integrate
- Fast compilation, fast execution
- Strong standard library for file I/O, process execution
- go-git library available if we want to avoid shelling out

**Negative:**
- Verbose compared to Python/TypeScript for simple operations
- No generics until recently (Go 1.18+), some patterns are repetitive
- Error handling, while explicit, can feel boilerplate-heavy
- Team needs Go familiarity (though it's easy to learn)

**Neutral:**
- Need to decide: shell out to git vs use go-git library (separate decision)
