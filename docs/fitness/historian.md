# Historian Fitness Report

Assessed: 2026-03-19
Component: `git-zhi-historian`
Context: v0.3.2 Milestone 1 — Historian on Real Repos

## Verdict: WORKS

The historian completed on all four validation repos with 100% commit
coverage and no errors. The calibrated thresholds (join 0.25, coherence
0.20) and conventional commit scope extraction produced strong clustering
across four distinct history shapes.

## Per-Repo Results

| Repo | Commits | Issues | Ratio | Single | Tracker | Cluster | Low-coherence | % low |
|------|---------|--------|-------|--------|---------|---------|---------------|-------|
| Chalk | 1367 | 69 | 20:1 | 6 | 57 | 6 | 6 | 8.7% |
| Iterum | 22 | 4 | 5.5:1 | 1 | 1 | 2 | 1 | 25% |
| Blawd | 136 | 21 | 6.5:1 | 9 | 0 | 12 | 9 | 42.9% |
| Registry | 353 | 42 | 8.4:1 | 8 | 21 | 13 | 8 | 19% |

### Chalk (1367 commits → 69 issues)

Best results of the four. GitHub issue refs (`#18`, `#518`, `#560` etc.)
grouped 57 issues via tracker-match — the dominant signal. Only 6
single-commit orphans (0.4% of commits). Conventional commit scope
extraction did not contribute significantly since Chalk's commit messages
use bracketed tags (`[fix]`, `[FEAT]`) rather than `type(scope):` format.

The 20:1 compression ratio reflects dense, well-referenced history where
most commits carry issue numbers.

### Iterum (22 commits → 4 issues)

Small repo, reasonable results. 22 commits compressed to 4 issues. One
tracker-match from a GitHub issue ref, two clusters from path/time
proximity, one single-commit orphan. The low-coherence rate (25%) is
expected for a 4-issue set — one orphan out of four is statistically
noisy, not a quality signal.

### Blawd (136 commits → 21 issues) — Stress Case

The predicted stress case confirmed. Blawd has no ticket references in
commits (zero tracker-match). All clustering relied on secondary signals:
path overlap, author match, time proximity, and message tokens.

The historian produced 12 clusters and 9 single-commit orphans. The 42.9%
low-coherence rate is the highest of the four repos but is expected —
without ticket refs, the clustering algorithm has less to work with.

Notably, Blawd still achieved 100% coverage (zero unmapped commits). The
lowered join threshold (0.25) was essential — at the original 0.35, most
of these commits would have been orphaned.

This repo demonstrates the historian's floor: usable but noisy results
when no ticket refs exist. Interactive triage (`historian triage`) would
improve quality by letting a human merge or reassign the low-confidence
clusters.

### Registry (353 commits → 42 issues)

Good mix of signals. 21 tracker-match issues from GitHub issue refs, 13
clusters from secondary signals, 8 single-commit orphans. The 8.4:1
compression ratio and 19% low-coherence rate are in the healthy range.

## Confidence Score Calibration

The four-tier confidence scoring behaves as designed:

| Tier | Score | Count (total) | Assessment |
|------|-------|---------------|------------|
| tracker-match | 0.90 | 79 | Correct — GitHub issue refs are high-signal |
| ticket-ref | 0.75 | 0 | Not triggered — repos use `#N` not `PROJ-N` |
| cluster | 0.55 | 33 | Reasonable — clusters formed from secondary signals |
| single-commit | 0.30 | 24 | Correct — single commits with no grouping signal |

The ticket-ref tier (0.75) was never triggered because none of the four
repos use Jira-style ticket references (`PROJ-123`). This tier would
activate on repos that mention Jira tickets in commit messages without
having a formal issue ref pattern.

## Conventional Commit Scope Extraction

The v0.3.1 calibration work added conventional commit scope extraction
(`feat(infer):` → `infer` as a ticket ref). This was validated against
PVM (85 commits → 5 issues) during calibration but did not significantly
affect the four validation repos:

- Chalk uses `[fix]:`/`[FEAT]` brackets, not `type(scope):` format
- Blawd uses free-form commit messages
- Registry uses GitHub issue refs (`#N`)
- Iterum has too few commits for the signal to differentiate

The scope extraction remains valuable for repos that use conventional
commits (like PVM and git-zhi itself) but is not the dominant signal for
these four repos.

## What the Historian Cannot Do

1. **Assess cluster semantic quality.** The historian groups commits by
   structural signals (paths, time, refs) but cannot evaluate whether a
   cluster represents a coherent unit of work. A 47-commit cluster touching
   many files might be one feature or three unrelated changes that happened
   to overlap in time and space.

2. **Handle repos with no commit discipline.** If every commit message is
   "fix" or "wip", all signals except path overlap and time proximity are
   zeroed. The historian will produce clusters, but they may not correspond
   to logical work units.

3. **Detect over-clustering.** When the join threshold is too low, unrelated
   commits merge into large clusters. The coherence threshold provides a
   brake, but the historian has no way to evaluate whether a cluster "makes
   sense" without domain knowledge.

## Fixes Applied

None needed. The historian worked as designed across all four repos.

## v0.4 Callout

`git-zhi-anthropologist` would provide cluster semantic analysis that the
historian cannot. Specifically:

- **Blawd:** Would reveal whether the 9 single-commit orphans represent
  genuinely standalone work or missed grouping opportunities
- **Registry:** Would assess whether the 13 clusters correspond to the
  actual feature structure of the Mojolicious app
- **Chalk:** Would evaluate whether the 6 low-coherence issues are noise
  or represent distinct work that happened to lack issue refs

The anthropologist's value is highest for repos with low ticket-ref
coverage — exactly where the historian's confidence is lowest.
