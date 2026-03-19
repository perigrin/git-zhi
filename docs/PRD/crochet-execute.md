# PRD: crochet:execute — SDLC Lifecycle Skill

## Problem

Today, executing a git-zhi chain requires manual orchestration: the developer
picks issues, starts them, writes code, runs reviews, closes issues, and
triggers postmortem. Each step invokes a different skill or tool by hand.
There is no single entry point that drives the full develop-review-close loop.

## Solution

A new crochet skill (`crochet:execute`) that wraps the entire execution phase
of the SDLC lifecycle. Given a milestone, it picks the next ready issue from
the DAG, executes a TDD loop with review gates, closes the issue, and repeats
until the milestone is complete.

## Architecture: Two Nested Loops + Gate Analyst

### Outer Loop (Issue Cycle)

```
while milestone has open issues:
  issue = next ready issue (git zhi chain list --ready --format json | head -1)
  git zhi issue edit <id> --state start

  [inner loop: TDD + code-simplifier]

  git zhi issue edit <id> --state done
  sanbao snapshot → gate analyst → TIER_1 or TIER_2
  PAAD review (tiered by gate analyst)
  if PAAD finds in-scope debt (and reopens < 3):
    append findings to issue body as ### Review Findings
    git zhi issue edit <id> --state reopen
    git zhi issue edit <id> --state start
    continue                                 # back to inner loop
  else if reopens >= 3:
    stop, report: "issue exceeded max review passes"
  else:
    out-of-scope findings → new issues
    continue to next issue

milestone complete:
  git zhi milestone edit <name> --state complete
  crochet:postmortem <name>
```

### Inner Loop (TDD Cycle)

Implemented as a Ralph Loop with a completion promise. The prompt for each
iteration is the issue's acceptance criteria plus TDD instructions:

```
/ralph-loop "<prompt>" --completion-promise "ISSUE_COMPLETE" --max-iterations 10
```

Each iteration:
1. Read the issue (`git zhi issue show <id>`) — includes review findings if reopened
2. Check git log and git diff for previous iteration's work
3. If review findings exist, address those first
4. Write a failing test (or continue from where last iteration left off)
5. Implement until tests pass
6. Run `code-simplifier` on the diff (refactor gate)
7. If simplifier finds issues → fix them, re-run tests
8. Commit with a descriptive message
9. If all AC met and clean → output completion promise

**Commit strategy:** Commit frequently with descriptive messages. Every iteration
should leave committed state so the next iteration can build on it. Never squash
or amend — the commit history is the iteration history.

### Sanbao Gate Analyst

After the inner loop completes and the issue is marked done, compute a sanbao
snapshot and run a gate analysis agent to determine review depth.

Sanbao operates at milestone scope — run `git-zhi-sanbao <milestone> --format json`
and extract the target issue's metrics from the per-issue `difficulties[]` array
and the `complexity` domain. No `--issue` filter exists; the full milestone
report is computed and the single issue's data is extracted. On small milestones
(v0.3.x scale) this cost is negligible.

**Sanbao per-issue metrics used:**
- **Difficulty score** — composite of MPG, cycle time, reopens, sentiment,
  session count. Normalized to [0,1]. Already computed by sanbao's difficulty
  domain.
- **Hotspot count** — files touched by this issue ranked by churn x size.
- **Change coupling** — unexpected co-change file pairs.
- **MPG** — commits per issue.

**Gate analyst thresholds:**

| Metric | Tier 1 (alignment only) | Tier 2 (full PAAD) |
|---|---|---|
| Difficulty score | < 0.4 | >= 0.4 |
| Hotspot files | <= 2 | > 2 |
| Change coupling | 0 unexpected | any unexpected |
| MPG | <= 3 | > 3 |

Any single tier-2 trigger escalates. The analyst provides a one-sentence
rationale so the decision is auditable in the report.

### Review Gates

**Refactor Gate (inner loop):** `code-simplifier` on the diff. Fast, focused
on clarity and reuse. The inner loop's fixed-point: tests pass AND simplifier
produces zero findings.

**Issue Gate (outer loop):** Tiered PAAD review, depth determined by the
sanbao gate analyst.

*Tier 1 — low complexity:*
- `paad:alignment` — does implementation match the issue's AC?

*Tier 2 — high complexity:*
- `paad:alignment` — does implementation match the issue's AC?
- `paad:agentic-architecture` — structural choices fit codebase patterns?
- `paad:agentic-review` — technical debt, security, test coverage gaps?

If PAAD finds in-scope issues, findings are appended to the issue body (as a
`### Review Findings` section) before reopening. The reopen requires two state
transitions: `--state reopen` (done → reopened) then `--state start`
(reopened → in-progress). The next Ralph Loop iteration reads the issue and
sees the findings — no separate feedback channel needed.

**Max reopens per issue: 3.** If an issue exceeds 3 PAAD-reopen cycles, the
skill stops and reports the problem. This prevents infinite outer-loop cycling
when PAAD keeps finding issues.

Out-of-scope findings become new git-zhi issues in the same milestone.

## Skill Structure

```
crochet/
  skills/
    execute/
      execute.md          # main skill
      inner-prompt.md     # template for Ralph Loop prompt
  commands/
    execute.md            # command wrapper
```

## Acceptance Criteria

1. `crochet:execute <milestone>` picks the next ready issue via `git zhi chain list --ready` and starts execution
2. Inner TDD loop uses Ralph Loop with `--completion-promise` and `--max-iterations`
3. `code-simplifier` runs after each green-test cycle
4. Sanbao gate analyst extracts per-issue metrics from milestone report and determines review tier
5. Tier 1 issues get `paad:alignment` only
6. Tier 2 issues get full PAAD suite (alignment + architecture + agentic-review)
7. In-scope findings are appended to issue body; issue is reopened via two-step state transition (reopen then start)
8. Out-of-scope findings create new issues
9. Max 3 PAAD-reopen cycles per issue before the skill stops and reports
10. Milestone completion triggers `crochet:postmortem`
11. The skill is idempotent — can be re-invoked to resume a partially-executed milestone
12. Each inner-loop iteration commits its state (no lost work on context limit)
13. Report includes gate analysis summary (tier counts, avg difficulty)

## Process AC

- [ ] Skill exercised on at least one real milestone (not just unit test)
- [ ] Ralph Loop integration confirmed (completion promise terminates correctly)
- [ ] PAAD gate confirmed (at least one finding round-trips through reopen)
- [ ] Gate analyst confirmed (tier 2 triggers on measured high-complexity issue)
- [ ] Sanbao per-issue snapshot confirmed (difficulty score extracted from milestone report)

## Decisions

1. **Max iterations:** Fixed default (10). This is a safety valve, not a planning
   tool. If the loop hits max without converging, the issue is too big or the AC
   is ambiguous — the skill stops and reports the problem. No per-issue tuning.
2. **Human-in-the-loop checkpoints:** Default pauses between issues for confirmation.
   Pass `--auto` to run without pauses.
3. **Partial execution:** The chain itself is the recovery mechanism. Re-invoking
   the skill on a partially-executed milestone resumes from the current state
   (already-closed issues are skipped). Ralph Loop handles inner-loop state via
   git commits.
4. **PAAD tiering:** Determined by sanbao gate analyst using measured per-issue
   complexity metrics (difficulty score, hotspots, coupling, MPG). No hardcoded
   heuristics — the analyst reads the data and decides.
5. **Findings feedback:** PAAD findings are written into the issue body, not
   passed out-of-band. The Ralph Loop reads the issue, sees the findings.
   Single source of truth.
6. **Commit strategy:** Commit frequently, never squash. The iteration history
   is the commit history. Clean-up rebase is a separate, optional step outside
   the skill.
7. **Sanbao scope:** Sanbao runs at milestone level (no `--issue` filter exists).
   Gate analyst extracts single-issue metrics from the full report. Acceptable
   cost at v0.3.x scale; `--issue` filter is a future optimization.
8. **Max reopens:** 3 PAAD-reopen cycles per issue. Prevents infinite outer-loop
   cycling. If exceeded, the skill stops and surfaces the problem.

## Dependencies

- Ralph Loop plugin (installed)
- PAAD plugin (installed)
- code-simplifier (superpowers, installed)
- sanbao (git-zhi companion binary)
- crochet:postmortem (exists)
- git-zhi (exists)

## Versioning

Target: v0.3.4
