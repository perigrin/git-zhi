# Infrastructure Validation Report

Assessed: 2026-03-19
Components: `perl:detect-version`, `perl:require-toolchain`
Context: v0.3.2 Milestone 3 — validation against real repos

## detect-version Validation

### Chalk (5.42.0)

- `.perl-version`: 5.42.0 — detected at cascade step 1
- `use VERSION`: `use 5.42.0;` in all .pm files — confirms step 1
- Expected dispatch: `perl:write-5.42` — CORRECT
- No version conflict (no cpanfile)

### Blawd (legacy)

- `.perl-version`: not present — step 1 skipped
- Backend: no CLAUDE.md — step 2 skipped
- `use VERSION`: `use 5.010` and `use 5.10.0` found in lib/ — step 3 finds 5.10
- Expected dispatch: `perl:write-toolchain` (< 5.36) — CORRECT
- The cascade works for legacy repos: step 3 catches what steps 1-2 miss

### Registry (version conflict)

- `.perl-version`: 5.42.0 — detected at cascade step 1
- cpanfile: `requires 'perl' => 'v5.40.2'`
- Version conflict: 5.42.0 > 5.40.2 — conflict resolution triggers
- Expected: warn user, default to 5.40.2 compatibility target — CORRECT

### Iterum (5.40.0)

- `.perl-version`: 5.40.0 — detected at cascade step 1
- Expected dispatch: `perl:write-5.40` — CORRECT

## require-toolchain Validation

None of the four validation repos have `<!-- Backend: ... -->` in their
CLAUDE.md (or have no CLAUDE.md at all). This means require-toolchain
would fail on all four repos with "Run `perl:setup` to configure one."

This is correct behavior — the repos haven't been set up with the plugin
yet. require-toolchain's job is to gate, not to auto-detect. `perl:setup`
handles detection and configuration.

## Observations

- Component: perl:detect-version
- Verdict: WORKS
- Notes: Cascade correctly handles all four repo shapes: explicit version
  (Chalk, Iterum), legacy with use VERSION (Blawd), version conflict
  (Registry). The conflict resolution path addresses the HIGH finding
  from M1 assess.

- Component: perl:require-toolchain
- Verdict: WORKS
- Notes: Correctly fails with actionable message on unconfigured repos.
  No false positives.

## Fixes Applied

None needed.
