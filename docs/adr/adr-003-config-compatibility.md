# ADR-003: Config Compatibility with Release Please

## Status

Accepted

## Context

release-damnit is designed as a drop-in replacement for Release Please. Teams using Release Please have existing:
- `release-please-config.json` - Package definitions, changelog paths, plugins
- `release-please-manifest.json` - Current versions for each package
- `*/VERSION` files - Version numbers in packages
- `*/CHANGELOG.md` files - Changelog entries

Requiring migration would create friction and risk.

## Decision

We will use Release Please configuration files unchanged.

### Files We Read

| File | How We Use It |
|------|---------------|
| `release-please-config.json` | Package paths, components, changelog paths, linked-versions plugin |
| `release-please-manifest.json` | Current versions (we also update this) |

### Files We Update

| File | Format |
|------|--------|
| `release-please-manifest.json` | JSON with package paths as keys, versions as values |
| `*/VERSION` | Plain text: `0.1.119 # x-release-please-version` |
| `*/CHANGELOG.md` | Conventional changelog format (same as RP) |

### Config Schema Support

We support the subset of `release-please-config.json` that matters for versioning:

```json
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "packages": {
    "workloads/jarvis": {
      "component": "jarvis",
      "changelog-path": "CHANGELOG.md"
    }
  },
  "plugins": [
    {
      "type": "linked-versions",
      "groupName": "ma-observe",
      "components": ["ma-observe-client", "ma-observe-server"]
    }
  ]
}
```

**Supported fields:**
- `packages` - Required. Map of path to package config.
- `packages.*.component` - Package name for releases.
- `packages.*.changelog-path` - Where to write changelog (relative to package path).
- `plugins` - Optional. Array of plugins.
- `plugins.*.type: "linked-versions"` - Link versions across packages.
- `plugins.*.components` - Which packages are linked.

**Ignored fields:**
- `release-type` - We only support "simple" (VERSION file) style.
- `bump-minor-pre-major` - We always treat pre-1.0 as "patch on feat".
- `versioning` - We only support semver.
- Other RP-specific options we don't need.

### Manifest Schema

We read and write `release-please-manifest.json`:

```json
{
  "workloads/jarvis": "0.1.119",
  "workloads/jarvis/clients/web": "0.1.10"
}
```

This file is the source of truth for current versions. We update it atomically with VERSION files.

### VERSION File Format

```
0.1.119 # x-release-please-version
```

The comment marker is optional but we preserve it if present. We only update the version number.

### CHANGELOG Format

We generate changelog entries matching Release Please's conventional-changelog format:

```markdown
## [0.1.120](https://github.com/owner/repo/compare/jarvis-v0.1.119...jarvis-v0.1.120) (2024-01-15)

### Features

* add calendar sync ([abc1234](https://github.com/owner/repo/commit/abc1234))

### Bug Fixes

* fix notification timing ([def5678](https://github.com/owner/repo/commit/def5678))
```

## Consequences

**Positive:**
- Zero migration effort for existing users
- Can switch back to Release Please if needed
- Existing tooling (like RP's changelog viewer) continues to work
- Teams don't need to learn new configuration

**Negative:**
- Bound to RP's configuration schema decisions
- Can't add features that don't fit RP's model
- Must track RP schema changes (though unlikely to change much)

**Neutral:**
- We implement a subset, not full compatibility
- Unsupported options are silently ignored (or warned, TBD)
