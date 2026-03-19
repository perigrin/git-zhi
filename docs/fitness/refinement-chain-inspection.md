# Chain Inspection Notes — perl-development-plugin

Inspected: 2026-03-19
Chain: 20 issues across 4 milestones in ~/dev/perl-development-plugin

## Sizing Assessment

All 20 issues are session-sized (~30 min each for skill file authoring).
Each issue produces one skill file or a small batch of related command files.
No issue is too large to complete in a single focused session.

One possible exception: "Build remaining commands" bundles 6 command wrappers
into 1 issue. Acceptable since each is a thin wrapper, but means 6 files
share one set of acceptance criteria.

## Acceptance Criteria Assessment

20/20 issues have explicit acceptance criteria with checkboxes.
Body lengths range from 271 to 819 characters — no stubs.
Criteria are verifiable (file exists, content matches PRD spec).

## Dependency Structure Assessment

### Problem: Over-sequential within milestones

Batch-add chains issues sequentially, but most are independent:

- **Writing skills:** write-5.42, write-5.40, write-5.38, write-5.36,
  write-toolchain are independent skill files. Only write-perl.md command
  depends on all 5 existing (it dispatches to them).

- **Quality skills:** test, test-mojo, debug, manage-deps, review,
  regression-test are all independent. Only the "remaining commands"
  issue depends on the skills existing.

- **Infrastructure:** using-pvm and using-custom are independent backends.
  require-toolchain depends on both. detect-version depends on
  require-toolchain.

### Problem: No cross-milestone dependencies

Infrastructure must complete before writing/quality skills can be validated
(they reference require-toolchain), but zhi doesn't enforce this ordering.

### Problem: setup orchestrator placement

perl:setup in plugin-assembly is correct (last milestone), but has no
explicit dependency on the skills it dispatches to. Implicit ordering
via milestone sequence is sufficient for human execution but not
machine-verifiable.

## Corrections Needed

1. Relax false sequential dependencies within milestones (parallelize
   independent skills)
2. Add cross-milestone dependency: writing/quality first issues should
   depend on detect-version (last infrastructure issue)
3. Consider splitting "Build remaining commands" into individual issues
   (LOW priority — acceptable as-is)

## Verdict

Chain is good enough to execute with minor corrections. Sizing and AC
quality are solid. Main issue is false sequential deps from batch-add.
