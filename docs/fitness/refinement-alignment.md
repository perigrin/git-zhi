# crochet:alignment — Refinement vs Plugin PRD

Assessed: 2026-03-19
Input: refinement chain (20 issues in perl-development-plugin)
Reference: docs/PRD/pvm-plugin-prd.md

## Verdict: WORKS

Alignment found 1 HIGH finding, 3 MEDIUM findings, and 1 MEDIUM structural
issue. Coverage is 100% for skills and commands. The HIGH finding is a real
gap — the PRD explicitly defines marketplace registration but no chain issue
covers it.

## Coverage: All 24 PRD deliverables mapped

All 16 skills and 8 commands are covered by chain issues. No orphan issues
exist (every issue maps to a PRD section). Commands are consolidated:
setup.md bundled with setup skill, write-perl.md separate, 6 commands batched.

## Findings

### HIGH

**Marketplace entry missing.** PRD has a "Marketplace Entry" section with
specific JSON and `/install-plugin perl-development@perigrin-marketplace`
command. No chain issue covers creating or publishing the marketplace
registration.

### MEDIUM

**Self-announcing not in all skill AC.** PRD design principle 4 says "Skills
are self-announcing." Only the write-5.42 issue explicitly mentions the
announce text in AC. Other skill issues should require the self-announcement
line.

**Version conflict resolution gap.** M1 assess found `.perl-version` vs
cpanfile conflict in Registry. The detect-version issue AC says "Unknown
version falls through to ask user" but doesn't address the conflict case
where both sources disagree.

**dist.ini dependency management gap.** M1 assess found manage-deps assumes
cpanfile but Blawd uses Dist::Zilla dist.ini. The manage-deps issue AC
doesn't address non-cpanfile projects.

### MEDIUM (structural)

**False sequential dependencies.** Writing skills 1-5 and quality skills
1-6 are independent but chained sequentially (batch-add artifact).

### LOW

**No validation issues in plugin chain.** Validation against real repos is
handled by the fitness assessment milestones in git-chain, not the plugin
repo. Correct architecture but worth noting.

## Correction Summary

| Finding | Severity | Action |
|---|---|---|
| Marketplace entry | HIGH | Add issue to plugin-assembly milestone |
| Self-announcing AC | MEDIUM | Add announce text to all skill issue AC |
| Version conflict | MEDIUM | Update detect-version issue AC |
| dist.ini gap | MEDIUM | Update manage-deps issue AC |
| False sequential deps | MEDIUM | Relax dependencies within milestones |
