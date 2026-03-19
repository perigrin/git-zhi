# Refinement Fitness Report

Assessed: 2026-03-19
Components: `crochet:refinement`, `crochet:alignment`
Context: v0.3.2 Milestone 2 — PRD to Chain

## Verdict: WORKS (both components)

Refinement produced a well-scoped, session-sized chain from the plugin PRD.
Alignment caught real gaps. The correction delta (52%) reflects AC
completeness issues and tooling artifacts, not fundamental scope errors.

## Refinement Quality

**Input:** 984-line PRD with 16 skills, 8 commands, and 6 supporting sections.

**Output:** 20 issues across 4 milestones, all with acceptance criteria.

| Metric | Value | Assessment |
|---|---|---|
| Issues produced | 20 | Reasonable for 24 deliverables |
| Issues with AC | 20/20 (100%) | All had checkboxes |
| Session-sized | 20/20 | Each is a single skill file or small batch |
| Correct scope | 19/20 | Marketplace entry missed (caught by alignment) |

**Sizing quality:** Excellent. Every issue produces one tangible artifact
(a skill file or command file). No issue is too large for a single session.
No over-decomposition — commands are batched where appropriate.

**Dependency structure (as-produced):** Poor. Batch-add created false
sequential dependencies within milestones. All 5 writing skills chained
serially despite being independent. Same for 6 quality skills. No
cross-milestone dependencies expressed.

**Dependency structure (after correction):** Good. Independent skills
parallelized, fan-in to command/orchestrator issues, cross-milestone
ordering via marketplace → verify dependency.

## Alignment Quality

| Finding | Severity | True positive? |
|---|---|---|
| Marketplace entry missing | HIGH | Yes — PRD section with no chain issue |
| Self-announcing AC incomplete | MEDIUM | Yes — PRD principle not propagated |
| Version conflict resolution | MEDIUM | Yes — surfaced by M1 assess |
| dist.ini gap | MEDIUM | Yes — surfaced by M1 assess |
| False sequential deps | MEDIUM | Yes — structural tooling artifact |

**True positive rate:** 5/5 (100%). All findings were actionable. No false
positives.

**Cross-document integration:** Two of five findings (version conflict,
dist.ini) came from M1 assess, not from the PRD itself. Alignment correctly
incorporated prior fitness observations. This is the strongest signal that
the multi-milestone assessment protocol works as designed.

## Correction Delta

| Metric | Count |
|---|---|
| Issues added | 1 (marketplace) |
| Issues with AC updated | 10 |
| Dependencies removed | 10 (false sequential) |
| Dependencies added | 12 (fan-in) |
| **Total issues changed** | **11 of 21 (52%)** |

The 52% rate breaks down as:
- **5% scope correction** (1 issue added of 21)
- **38% AC quality improvement** (8 self-announcing + 2 gap fixes)
- **48% structural correction** (dependency relaxation)

The scope was 95% correct as-produced. The corrections were quality
improvements, not scope changes.

## Systematic Issues

1. **Batch-add creates false deps.** This is a git-zhi tooling limitation,
   not a refinement quality issue. Every batch-add chain will need
   dependency correction. Consider adding a `--parallel` flag to
   `git zhi issue add` that creates issues without chaining them.

2. **PRD principles don't auto-propagate to AC.** "Skills are self-announcing"
   is a design principle, but refinement didn't translate it into per-issue
   AC checkboxes. The refinement process should include a step that scans
   PRD design principles and adds them as AC items to relevant issues.

3. **Prior fitness findings need explicit feed-in.** The version conflict
   and dist.ini gaps were caught because alignment had access to the M1
   assess report. Without that context, alignment would have missed them.
   The multi-milestone protocol provides this context naturally.

## v0.4 Callouts

- `git zhi issue add --parallel` to avoid false sequential deps
- Refinement should auto-scan PRD design principles section and propagate
  to issue AC
- Alignment should auto-diff PRD markdown headers against issue titles
  for mechanical coverage checking
