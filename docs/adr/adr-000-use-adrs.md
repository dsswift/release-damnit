# ADR-000: Use Architecture Decision Records

## Status

Accepted

## Context

This project needs to document significant architectural and design decisions in a way that:
- Provides context for future maintainers
- Explains the reasoning behind choices
- Records alternatives that were considered
- Tracks when decisions were made and why

## Decision

We will use Architecture Decision Records (ADRs) to document significant decisions.

### Format

Each ADR will be a Markdown file in `docs/adr/` with the following structure:

```markdown
# ADR-NNN: Title

## Status
[Proposed | Accepted | Deprecated | Superseded by ADR-XXX]

## Context
[What is the issue that we're seeing that is motivating this decision?]

## Decision
[What is the change that we're proposing and/or doing?]

## Consequences
[What becomes easier or more difficult to do because of this change?]
```

### Naming

Files are named `adr-NNN-short-title.md` where NNN is a zero-padded sequence number.

### When to Write an ADR

Write an ADR when:
- Choosing between competing technologies or approaches
- Making a decision that would be costly to reverse
- Making a decision that affects the overall architecture
- A decision requires explanation beyond "it was obvious"

## Consequences

**Positive:**
- Decisions are documented with context
- Future maintainers can understand why things are the way they are
- Avoids re-litigating settled decisions
- Forces clarity of thought when making decisions

**Negative:**
- Adds overhead to decision-making process
- ADRs can become stale if not maintained
- Requires discipline to actually write them
