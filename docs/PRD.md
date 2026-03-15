---
title: git-chain PRD v3
tags: ["projects", "ideas"]
description: "Product requirements document for git-chain, a local-first git-native dependency-driven development workflow scheduler with telemetry"
---

# git-chain: Product Requirements Document

## Overview

**`git-chain` is a git-native task graph that tells developers — and agents — what to do next.**

`git-chain` is a git subcommand that manages development work as a dependency graph with built-in telemetry. All state lives locally in the git repository as per-entity refs under `refs/chain/`. It requires no external services, works offline, and syncs automatically via standard git push/pull. Invoked as `git chain` (git finds the `git-chain` binary on `$PATH`).

Following git’s own convention, `git-chain` discovers subcommands on `$PATH`: any executable named `git-chain-<n>` can be invoked as `git chain <n>`. This enables third-party plugins, community extensions, and local scripts to extend the tool without modifying the core binary.

This is a standalone Go binary — no runtime dependencies, no GitHub dependency, no API tokens required.

## 60-Second Example

```bash
# Install
go install github.com/perigrin/git-chain@latest

# Create your first issue (lazy-inits everything)
cd your-project
git chain issue add
# $EDITOR opens — write your issue, save, exit.
# Chain state initialized. Default milestone created. Refspecs configured.

# Add a few more issues
git chain issue add
# Write multiple issues separated by --- for batch creation with sequential dependencies

# See the chain
git chain list

# What should I work on?
git chain issue show
# -> Shows the current in-progress issue, or the next issue to work on (HEAD)

# Tag it for easy reference
git chain issue edit 019444a1 --tag lexer

# Start working
git chain issue edit lexer --state start

# Do your work, make commits as normal...

# Finish
git chain issue edit lexer --state done

# Check project health
git chain milestone show HEAD

# Sync with team (just push/pull as normal)
git push
```

An AI coding agent does the same thing:

```bash
git chain issue show --format json
# -> next issue on critical chain with full context

git chain issue edit 019444a1 --state start
# agent works...
git chain issue edit 019444a1 --state done
```

## Problem Statement

Development work forms a dependency graph, not a flat list. Existing issue trackers treat dependencies as informational metadata rather than operational constraints, making it difficult to identify what's actually ready to work on, what's blocking the critical path, or when the project will realistically ship.

Additionally, planning tools rely on manual estimates — developers guess how long tasks will take, then the plan drifts from reality as guesses prove wrong. Nobody goes back to update the estimates because it's busywork.

git-chain takes a different approach:

- **Dependencies are operational, not informational.** The task graph determines what's ready, what's blocked, and what the critical chain is. The scheduler selects work; developers don't have to figure out what to do next.
- **Telemetry replaces estimation.** The tool observes repository activity signals — commit frequency, completion rate, elapsed time — and produces forecasts. Given a deadline, it tells you what scope fits. Without one, it tells you when you'll be done. Nobody enters estimates. Nobody updates estimates. The system watches what actually happens and projects forward.
- **State lives in git.** No external services, no SaaS accounts, no API tokens. Push/pull syncs everything. Works offline. Works for agents.

## Target Users

- **Primary:** Solo developers and small teams (2–3) using git daily
- **Primary:** AI coding agents managing task graphs
- **Secondary:** Small OSS maintainers who want project state in the repo itself
- **Future (with external tracker sync):** Larger teams via Jira/GitHub Issues integration, PM visibility

## Non-Goals

git-chain does not attempt to model:

- Developer resource allocation or team capacity planning (see v0.4 roadmap)
- Calendar scheduling or time-of-day awareness
- Permissions, roles, or access control
- Cross-repository dependencies (future consideration)
- External project management workflows (dashboards, reporting, Gantt charts — see v0.4 roadmap)
- Enforcement of issue sizing — the tool assumes issues are session-sized (see Task Definition) and reflects deviations back to you via telemetry, but it won't reject an issue for being too large or too small

If you need those things today, use Jira or Linear. git-chain is for people who want their task graph in git and their forecasts derived from what actually happened.

## Conceptual Model

git-chain represents work as a dependency DAG stored in git. Issues are nodes. Prerequisites are edges. The scheduler uses this graph to determine what's ready, what's critical, and what to work on next.

### The DAG Scheduler

```
issue graph (DAG)
     ↓
ready set (unblocked issues)
     ↓
critical chain (longest sequential path)
     ↓
next issue (issue on critical chain with most downstream dependencies)
     ↓
HEAD (current or next issue)
```

Position in the chain determines priority. `git chain list --critical` shows the critical chain — the longest sequential path through the dependency graph. Work not on the critical chain can happen in parallel or be cut without affecting the delivery date. The DAG is the real structure; `git chain list` shows a linearized view (topological sort) by default.

### Core Concepts

| Concept         | What It Is                                                                                     |
|-----------------|------------------------------------------------------------------------------------------------|
| **Chain**       | A DAG of issues. The work itself. Default chain is `_`; named chains are a future feature.     |
| **Milestone**   | A segment of the chain + a buffer. A delivery grouping with an optional due date.              |
| **Buffer**      | Calculated reserve measured in sessions. Starts at 50% of issue count; refines dynamically as telemetry data accumulates. Not a phase you enter — you burn buffer or finish with buffer to spare. |
| **MPG**         | Rolling average commits per issue. An activity signal — are issues right-sized?                 |
| **Speed**       | Issues completed per calendar period. A throughput signal — observed, not controlled.           |
| **Tank**        | How much work fits in a milestone. Date-constrained: issues that fit in the timeframe given current speed. Undated: total scope at sustainable pace. |

### Principles

1. **Local-first, git-native.** All state lives in the git repository itself. No external databases, no API dependencies. Works offline. Syncs when you push/pull.
2. **Target dates are constraints, scope is variable.** Projects commit to a date. What ships by that date flexes based on reality.
3. **Thin vertical slices, session-sized.** Every issue represents an independently deployable slice of value, sized to fit within a single focused session under ideal conditions. See Task Definition below for details.
4. **Observe and forecast, don't estimate.** Track activity signals empirically. Use data to forecast, not guesses. Telemetry plus forecasting is not estimation.
5. **No sprints (unless you want them).** There is no built-in sprint concept. If you want sprints, create two-week time-boxed milestones — the tool doesn't care. But it doesn't force that cadence on you either.
6. **Work in progress is limited.** Finish or pause an issue before starting a different one.
7. **Agent-friendly.** Every operation is available via CLI or direct git ref access. No interactive prompts required. Structured context on issues enables agents to begin work immediately.

### Task Definition

An issue in git-chain is defined as a unit of work that can be comfortably completed in a single ideal session of uninterrupted work that stands alone as a deliverable slice of value for the product.

**Ideal session:** A focused block of work with minimal context switches, interruptions, or external dependencies.

**Sizing guidance:**

- If an issue consistently requires multiple sessions, it's too big — use `issue edit --split` to break it into smaller issues.
- If an issue is trivially small, consider grouping it with related work into a single issue. If you discover mid-session that an issue is trivially small, you can merge it with the next issue in its direct dependency chain or with other trivially small issues using `issue edit --merge`.
- The target is one issue = one session. Not every issue will land exactly there, but the closer you get, the better the telemetry works.

**Why it matters:**

- Ensures accurate telemetry — MPG and speed signals stabilize when issues are consistently session-sized.
- Keeps the buffer meaningful — Goldratt's 50% rule assumes comparable work units. Wildly varying issue sizes make buffer calculations noisy.
- Reduces cognitive load — developers and agents know they can start and finish an issue in one session.
- Keeps the product shippable — every completed issue is deployable as-is. You can stop anywhere in the chain and have a working product, not a half-built feature waiting for the next three issues to be useful.

**Relation to telemetry:** The "one session" principle stabilizes measurement windows. Every issue corresponds to one or more measurement windows, but the target is a single session — variance across multiple sessions increases noise and may trigger buffer consumption, signaling that the issue is too large. This is how the system teaches you to size issues: not through rules, but by reflecting the results back to you.

### Critical Chain Scheduling

git-chain adapts the scheduling model from Critical Chain Project Management (CCPM). Tasks are sized assuming ideal conditions. Uncertainty is pooled into a shared milestone buffer, which absorbs variance in execution. git-chain differs by deriving buffer signals from repository telemetry rather than manual estimates.

## Development Telemetry

git-chain treats the repository as a **sensor**. It observes activity signals and produces derived indicators. This is closer to observability than project management — the system instruments what's actually happening rather than tracking what someone planned to happen.

**The core distinction: telemetry + forecasting ≠ estimation.** Nobody enters estimates. Nobody updates estimates. The system watches what actually happens — commits, completions, elapsed time — and projects forward. Forecasts get more accurate as data accumulates. They are never promises.

### Observed Signals

| Signal            | Source                                                              |
|-------------------|---------------------------------------------------------------------|
| **Commits**       | Counted between measurement bookmarks (start/pause/resume/done)     |
| **Elapsed time**  | Calendar time between state transitions                             |
| **Completion**    | Issue state transitions from in-progress to done                    |

### Derived Indicators

**MPG (commits per issue)** — Rolling average of commits across completed issues. Think of it like a car's trip computer: you don't estimate how much gas a trip will take — you drive, and the car tells you your MPG. Over time, you learn your vehicle's efficiency. If your MPG is consistently high (many commits per issue), your issues are too big — slice thinner. If it's consistently low, you're either writing very tight issues or your commits are too coarse.

We assume you have good git-commit hygiene and your commits tell a story about how the issue was resolved through a series of logical changes. If that's not the case then just like your car's trip computer, your mileage may vary. Commit counts are activity signals, not measures of developer effort — and MPG is learning feedback, not performance measurement. A consistently high MPG teaches you to slice issues thinner. The system reflects your patterns back to you so you can adjust.

**Speed (issues per period)** — Issues completed per calendar period. This fluctuates based on available time and is observed, not controlled. MPG tells you if issues are right-sized. Speed tells you when you'll finish.

**Buffer** — The buffer is measured in sessions (see Task Definition above for what constitutes a session).

Goldratt's 50% rule applies: with no other input, the buffer is 50% of the issue count in a milestone. A milestone with 10 issues has a 5-session buffer. But not all sessions are equal. As you complete issues and accumulate performance data, the buffer calculation refines itself — variance is measured against the ideal session baseline, so a milestone where issues are averaging 12 commits each will dynamically size a larger buffer than one where issues average 3, because the sessions are heavier and variance has more room to bite. If you suddenly get inundated with meetings and your speed drops, that eats buffer too — even though the work itself remains consistent. Buffer is consumed when an issue deviates significantly from the ideal session baseline: more commits than the rolling MPG average, or slower throughput than current speed. Buffer can be restored if you get more efficient or pick up speed as well. You never "enter" the buffer phase — it's a reserve that absorbs variance.

In summary: *Initial buffer* — 50% of milestone issue count. *Refinement* — as issues complete, MPG and speed adjust the expected session weight. *Consumption* — buffer burns when issues exceed expected commit counts or when throughput drops.

**Fever chart** — The primary health indicator. Compares % of issues complete vs % of buffer consumed. Green (buffer burn < progress) means healthy. Yellow (roughly equal) means watch closely. Red (buffer burn significantly exceeds progress) means cut scope or extend the date.

**Time-in-chain ratio** — The ratio of time with an issue in active measurement (between start and pause/done) to total calendar time. A low ratio means your speed-based forecasts assume more focused work than you're actually doing — treat them accordingly.

### Forecasting

The same telemetry answers two different questions depending on how the milestone is configured:

- **Date-constrained milestone** (has a due date): "You committed to April 15. At current speed, you'll complete 4 of 6 issues. Here are the 2 lowest-impact candidates to cut." The date is the constraint; scope flexes.
- **Undated milestone** (scope-constrained): "At current speed, you'll be done in ~2.9 weeks." The scope is the constraint; the date is derived.

Both modes use the same signals. The question just flips: *will you make the deadline* vs *when will you be done*. These are two legs of the iron triangle — scope and time. The third leg (resources) arrives in v0.4: *would adding someone actually help here, and where?*

### Shadow Work Detection

Gaps between measurement windows — time when no issue is actively being worked — represent shadow work: context switches, meetings, untracked tasks, exploratory work, scope creep. A git-chain issue represents one ideal session; any time spent outside that session is treated as shadow work. git-chain doesn't know *what* the shadow work is, but it knows it's there, and it adjusts forecast confidence accordingly. A time-in-chain ratio of 60% means 40% of your time is invisible to the scheduler. That's not a problem the tool can fix, but it's information you should have.

### Signal Noise

Telemetry based on commits assumes developers commit reasonably often with reasonable granularity. Edge cases — huge batch commits, squash merges, generated code — introduce noise. This is acceptable because the indicators are framed as **signals**, not measurements. A noisy speedometer is still more useful than no speedometer.

### Extensibility

The observed signals list is not closed. As the tool matures, additional inputs can feed the telemetry model — commit churn (edits to previously committed files), diff size, contributor count, reflog activity, CI pass/fail rates, or anything else git can observe. Each new signal refines the forecasts without changing the architecture. The telemetry system is a sensor array, not a fixed formula.

## Architecture

Issues and milestones are stored as per-entity git refs under `refs/chain/`. Each ref is an append-only event stream. Current state is derived from the latest commit on each ref. The issue dependency graph is reconstructed dynamically from these states. No mutable state files, no external databases — just git objects and refs.

### Event-Sourced Chain Graph

Chain state is not stored as mutable records. Instead, all operations append commits describing graph mutations. The current graph is **derived** by walking the commit history on each entity's ref. This mirrors how git's own commit graph works: append-only, history is immutable, state is derived.

This design has several important consequences:

- **Merges combine event streams.** Two developers editing different issues touch different refs — zero merge conflicts ever. Same-issue edits produce parallel commit histories on the same ref, resolved the same way git resolves any diverged branch.
- **History never mutates.** Every change to an issue is a new commit on that issue's ref. `git log refs/chain/_/issues/<id>` is the complete audit trail.
- **State is reconstructed.** The current state of an issue or milestone is derived by reading the latest commit on its ref. The full history of changes is available by walking the commit chain.

This is the same pattern used by git-bug and similar git-native tools: per-entity refs where each ref's commit history is the event log for that entity.

#### Core Event Types

Each commit on an entity ref represents one of these logical operations:

| Event                | Effect                                                  |
|----------------------|---------------------------------------------------------|
| `IssueCreated`       | New issue blob written; ref created                     |
| `IssueUpdated`       | Title, body, context, or metadata changed               |
| `StateTransitioned`  | State field changed (pending → in-progress → done, etc) |
| `DependencyAdded`    | `blocked_by` or `blocks` edge added                     |
| `DependencyRemoved`  | `blocked_by` or `blocks` edge removed                   |
| `IssueSplit`         | Issue replaced by multiple child issues                 |
| `IssueMerged`        | Multiple issues combined into one; dependencies rewired |
| `IssueCancelled`     | State → cancelled; dependencies reconnected             |
| `MilestoneAssigned`  | Issue moved to a different milestone                    |
| `MeasurementStarted` | HEAD sha recorded; measurement window opened            |
| `MeasurementPaused`  | HEAD sha recorded; measurement window closed            |
| `MeasurementResumed` | HEAD sha recorded; measurement window reopened          |
| `MeasurementClosed`  | HEAD sha recorded; final measurement window closed      |

These are not stored as separate event records — each is represented as a new commit on the entity's ref containing the updated issue blob. The event type is implicit in the diff between commits. This keeps the storage model simple (it's just markdown files in git) while preserving the append-only, derived-state properties of event sourcing.

#### The Core Rule: Events Never Delete Information

The principle that makes this work in a distributed system: **events only add new facts.** Nothing is mutated in place; nothing is removed. The current state is always derived by replaying the event history.

This matters most for dependencies. Rather than mutating an edge list, dependencies are modeled as additive events — `DependencyAdded` and `DependencyRemoved` are both *additions* to the event log. The current dependency graph is computed as `edges_added - edges_removed`. This follows the classic "two-phase set" pattern used in distributed systems: additions and removals are both recorded as events, and the current state is derived from their difference. This makes merges nearly trivial — two branches that independently add or remove different dependencies produce event logs that combine cleanly without reconciliation logic.

The same principle applies to every operation. Closing an issue doesn't delete it — it adds a `StateTransitioned` event. Splitting an issue doesn't replace it — it adds an `IssueSplit` event. Merging issues doesn't destroy them — it adds an `IssueMerged` event. Moving between milestones doesn't modify the old milestone — it adds a `MilestoneAssigned` event. Because every operation is additive and commutative, two branches of work always merge by concatenating their event logs. The derived state handles the rest.

### Issue Identity

Issue identities use **UUIDv7** to ensure stable, globally unique identifiers across distributed repositories. Each chain mutation is recorded as a git commit referencing these UUIDs.

**Why UUIDv7 over git object IDs:** UUIDv7 is time-ordered, globally unique without coordination, and stable across history rewrites (`git rebase`, `git filter-repo`). Git object IDs would couple issue identity to commit history, making rebases destructive to the task graph. UUIDv7 keeps identity independent of git mechanics.

**Why UUIDv7 over sequential numbers:** Sequential numbers require a central authority to avoid collisions. Two branches creating "issue #12" simultaneously would conflict on merge. UUIDv7 is collision-free by design.

**The hybrid model:** Issue IDs are UUIDv7. Event IDs are git commit hashes. This cleanly separates "what entity are we talking about" from "what happened to it and when."

**Display and reference:** UUIDv7s are displayed truncated in CLI output (first 8 characters by default). Any unique prefix works as a reference. The CLI also resolves by tag name or title substring when unambiguous.

```bash
# All of these work:
git chain issue show 019444a1
git chain issue show 019444a1-b2c3-7def
git chain issue show auth-refactor   # resolves by tag
git chain issue show oauth           # resolves by title if unambiguous
git chain issue show                 # HEAD — current or next issue on critical chain
```

### Graph Invariants

The following structural guarantees are enforced on every write operation:

1. **The issue graph must be acyclic.** Adding a dependency that would create a cycle is rejected.
2. **All dependency edges must reference existing issues.** Dangling references are rejected at write time.
3. **Done/cancelled issues cannot gain new incoming dependencies.** You cannot add a `blocked_by` pointing to a closed issue's future work.
4. **Split issues inherit downstream dependencies.** When issue A is split into A1 + A2, anything that was blocked by A becomes blocked by the last issue in the split chain.
5. **Cancelled issues reconnect the graph.** When an issue is cancelled, its upstream dependencies are connected directly to its downstream dependencies so the critical chain remains intact.

These invariants are checked locally on every mutating operation. Violations from concurrent edits on different clones are detected and reported on the next read operation after fetch.

### Storage Layout

State is stored as per-entity refs under `refs/chain/`, similar to git-bug. Each issue and milestone gets its own git ref, and each ref's commit history is the audit trail for that entity. State never appears in the working tree.

**Why per-entity refs:** Two people editing different issues touch different refs — zero merge conflicts ever. Same-issue conflict means the same ref — fast-forward or explicit conflict. Git's existing push/pull handles sync. No custom merge logic needed.

#### Ref Layout

```
.git/refs/chain/
  _/                                  # default chain (unnamed)
    config                            # ref → config blob
    issues/
      019444a1-b2c3-7def-...          # ref → commit chain → issue blob
      019444a2-c3d4-7ef0-...          # ref → commit chain → issue blob
    milestones/
      v0.1                            # ref → commit chain → milestone blob
    tags/
      auth-refactor                   # ref → points at issue 019444a2
      parser-mvp                      # ref → points at milestone v0.1
  backend/                            # named chain (multi-chain, future)
    config
    issues/...
    milestones/...
```

**Reading state:**

- `git show refs/chain/_/issues/<id>:issue.md` — read current issue state
- `git for-each-ref refs/chain/_/issues/` — list all issues
- `git log refs/chain/_/issues/<id>` — view audit trail for an issue

**Writing state:** The CLI creates a new commit on the entity's ref with the updated blob.

**Chain naming:** Chains are always named. The default chain is `_`. Multi-chain support (different namespaces under `refs/chain/<name>/`) is planned for v0.3. The ref layout supports multi-chain without migration.

#### Performance Note

Walking commit history to rebuild state is fast for the expected scale (tens to low hundreds of issues). Each entity ref has a short commit chain — one commit per mutation of that entity, not one commit per mutation of the entire system. For projects that accumulate very long entity histories, a `chain gc` command could snapshot derived state, but this is an optimization for v0.3, not a design concern for v0.1.

### Sync

Sync is automatic. Lazy init configures refspecs so `git push` and `git pull` sync `refs/chain/*` alongside normal branches.

**Per-entity ref merge semantics:**

- Different issues = different refs = zero merge conflicts ever
- Same-issue conflict = same ref = fast-forward or explicit conflict
- Second pusher must pull and reconcile (standard git workflow)

**Semantic reconciliation:** Git merges commits structurally, but the tool must interpret them at the domain level. When semantic conflicts occur — for example, one branch closes an issue while another splits it — both commits land on the same ref. The tool detects the divergence on the next read and reports it:

```
$ git chain-sync
⚠ Conflict on 019444a2: Parse basic sub declarations
  Local:  state → done (closed by abc123)
  Remote: split into 019444a2 + 019444d1 (by def456)

  Both events preserved. Run: git chain issue edit 019444a2
  to resolve.
```

Because the full event log is preserved, resolution is always possible without data loss. The user opens the issue in `$EDITOR`, reviews both events, and chooses the correct state.

**Design assumption:** Low-contention (solo / small team). High-contention workflows are left as a lemma for the reader (or for v0.4 "Multi-Player").

**Recommended git aliases for explicit sync:**

```bash
# .gitconfig
[alias]
  chain-push = push origin refs/chain/*:refs/chain/*
  chain-pull = fetch origin refs/chain/*:refs/chain/*
  chain-sync = !git chain-pull && git chain-push
```

There is no `sync` command — `git push` and `git pull` handle everything. The aliases are a convenience for syncing chain state independently of your working branches.

## Issue Format

Issues are markdown files with YAML frontmatter. The ID is the filename (UUIDv7). The body contains three standard sections: Prerequisites (definition of ready), Context (execution context for humans and agents), and Acceptance Criteria (definition of done).

```yaml
---
title: "Parse basic signatures"
state: pending           # pending | in-progress | done | cancelled
milestone: "v0.1"
blocked_by:
  - "019444a1-..."
blocks:
  - "019444a3-..."
created: "2026-03-14T10:30:00Z"
updated: "2026-03-14T14:22:00Z"
sessions:
  - start_sha: "abc123"
    end_sha: "ghi456"
    commits: 4
  - start_sha: "mno789"
    end_sha: "stu012"
    commits: 3
---

## Prerequisites

- [x] libfoo v2.3 released
- [ ] staging environment deployed

## Context

- paths: lib/Parser/Signature.pm, t/parser/signature_*.t
- docs: docs/parser-design.md
- commands: prove -lv t/parser/
- entrypoints: lib/Parser/Signature.pm

Implement parsing for basic subroutine signatures.

## Acceptance Criteria

- [ ] positional params work
- [ ] error messages include line numbers
```

### Structured Execution Context

The optional `## Context` section lives in the markdown body immediately after Prerequisites. It turns an issue from a description into a **runnable work packet**. Both humans and agents can immediately start work without archaeology. Because it's in the body rather than frontmatter, it's easy to read and edit as documentation — the tool parses it when needed for `--format json` output.

| Field          | Purpose                                                      |
|----------------|--------------------------------------------------------------|
| `paths`        | Files and directories relevant to this issue (globs allowed) |
| `docs`         | Design docs, specs, or references                            |
| `commands`     | Build, test, or verification commands                        |
| `entrypoints`  | Where to start reading/editing code                          |

All fields are optional. The context block is not required — issues work fine without it. But when present, it dramatically reduces ramp-up time for both humans and agents.

**Design note:** Context is the inverse of the Prerequisites problem. Prerequisites contain machine-relevant data (external dependencies, resource constraints) embedded in human-readable checklists that the tool can't factor into graph calculations. Context contains human-readable documentation that the tool needs to parse into machine-accessible structured data. Both are cases where the boundary between human and machine readability needs future refinement.

**Example: what an agent sees**

```bash
$ git chain issue show 019444a2 --format json
```

```json
{
  "id": "019444a2-c3d4-7ef0-...",
  "title": "Parse basic signatures",
  "state": "pending",
  "tags": ["auth-refactor"],
  "prerequisites": [
    {"text": "libfoo v2.3 released", "checked": true},
    {"text": "staging environment deployed", "checked": false}
  ],
  "context": {
    "paths": ["lib/Parser/Signature.pm", "t/parser/signature_*.t"],
    "docs": ["docs/parser-design.md"],
    "commands": ["prove -lv t/parser/"],
    "entrypoints": ["lib/Parser/Signature.pm"]
  },
  "acceptance_criteria": [
    {"text": "positional params work", "checked": false},
    {"text": "error messages include line numbers", "checked": false}
  ]
}
```

An AI coding agent can immediately: check prerequisites, load the relevant files, read the docs, attempt the implementation, run the verification commands, and check acceptance criteria.

### Issue IDs

UUIDv7 (timestamp-ordered, collision-free). Displayed truncated in CLI output (first 8 characters by default). Any unique prefix, tag name, or unambiguous title substring works as a reference.

### Prerequisites (Definition of Ready)

Listed at the top of the body, before the main description. `issue edit --state start` checks that all prerequisite items are checked. Warns if starting with unchecked prerequisites.

**Note:** Prerequisites may include external dependencies and resource constraints (DBA availability, staging environment readiness, third-party API access) that the tool cannot currently factor into critical chain or buffer calculations. These are tracked as manual checklists for now. Getting external dependencies into the graph calculation engine is a priority for future work — without them, project forecasts are based on an incomplete picture of what actually blocks progress.

### Acceptance Criteria (Definition of Done)

Listed at the bottom of the body, after the main description. `issue edit --state done` checks that all acceptance criteria are checked. Warns if completing with unchecked criteria.

**Verification commands:** Agents (or humans) are encouraged to annotate acceptance criteria with the exact command that proves the criterion is met, using backtick-delimited commands inline:

```
- [x] positional params work (`prove -lv t/parser/signature_positional.t`)
- [x] error messages include line numbers (`grep -c 'line [0-9]' t/parser/errors.expected`)
```

The criteria text stays human-readable, but now carries its own verification. Plugins like `git-chain-smoker` can parse the backtick-delimited commands and run them as regression tests. The issue becomes progressively more machine-verifiable as work happens — acceptance criteria start as intent and finish as executable specifications.

**Verifiable done states:** Because issues track the exact commit SHA when they were marked done, checking out that reference and running the acceptance criteria commands should produce a clean pass. If it doesn't, either someone rewrote history or the criteria were checked without the implementation actually meeting them. Git's content-addressed storage plus executable acceptance criteria means "done" is a provable claim, not just a checkbox.

### `---` Splitting

Both `issue add` and `issue edit --split` use `---` to delimit multiple issues from a single `$EDITOR` session. Multiple blocks separated by `---` create multiple issues, chained sequentially.

**Convention:** `---` serves double duty as YAML frontmatter delimiters and as issue separators, following the same convention used by Deckset, Marp, and other markdown-based tools. The first `---...---` block at the top of the session is the first issue's frontmatter. A subsequent `---` followed by YAML key-value pairs opens a new issue. If you need a horizontal rule inside an issue body, use `***` or `___` instead — both are valid markdown horizontal rules that won't be mistaken for an issue boundary.

### State Transitions

```
pending ──────────► in-progress ──────────► done
                        │   ▲
                   start│   │resume
                        │   │
                   pause│   │
                        ▼   │
                    (still in-progress,
                     measurement paused)

pending ──────────────────────────────────► cancelled
in-progress ──────────────────────────────► cancelled
cancelled ─── ($EDITOR only) ─────────────► pending
```

State is an explicit field. There is no "triage" state — issues are always in the chain. There is no "backlog" state — pending issues are simply pending.

**Uncancelling:** There is no `--state uncancel` flag — cancellation is intentionally a deliberate act. But if you open a cancelled issue in `$EDITOR` and change `state: cancelled` back to `state: pending`, the tool accepts it. The dependencies that were reconnected around the cancelled issue are not automatically restored — you'll need to re-add them manually.

### Measurement Bookmarks

- `issue edit --state start` — records current HEAD sha, opens measurement window
- `issue edit --state pause` — records HEAD sha, closes measurement window (state remains in-progress)
- `issue edit --state resume` — records HEAD sha, reopens measurement window
- `issue edit --state done` — records HEAD sha, closes final measurement window

The commit count between start/end bookmarks in each measurement window feeds the telemetry signals. MPG is the rolling average of these counts across completed issues.

## Milestone Format

Milestones are plain YAML files. Unlike issues, milestones are pure metadata — there's no body content that benefits from markdown formatting.

```yaml
name: "v0.1"
due: "2026-04-15"              # optional
description: "Initial parser implementation."  # optional
resolution: "make integration-test"  # optional — milestone-level verification (v0.2)
created: "2026-03-14T10:00:00Z"
```

**Resolution commands (v0.2):** If issue acceptance criteria are unit tests, the milestone resolution command is the integration test. It verifies that all the completed issues work together as a whole. When present, an agent walking the chain knows the milestone is truly done when all issues are complete *and* the resolution command passes — not just when the last checkbox is checked.

## Configuration

```yaml
# config
version: 1
default_milestone: "v0.1"
```

Configuration is minimal. No hook configuration — hooks are the user's responsibility. No sync configuration — refspecs are configured on lazy init.

## Lazy Initialization

There is no `init` command. The first `git chain` command that modifies state (e.g., `issue add`) performs lazy initialization:

1. Creates the ref namespace under `refs/chain/_/`
2. Creates default config
3. Configures refspecs for automatic sync on push/pull
4. Creates a default milestone if none exists

The first `git chain` command on a clone detects existing refs, fetches them, and configures local refspecs.

## Commands

git-chain has 10 commands organized into three groups: top-level (2), issue (4), and milestone (4).

All commands that produce output support `--format json` for machine-readable output. User-defined templates for custom reporting are future work.

**Tags:** Issues and milestones can be tagged with short human-readable names, following the same mental model as git tags — lightweight named references that point at entities. `HEAD` is a built-in tag that resolves contextually: for issue commands, it points to the current in-progress issue, or if none, the next issue on the critical chain (selected by most downstream dependencies); for milestone commands, it points to the milestone of the HEAD issue. Where commands accept a `<ref>` argument, any of the following work: UUIDv7 (or unique prefix), tag name, title substring, or `HEAD`. If `<ref>` is omitted, `HEAD` is the implicit default.

**`git chain next`:** Effectively an alias for `git chain issue show HEAD` — show the current in-progress issue, or if none, the next issue on the critical chain with the most downstream dependencies, with full context, prerequisites, and acceptance criteria.

### Top-Level Commands

#### `git chain list`

Show the full chain, linearized by default.

**Behavior:**

- Default: topological sort of all issues, grouped by milestone
- `--graph`: ASCII DAG visualization showing dependency structure
- `--critical`: highlight the critical chain (longest sequential path)
- `--format json`: machine-readable output

**Output (default — linearized):**

```
v0.1 [due: Apr 15 — 22 days left]

  019444a1  Implement lexer                 ● in-progress
  019444a2  Parse basic sub declarations     ○ pending (blocked by 019444a1)
  019444a3  Parse signatures                 ○ pending (blocked by 019444a1)
  019444a4  Error recovery                   ○ pending
  019444a5  Full method modifier support     ○ pending

v0.2 [no due date]

  019444b1  Bootstrap compiler               ○ pending (blocked by 019444a2)
  019444b2  Test harness                     ○ pending
```

**Output (--critical):**

```
Critical Chain (3 issues)

  019444a1  Implement lexer                    ● in-progress
   ↓
  019444a2  Parse basic sub declarations       ○ pending
   ↓
  019444b1  Bootstrap compiler                 ○ pending

Current constraint: 019444a1
Parallel work available: 019444a3, 019444a4, 019444a5, 019444b2
```

**Output (--critical --format json):**

```json
{
  "critical_chain": [
    {"id": "019444a1-...", "title": "Implement lexer", "state": "in-progress"},
    {"id": "019444a2-...", "title": "Parse basic sub declarations", "state": "pending"},
    {"id": "019444b1-...", "title": "Bootstrap compiler", "state": "pending"}
  ],
  "current_constraint": "019444a1-...",
  "parallel_work": ["019444a3-...", "019444a4-...", "019444a5-...", "019444b2-..."]
}
```

**Options:**

- `--all` — Include done/cancelled issues
- `--milestone <name>` — Filter to a specific milestone
- `--graph` — ASCII DAG visualization
- `--critical` — Show critical chain only
- `--format json` — Machine-readable output

**Legend:** `●` in-progress, `○` pending, `✓` done, `✗` cancelled

-----

#### `git chain config`

Manage git-chain settings.

**Behavior:**

- Without arguments: display current configuration
- With key/value: set a configuration option
- Settings stored in `refs/chain/_/config`

**Examples:**

```
$ git chain config
version: 1
default_milestone: v0.1

$ git chain config default_milestone v0.2
Set default_milestone = v0.2
```

-----

### Issue Commands

#### `git chain issue add`

Create one or more new issues.

**Behavior:**

- Opens `$EDITOR` for issue content (frontmatter + body)
- Multiple issues can be created in one `$EDITOR` session by separating with `---`
- Issues created from `---`-separated blocks are chained sequentially (each blocks the next)
- If stdin is not a terminal, reads from stdin
- Lazy-inits chain state on first use

**Examples:**

Quick capture (single issue):

```
$ git chain issue add
# $EDITOR opens with template:
# ---
# title: ""
# milestone: "v0.1"
# ---
#
# ## Prerequisites
#
# ## Context
#
# - paths:
# - docs:
# - commands:
# - entrypoints:
#
# ## Acceptance Criteria

Created 019444c1: Fix heredoc edge case
```

Batch capture (multiple issues):

```
$ git chain issue add
# $EDITOR opens, user writes:
# ---
# title: "Parse basic signatures"
# ---
#
# Basic sig parsing
#
# ---
# title: "Parse complex signatures"
# ---
#
# Complex sig parsing

Created 2 issues (chained sequentially):
  019444c1  Parse basic signatures
   ↓
  019444c2  Parse complex signatures
```

**Options:**

- `--before <ref>` — Position before another issue
- `--after <ref>` — Position after another issue
- `--milestone <name>` — Assign to specific milestone (default: current milestone)

-----

#### `git chain issue list`

List issues.

**Behavior:**

- Lists all open issues sorted by chain position
- Shows state and blocking status

**Output:**

```
  019444a1  Implement lexer                 ● in-progress
  019444a2  Parse basic sub declarations     ○ pending
  019444a3  Parse signatures                 ○ pending
  019444a4  Error recovery                   ○ pending
  019444a5  Full method modifier support     ○ pending
  019444b1  Bootstrap compiler               ○ pending
  019444b2  Test harness                     ○ pending
```

**Options:**

- `--all` — Include done/cancelled issues
- `--milestone <name>` — Filter to specific milestone
- `--state <state>` — Filter by state (pending/in-progress/done/cancelled)
- `--format json` — Machine-readable output

-----

#### `git chain issue show <ref>`

View an issue with full context: dependencies, measurement windows, execution context, and chain position.

**Behavior:**

- Displays issue details plus dependency context
- Shows position in critical chain (if applicable)
- Shows measurement windows (commit bookmarks)
- Shows execution context if present

**Output:**

```
019444a2: Parse basic sub declarations

  State:      pending
  Milestone:  v0.1 (due Apr 15)
  Tags:       auth-refactor
  Created:    2026-03-14

  Blocked by:
    ✓ 019444a1  Implement lexer (done)

  Blocks:
    019444b1  Bootstrap compiler (pending)

  Measurement:
    abc123..ghi456  (4 commits)
    mno789..stu012  (3 commits)

  Context:
    Paths:        lib/Parser/Signature.pm, t/parser/signature_*.t
    Docs:         docs/parser-design.md
    Commands:     prove -lv t/parser/
    Entrypoints:  lib/Parser/Signature.pm

  Chain position:
    019444a1 → [019444a2] → 019444b1
                ^^^^^^^^
    2nd in critical chain (next up)

  ## Prerequisites
    [x] libfoo v2.3 released
    [ ] staging environment deployed

  Body:
    Implement parsing for basic subroutine signatures.

  ## Acceptance Criteria
    [ ] positional params work
    [ ] error messages include line numbers
```

**Audit trail:** Use `git log refs/chain/_/issues/<id>` to view the full history of changes to an issue.

-----

#### `git chain issue edit <ref>`

Modify an issue. The swiss army knife — state transitions, positioning, dependencies, splitting, merging, and cancellation are all available via flags or `$EDITOR` frontmatter editing.

**Behavior:**

- Without flags: opens `$EDITOR` with the issue's current content (frontmatter + body)
- With flags: applies the specified modifications directly
- Multiple flags can be combined in a single invocation

**Flags:**

| Flag                 | Effect                                                          |
|----------------------|-----------------------------------------------------------------|
| `--state <state>`    | Transition state: `start`, `pause`, `resume`, `done`, `cancel` |
| `--before <ref>`     | Reposition before another issue                                 |
| `--after <ref>`      | Reposition after another issue                                  |
| `--block <ref>`      | Add forward dependency (this issue blocks `<ref>`)              |
| `--unblock <ref>`    | Remove forward dependency                                       |
| `--milestone <name>` | Move to a different milestone                                   |
| `--tag <name>`       | Add a human-readable tag to this issue                          |
| `--untag <name>`     | Remove a tag                                                    |
| `--split`            | Open `$EDITOR` to split into multiple issues (use `---` separators) |
| `--merge <ref>`      | Merge this issue with another (combines into one session; dependencies rewired) |
| `--purge`            | Permanently delete (erroneous data only; requires confirmation) |

**State transitions with `--state`:**

```
$ git chain issue edit 019444a2 --state start
Started 019444a2: Parse basic sub declarations
  Recorded HEAD: abc1234
  ⚠ Unchecked prerequisites:
    [ ] staging environment deployed
  Start anyway? [y/N]

$ git chain issue edit 019444a2 --state pause
Paused 019444a2: Parse basic sub declarations
  Recorded HEAD: def5678
  Window: abc1234..def5678 (4 commits)

$ git chain issue edit 019444a2 --state resume
Resumed 019444a2: Parse basic sub declarations
  Recorded HEAD: ghi9012

$ git chain issue edit 019444a2 --state done
Completed 019444a2: Parse basic sub declarations
  Recorded HEAD: jkl3456
  Window: ghi9012..jkl3456 (3 commits)
  Total: 7 commits across 2 measurement windows
  MPG impact: rolling average now 6.2 commits/issue
  ⚠ Unchecked acceptance criteria:
    [ ] error messages include line numbers
  Complete anyway? [y/N]
```

**`$EDITOR` workflow:**

```
$ git chain issue edit 019444a2
# $EDITOR opens with current issue content:
# ---
# title: "Parse basic sub declarations"
# state: pending
# milestone: "v0.1"
# blocked_by:
#   - "019444a1-..."
# blocks:
#   - "019444b1-..."
# ---
#
# ## Prerequisites
# - [x] libfoo v2.3 released
#
# ## Context
# - paths: lib/Parser/Declaration.pm
# - commands: prove -lv t/parser/
#
# Implement parsing for basic subroutine declarations.
#
# ## Acceptance Criteria
# - [ ] positional params work
#
# User edits frontmatter and/or body, saves and exits.

Updated 019444a2: Parse basic sub declarations
  Changed: title, milestone
```

**Splitting with `--split`:**

```
$ git chain issue edit 019444a2 --split
# $EDITOR opens with issue content.
# User inserts --- to split into multiple blocks.
# Each block becomes a separate issue, chained sequentially.
# Downstream dependencies transfer to the last issue.

Split 019444a2 into:
  019444a2  Parse basic signatures
   ↓
  019444d1  Parse complex signatures

Dependencies transferred: 019444b1 now blocked by 019444d1
```

**Merging (combining trivially small issues):**

```
$ git chain issue edit 019444a4 --merge 019444a5
Merged 019444a5 into 019444a4:
  019444a4  Error recovery + full method modifier support

Dependencies from 019444a5 transferred to 019444a4.
Measurement windows combined.
```

**Cancellation:**

```
$ git chain issue edit 019444a5 --state cancel
Cancelled 019444a5: Full method modifier support
  Reconnected dependencies around cancelled issue
```

**Purging (permanent deletion):**

```
$ git chain issue edit 019444a5 --purge
Permanently delete 019444a5: Full method modifier support? [y/N] y
Purged 019444a5 (permanently deleted)
```

-----

### Milestone Commands

#### `git chain milestone add <name>`

Create a new milestone.

**Behavior:**

- Creates a milestone with the given name
- Default milestone auto-created on first `issue add` if none exists

**Examples:**

```
$ git chain milestone add v0.2
Created milestone: v0.2
```

**Options:**

- `--due <date>` — Set due date (YYYY-MM-DD or relative: `2 weeks`, `friday`, `end of month`)

-----

#### `git chain milestone list`

List all milestones.

**Behavior:**

- Shows milestones with issue counts and buffer status (color-coded GREEN/YELLOW/RED)
- `--health` adds MPG and speed per milestone
- Highlights current milestone (first with pending issues)

**Output (default):**

```
  v0.1        due Apr 15    2/5 done    GREEN    ← current
  v0.2        (no date)     0/2 done    —
```

**Output (--health):**

```
  v0.1        due Apr 15    2/5 done    GREEN    MPG: 5.3    speed: 1.4/wk    ← current
  v0.2        (no date)     0/2 done    —        MPG: —      speed: —
```

**Options:**

- `--health` — Show MPG and speed per milestone
- `--format json` — Machine-readable output

-----

#### `git chain milestone show <ref>`

Show milestone detail with issues and fever chart.

**Behavior:**

- Shows milestone details, issue list, and fever chart
- This is your project health check

**Output:**

```
$ git chain milestone show HEAD

v0.1 [due: Apr 15 — 22 days left]

Issues:
  ✓ 019444a1  Implement lexer                     (5 commits)
  ✓ 019444a2  Parse basic sub declarations         (7 commits)
  ○ 019444a3  Parse signatures
  ○ 019444a4  Error recovery
  ○ 019444a5  Full method modifier support

Progress: 2/5 issues done (40%)
MPG:      6.0 commits/issue (rolling average)
Speed:    1.4 issues/week
Buffer:   2 sessions (refined from MPG + speed)
          0.3 consumed (019444a2 ran slightly hot)
Forecast: ~2.1 weeks remaining at current speed

Time-in-chain: 72%
  ⚠ Forecast assumes focused work — actual pace may be slower

Resolution: make integration-test    (v0.2)

Fever chart:
  [====........] 40% progress
  [=...........] 15% buffer consumed
       ^ GREEN — ahead of burn rate
```

**If buffer is in the red:**

```
Fever chart:
  [===.........] 30% progress
  [========....] 70% buffer consumed
              ^ RED — burning faster than progress

Cut candidates (lowest impact on critical chain):
  019444a5  Full method modifier support
  019444a4  Error recovery

Recommendation: MPG averaging 12.4 commits/issue.
                Consider slicing issues thinner.
```

**Buffer status zones:**

```
GREEN:  buffer burn % < progress %       (ahead of schedule)
YELLOW: buffer burn % ≈ progress %       (on track, watch closely)
RED:    buffer burn % > progress % + 20  (behind, consider cutting scope)
```

-----

#### `git chain milestone edit <ref>`

Modify a milestone.

**Behavior:**

- Without flags: opens `$EDITOR` with milestone content
- With flags: applies modifications directly

**Examples:**

```
$ git chain milestone edit v0.1 --due 2026-04-30
v0.1: due date → Apr 30 (was: Apr 15)

$ git chain milestone edit v0.1 --name "Parser MVP"
v0.1: renamed → Parser MVP
```

**Options:**

- `--due <date>` — Set or update due date (use `--due none` to clear)
- `--name <name>` — Rename the milestone
- `--resolution <command>` — Set the milestone-level verification command (v0.2; use `--resolution none` to clear)
- `--tag <name>` — Add a human-readable tag to this milestone
- `--untag <name>` — Remove a tag

-----

## Agent Access

Three layers, from most to least integrated:

### 1. CLI with JSON output (preferred)

All commands support `--format json`. This is the primary integration point for AI coding agents and scripting.

```bash
# Get the next issue with full execution context
git chain issue show --format json

# Start work
git chain issue edit 019444a2 --state start

# agent works using context.paths, context.commands, acceptance_criteria...

# Finish
git chain issue edit 019444a2 --state done
```

The structured execution context on issues (`context.paths`, `context.docs`, `context.commands`, `context.entrypoints`) gives agents everything they need to begin work immediately without repo archaeology.

**Autonomous milestone completion (v0.2):** Because HEAD advances as issues complete, an agent loop (such as Claude Code's Ralph Loop) can walk the entire chain:

```
loop:
  issue = git chain issue show --format json
  if no pending issues: run milestone resolution command → stop
  git chain issue edit <id> --state start
  # work using context, paths, entrypoints
  # verify using acceptance criteria commands
  git chain issue edit <id> --state done
  # HEAD advances to next issue → repeat
```

Issue acceptance criteria are the per-issue verification. The milestone's `resolution` command is the integration test — it fires after the last issue completes and verifies the whole thing works together. An agent can implement an entire milestone autonomously, stopping only when the resolution command passes or when it needs human intervention.

### 2. Direct ref access via `git show`

Read individual issues with `git show refs/chain/_/issues/<id>:issue.md`. List issues with `git for-each-ref refs/chain/_/issues/`. No chain binary needed.

### 3. Future: MCP server

Expose chain state as MCP tools for deeper agent integration. See Future Considerations.

## Technical Requirements

### Dependencies

- **Go 1.22+** (build-time only)
- **git** (runtime — for ref manipulation)
- No runtime dependencies beyond git itself

### Installation

```bash
go install github.com/perigrin/git-chain@latest
```

Or download a prebuilt binary from releases (place `git-chain` on your `$PATH`).

### Build

```bash
git clone https://github.com/perigrin/git-chain
cd git-chain
go build -o git-chain .
```

## Implementation Phases

### v0.1 — Core

- Event-sourced per-entity ref storage under `refs/chain/_/`
- Graph invariant enforcement (acyclicity, referential integrity, reconnection on cancel)
- Lazy init (no init command)
- All 10 commands with `--format json` support
- Issue lifecycle: add, list, show, edit (all flags including --state, --split, --merge, --purge)
- Structured execution context (optional `## Context` section in issue body)
- Milestone lifecycle: add, list, show, edit
- Telemetry: MPG, speed, buffer, time-in-chain, shadow work detection
- Fever chart in `milestone show`
- Automatic refspec configuration for sync
- UUIDv7 issue IDs with prefix-based, tag-based, and title-based resolution
- Lightweight tags on issues and milestones (`refs/chain/_/tags/`)
- `HEAD` as built-in tag resolving to current in-progress issue (or next on critical chain by downstream dependency count); milestone HEAD derived from issue HEAD
- `git-chain-*` subcommand discovery on `$PATH`
- Single chain only (default `_`)

### v0.2 — Agentic

- Milestone resolution commands (integration-test-level verification)
- `chain validate` command (aggregate integrity check)
- `git-chain-smoker` plugin (acceptance criteria as regression tests, milestone resolution as stop condition)
- Autonomous agent loop support (Ralph Loop walks the chain, issue by issue, milestone resolution as completion gate)
- Urgency metadata on issues
- Scope cutting recommendations based on urgency

### v0.3 — Multi-Chain + Polish

- Multi-chain support (`--chain <name>` flag)
- Named chains under `refs/chain/<name>/`
- Cross-chain visibility
- `chain gc` / `chain snapshot` for compacting long entity histories
- Post-commit hooks (scan commit messages for `closes <ref>`, `starts <ref>` — documented as a recommended `.githooks` pattern, not git-chain's responsibility)

### v0.4 — Multi-Player + UI

v0.1 through v0.3 cover two legs of the iron triangle: scope and time. Multiplayer unlocks the third — resources. With contributor attribution on issues, the tool can predict where adding people will actually *help* versus where it just adds communication overhead (the critical chain already shows where the work bottleneck is; resource awareness shows whether parallelizing it is possible or whether shared context/files make it slower).

- External tracker sync via `git-chain-*` plugins (GitHub Issues, Jira, GitLab)
- Resource contention and expertise routing — if the same contributor is assigned to parallel issues on the critical chain, they're actually sequential. If parallel issues share files (via context paths), adding a second contributor may cause merge friction rather than speedup. And critically: if the commit history shows Developer A has deep experience in the code an issue touches, delaying that issue until Developer A is available *may actually improve overall delivery timelines* compared to starting Developer B immediately. The data to make these predictions — commit history crossed with issue context paths — is already in git.
- External dependencies in graph calculations
- Web UI (HTMX-driven, reads from git refs) — fever charts, drag-and-drop issue ordering, visual DAG, clickable state transitions
- Interactive TUI for terminal-native chain visualization
- Native GUI for macOS/iOS

## Future Considerations

Beyond v0.4:

1. **User-defined output templates** — Expand `--format` beyond `json` to support user-defined templates (Go `text/template` or similar) for custom reporting, dashboards, and integration with other tools.
2. **Fever chart history** — Track buffer burn over time using the commit history on entity refs. Would enable trend analysis: "buffer burn accelerating."
3. **Multi-repo chains** — Cross-repo dependencies using remote refs. One chain spanning multiple repositories.
4. **`git chain place`** — Mise-en-place for development. Reads the current issue's Context block and sets up your working environment: opens files and entrypoints in your editor, loads docs, runs baseline test commands. Everything in its place before you start cooking. (Agents already get equivalent data from `git chain issue show --format json`.)
5. **`git-crochet`** — A `git-chain-*` plugin that uses an LLM to decompose a PRD or project description into a chain: issues with dependencies, context blocks, acceptance criteria, and milestone structure. The LLM does the decomposition; `git-chain` stores the result.

## Success Criteria

The tool is successful if:

1. Time from "I have an idea" to "it's captured as an issue" is under 10 seconds (`git chain issue add`)
2. "What should I work on next?" is answered instantly (`git chain issue show`)
3. Daily workflow (`git chain list`, `git chain issue edit --state start/done`) requires no browser and no network
4. Project health is visible at a glance (`git chain milestone show HEAD`)
5. Zero configuration required — lazy init handles everything
6. Works completely offline; syncs transparently when connectivity returns
7. A human or agent can learn the tool from `--help` alone
8. An agent without the CLI can access state via `git show` on the entity refs
9. An agent with the CLI can start work immediately using structured execution context — no repo archaeology required
10. An agent can walk the chain autonomously — working each issue, verifying acceptance criteria, and completing a milestone with no human intervention beyond the initial task definition