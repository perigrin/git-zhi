# crochet:refinement As-Produced Record

Recorded: 2026-03-19
Input: docs/PRD/pvm-plugin-prd.md
Target repo: ~/dev/perl-development-plugin

## Chain Summary

| Milestone | Issues | Scope |
|---|---|---|
| plugin-infrastructure | 4 | Backend skills, require-toolchain, detect-version |
| plugin-writing-skills | 6 | 5 version-specific write skills + write-perl command |
| plugin-quality-skills | 7 | test, test-mojo, debug, manage-deps, review, regression-test + commands |
| plugin-assembly | 3 | setup orchestrator, README, final verification |
| **Total** | **20** | |

## Milestone: plugin-infrastructure (4 issues)

1. Build perl:using-pvm skill — PVM install procedure and capability map
2. Build perl:using-custom skill — manual capability table
3. Build perl:require-toolchain skill — shared backend dependency gate
4. Build perl:detect-version skill — version detection from multiple sources

Chain: sequential (1 → 2 → 3 → 4)

## Milestone: plugin-writing-skills (6 issues)

1. Build perl:write-5.42 skill — modern Perl 5.42 code generation
2. Build perl:write-5.40 skill — Perl 5.40 code generation
3. Build perl:write-5.38 skill — Perl 5.38 code generation
4. Build perl:write-5.36 skill — Perl 5.36 code generation
5. Build perl:write-toolchain skill — broadly compatible Perl 5.20+
6. Build write-perl.md command — version detection and dispatch

Chain: sequential (1 → 2 → 3 → 4 → 5 → 6)

## Milestone: plugin-quality-skills (7 issues)

1. Build perl:test skill — Test2::V0 testing patterns
2. Build perl:test-mojolicious skill — Test::Mojo patterns
3. Build perl:debug skill — systematic Perl debugging
4. Build perl:manage-deps skill — CPAN dependency management
5. Build perl:review skill — static analysis and code review
6. Build perl:regression-test skill — version matrix testing
7. Build remaining commands — test, debug, deps, review, regression

Chain: sequential (1 → 2 → 3 → 4 → 5 → 6 → 7)

## Milestone: plugin-assembly (3 issues)

1. Build perl:setup skill — project scaffolding orchestrator
2. Write README.md from PRD overview
3. Verify plugin.json and file structure against PRD

Chain: sequential (1 → 2 → 3)

## Dependency Structure Notes

- All chains are sequential within milestones (batch add default)
- No cross-milestone dependencies expressed in zhi (milestones are implicitly ordered)
- Infrastructure must complete before writing skills can be validated
- Writing and quality skills are independent of each other (could be parallel)
- Assembly depends on all skills being complete

## Observations

- Component: crochet:refinement
- Verdict: (to be assessed in Issue 2.2)
- Notes: 20 issues across 4 milestones. PRD has 16 skills + 8 commands = 24 deliverables. The 20-issue count consolidates commands into 2 batch issues (write-perl command with writing skills, remaining 6 commands as one issue). Each skill issue is session-sized (~30 min each for skill file authoring).
