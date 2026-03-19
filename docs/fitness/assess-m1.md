# crochet:assess Fitness Report — M1

Assessed: 2026-03-19
Component: `crochet:assess`
Context: v0.3.2 Milestone 1 — Post-historian assessment of four validation repos

## Verdict: WORKS

Assess produced actionable findings for all four repos. The cross-repo
comparison surfaced version conflict and dependency management gaps that
wouldn't have appeared from any single repo. The historian chain context
was most useful for Chalk (highest quality chain) and least useful for
Iterum (too few issues to matter).

## Per-Repo Results

### Chalk

- **Perl version:** 5.42.0 (`.perl-version` present)
- **Build system:** Custom (`./prove`, `perl -Ilib`)
- **OO system:** `feature 'class'` (native 5.42)
- **Chain state:** 69 historian issues, all done

| PRD Requirement | Status | Notes |
|---|---|---|
| `perl:detect-version` → 5.42 | SATISFIED | `.perl-version` = `5.42.0` |
| `perl:write-5.42` dispatch | SATISFIED | CLAUDE.md references this skill |
| `perl:test` (Test2::V0) | GAP | Custom test harness, not `pvx prove` |
| `perl:setup` (PVM detection) | PARTIAL | Uses plenv, not PVM |
| `perl:manage-deps` (cpanfile) | GAP | No cpanfile |
| `perl:regression-test` | BLOCKING | Single unstable-feature version |

**Key finding:** Reference consumer (Chalk) doesn't use reference backend
(PVM). `perl:setup` would fall through to `perl:using-custom`.

### Iterum

- **Perl version:** 5.40.0 (`.perl-version` present)
- **Build system:** Standard (`prove -lr t/`)
- **OO system:** `use experimental qw(class)` (5.40 style)
- **Chain state:** 4 historian issues, all done

| PRD Requirement | Status | Notes |
|---|---|---|
| `perl:detect-version` → 5.40 | SATISFIED | `.perl-version` = `5.40.0` |
| `perl:write-5.40` dispatch | SATISFIED | Correct routing |
| `perl:write-5.40` content | PARTIAL | Uses `use experimental qw(class)` not `no warnings 'experimental::class'` |
| `perl:setup` | GAP | No CLAUDE.md |

**Key finding:** `perl:write-5.40` should document both `no warnings
'experimental::class'` and `use experimental qw(class)` as equivalent forms.

### Blawd

- **Perl version:** Unknown (no `.perl-version`, no `use VERSION`)
- **Build system:** Dist::Zilla (`dist.ini`)
- **OO system:** Custom `Blawd::OO` (Moose/Moo wrapper)
- **Chain state:** 21 historian issues, 42.9% low-coherence

| PRD Requirement | Status | Notes |
|---|---|---|
| `perl:detect-version` | BLOCKING | No signal — falls through to "ask user" |
| `perl:write-toolchain` | EXPECTED | Legacy project → correct target |
| `perl:manage-deps` | GAP | Uses `dist.ini [AutoPrereqs]`, not cpanfile |
| `perl:test` | GAP | Likely Test::More, not Test2::V0 |

**Key finding:** Three simultaneous failures — version detection, dependency
management, and test framework all need fallback paths that the PRD doesn't
fully address. `perl:manage-deps` assumes cpanfile but Blawd uses Dist::Zilla.

### Registry

- **Perl version:** 5.42.0 (`.perl-version`), but cpanfile requires `v5.40.2`
- **Build system:** Carton + Mojolicious
- **OO system:** Mixed (Function::Parameters, Mojo base classes)
- **Chain state:** 42 historian issues, 19% low-coherence
- **CLAUDE.md:** Present, detailed

| PRD Requirement | Status | Notes |
|---|---|---|
| `perl:detect-version` | CONFLICTING | `.perl-version` says 5.42, cpanfile says 5.40.2 |
| `perl:test-mojolicious` | SATISFIED | Mojolicious app with Test::Mojo |
| `perl:manage-deps` | SATISFIED | cpanfile present |
| `perl:regression-test` | SATISFIED | 5.40-5.42 matrix is meaningful |

**Key finding:** `.perl-version` vs cpanfile version conflict.
`perl:detect-version` reads `.perl-version` first and would dispatch to
`perl:write-5.42`, but the project targets 5.40.2. Writing 5.42-specific
code (`:writer`, lexical methods) would break compatibility.

## Findings the Historian Missed

1. **Version detection conflicts** (Registry) — `.perl-version` vs cpanfile
   disagreement requires semantic understanding of Perl toolchain conventions.

2. **Dependency management diversity** — historian can't distinguish cpanfile
   projects from dist.ini projects. Blawd's workflow is fundamentally different.

3. **Test framework heterogeneity** — historian sees test files but not which
   framework (custom harness, Test::More, Test::Mojo) they use.

## PRD Gaps Surfaced

| Gap | Severity | Repos |
|---|---|---|
| Version conflict resolution (`.perl-version` vs cpanfile) | HIGH | Registry |
| `perl:manage-deps` doesn't handle `dist.ini` | HIGH | Blawd |
| Reference consumer doesn't use reference backend | MEDIUM | Chalk |
| `perl:write-5.40` missing `use experimental` form | LOW | Iterum |
| Version detection fallthrough UX for legacy repos | LOW | Blawd |
| Custom test runners not addressed | LOW | Chalk |

## Chain Context Assessment

| Repo | Chain useful? | Why |
|---|---|---|
| Chalk | Yes | 69 issues with GitHub refs — rich narrative |
| Registry | Moderate | 42 issues, 21 tracker-matched |
| Iterum | Minimal | 4 issues — too small |
| Blawd | Limited | 42.9% low-coherence degrades quality |

## v0.4 Callout

Assess could auto-detect version conflicts and dependency file mismatches
without needing a full PRD comparison — useful as a standalone project
health check tool.
