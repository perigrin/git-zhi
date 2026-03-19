# Correction Delta — Refinement Chain

Applied: 2026-03-19

## Changes Applied

### HIGH findings

1. **Added 1 issue:** "Register plugin in marketplace" to plugin-assembly
   milestone, blocked by "Verify plugin.json and file structure." (PRD
   "Marketplace Entry" section had no corresponding chain issue.)

### MEDIUM findings (applied)

2. **Updated detect-version AC:** Added version conflict resolution criterion
   (`.perl-version` vs cpanfile disagreement → warn and ask).

3. **Updated manage-deps AC:** Added dist.ini coexistence documentation
   criterion (detect dist.ini, explain cpanfile compatibility).

4. **Updated 8 skill issue ACs:** Added self-announcing text requirement
   to write-5.40, write-5.38, write-5.36, write-toolchain, test-mojolicious,
   debug, review, regression-test.

### MEDIUM findings (structural)

5. **Relaxed 10 false sequential dependencies:**
   - Infrastructure: using-pvm and using-custom now parallel (both block
     require-toolchain)
   - Writing: 5 write skills now parallel (all block write-perl.md command)
   - Quality: 6 quality skills now parallel (all block remaining commands)

6. **Added 1 dependency:** marketplace issue blocked by verify issue.

### MEDIUM findings (accepted as delta)

None. All MEDIUM findings were applied.

## Delta Measurement

| Metric | Count |
|---|---|
| Issues added | 1 |
| Issues removed | 0 |
| Issues with AC updated | 10 (detect-version, manage-deps, 8 self-announcing) |
| Dependencies removed | 10 (false sequential) |
| Dependencies added | 12 (fan-in to command/verify issues) |
| Issues renamed | 0 |
| Issues re-milestoned | 0 |
| **Total issues changed** | **11 of 21 (52%)** |

## Assessment

52% of issues required correction. The HIGH finding (marketplace entry) was
a real coverage gap. The MEDIUM findings were quality improvements: AC
completeness (self-announcing), PRD alignment (version conflict, dist.ini),
and dependency correctness (false sequential deps).

The refinement produced correct scope and sizing but had three systematic
issues:
1. Batch-add creates false sequential deps (structural limitation of the tool)
2. AC items need explicit PRD cross-reference (self-announcing was in PRD
   but not propagated to all issues)
3. Cross-document findings (M1 assess) need to be fed back into the chain
   (version conflict, dist.ini gaps)
