# Contributing to git-zhi

## Build & Run

Requires **Go 1.24+** and **git**.

```bash
make build              # single unified binary, version stamped from git
make install            # build, then install to $PREFIX/bin (default ~/.local)
make cross-compile      # all four release platforms
```

Build with `make`, not `go build` directly. The Makefile is the only place
version, commit and build time are defined, and the release workflow calls it
too — so every binary reports a version derived the same way. A binary built
with bare `go build` carries no ldflags and reports
`unknown (built without make)`, which is deliberate: a plausible-looking
version number there cannot be told apart from a real release.

All commands ship as one binary. The plugin command trees (historian, jira,
mermaid, project, verify, sanbao, docs) are registered as subcommands in
`cmd/git-zhi/main.go`; they import `internal/cli` for the `App` struct, so
`cli.NewRootCommand` cannot register them itself.

## Testing

```bash
go test ./...                    # run all tests
go test ./internal/graph/...     # run tests for a specific package
go test -run TestCycleDetection  # run a single test by name
go test -v ./...                 # verbose output
go test -race ./...              # with race detector
```

Tests use real on-disk git repos via `t.TempDir()` + `git.PlainInit`. No
mocks. This catches issues that in-memory testing misses.

## Architecture

### Package Structure

```
cmd/git-zhi/           unified entry point, plugin subcommand wiring
internal/
  cli/                 Cobra commands, App struct, test helpers
  storage/             git ref CRUD (blob/tree/commit/ref operations)
  issue/               Issue domain model, parse/marshal, sections, loader, label indexes
  milestone/           Milestone domain model, loader
  graph/               DAG, invariants, critical chain, topo sort
  telemetry/           MPG, speed, buffer, fever chart, time-in-chain
  config/              Chain configuration
  resolve/             Ref argument resolution (HEAD, tag, UUID prefix, title)
  actor/               Worker identity (human/agent type, parsing)
  verify/              AC command extraction, execution, prioritization, CLI
  sanbao/              Observatory plugin (report, cmd)
    dora/              DORA metrics (lead time, cycle time, rework rate)
    space/             SPACE metrics (agent autonomy, review cycles)
    calms/             CALMS indicators (verification coverage, WIP compliance)
    sentiment/         VADER commit sentiment analysis
    difficulty/        Composite issue difficulty scoring
    complexity/        Language-agnostic code complexity (churn, coupling)
    report/            Report aggregation
  docs/                Documentation scaffolding, validation, health checks, CLI
  lineage/             Blame-to-issue mapping, observed coupling graph
  historian/           Core extension: reconstruct issues from git history
    extract/           Git log parsing, diff fingerprinting
    cluster/           Greedy sequential clustering, centroid coherence
    enrich/            Cluster-to-issue conversion, confidence scoring
  jira/                Jira sync plugin domain logic
    client/            Jira REST API client (HTTP, auth, pagination)
    sync/              Bidirectional sync, conflict detection, snapshots
    credentials/       Env var and git config credential resolution
  project/             Cross-repo aggregation, CCPM buffers, capacity
  mermaid/             Gantt and DAG chart generation from JSON
  version/             Build-time version information (ldflags)
```

### Storage Model

State is stored as per-entity git refs under `refs/zhi/`. Each entity gets
its own ref whose commit history is an append-only event log. Current state
is derived by reading the latest commit. Events never delete information —
they only add facts (two-phase set pattern for dependencies).

### Core Domain Concepts

- **Chain**: The dependency DAG of issues. Default chain is `_`.
- **Issue**: A node in the DAG. UUIDv7 identity. States: pending, in-progress, done, cancelled, reopened. State transitions record actor identity and timestamp.
- **Milestone**: A delivery grouping with optional due date and buffer.
- **Critical Chain**: Longest sequential path — determines what blocks delivery.
- **Buffer**: Absorbs variance. Starts at 50% of issue count, refines via telemetry.
- **HEAD**: Current in-progress issue, or next on critical chain.
- **observed_paths**: File paths touched during a session; recorded on done transition via `git diff --name-only`. Used by verify and the parallelizer for path-overlap detection.
- **urgency**: Per-issue scheduling priority (high/normal/low) within a milestone.
- **Labels**: Free-form tag strings for categorizing and filtering issues. Label indexes live under `refs/zhi/_/labels/<label>/<uuid>`.
- **Assigned**: Advisory worker identity on an issue. Any worker can start any ready issue.
- **Historian**: Core extension that reconstructs issues from git history via extract→cluster→enrich pipeline. Config at `refs/zhi/_/historian/config`.
- **TrackerID**: External tracker reference (e.g. `jira:LOPS-142`) for sync plugins.
- **Confidence/Source**: Historian's mapping reliability (0.0-1.0) and construction method (tracker-match, ticket-ref, cluster, single-commit).

### Graph Invariants (enforced on every write)

1. Acyclic — no cycles allowed
2. Referential integrity — no dangling edges
3. Done/cancelled issues cannot gain new incoming dependencies
4. Split issues inherit downstream dependencies
5. Cancelled issues reconnect the graph

### Key Design Decisions

- **Per-entity refs** over single-file state: eliminates merge conflicts
- **UUIDv7** over git OIDs or sequential numbers: stable across rebases
- **Lazy init** over explicit `init` command: first mutating command bootstraps
- **Events as commits**: event type is implicit in the diff between commits
- **Markdown+YAML** for issues (human-readable), **YAML frontmatter + markdown body** for milestones (backward-compatible with pure YAML v0.1 milestones)
- **Critical Chain scheduling** (Goldratt): safety pooled into shared buffer
- **`docs check` reachability exemptions**: `docs/decisions/` and `docs/postmortems/`
  are exempt from the reachability check — ADRs and postmortems form a
  sequential record indexed by number, not by explicit links from
  CONTRIBUTING.md, so requiring a link would be a false positive on every
  entry. The exempt list is `reachabilityExemptPrefixes` in
  `internal/docs/check.go`; add a directory there, with a reason, if it needs
  the same exemption.

## Conventions

### Code

- Every `.go` file starts with a 2-line `ABOUTME:` comment explaining what it does
- Match the style and formatting of surrounding code
- Prefer simple, clean, maintainable solutions over clever ones
- Make the smallest reasonable changes to achieve the desired outcome

### Testing

- TDD: write a failing test before writing implementation
- Tests must comprehensively cover all functionality
- No mocks — real git repos, real data
- Test output must be pristine: no failures, no unexpected warnings

### Git

- Default branch is `pu`
- Feature branches: create from `pu`, merge back to `pu`
- Commit messages: focus on "why" not "what"
- Never skip hooks (`--no-verify`)

### Commands

All commands support `--format json` for machine-readable output.

**Top-level:** `list`, `config`, `next`
**Issue:** `add`, `list`, `show`, `edit`
**Milestone:** `add`, `list`, `show`, `edit`, `prune`

Notable flags:
- `list --ready` — show the ready set with path-overlap analysis
- `list --label <name>` — filter by label (graph built from all issues, display filtered)
- `next --actor <id>` — resolve HEAD for a specific worker identity
- `next --label <name>` — filter ready set by label
- `issue edit --label/--unlabel` — add/remove labels
- `issue edit --assign/--unassign` — set/clear worker assignment
- `issue edit --batch` — bulk edit from JSON stdin (used by sync plugins)
- `issue list --assigned <worker>` — filter by assignment
- `milestone show --workers N` — include parallelization forecast up to N workers
- `milestone show --label <name>` — filter milestone issues by label
- `milestone edit --resolve` — run the milestone's resolution command
- `milestone prune --dry-run` — report which empty, unauthored milestones would be removed
- `milestone edit --state complete` — run quality gates and mark milestone completed
- `config reindex` — rebuild label indexes from issue data

Plugin commands (invoked as `git zhi <name>`):
- `verify` — extract and run acceptance criteria from issue descriptions
- `sanbao report` — emit observatory report (DORA/SPACE/CALMS metrics, sentiment, difficulty, complexity)
- `docs init` — scaffold documentation structure
- `docs check` — validate documentation completeness
- `docs health` — report documentation health status
- `historian` — reconstruct issues from git history (extract→cluster→enrich)
- `historian triage` — interactive cluster review ([a]ssign/[s]kip/[m]erge/[q]uit)
- `historian status` — coverage report (mapped vs unmapped commits)
- `historian reindex` — rebuild label indexes
- `jira <key>` — fetch Jira ticket, emit issue YAML to stdout
- `jira sync pull` — inbound sync, emit batch edit JSON (pipe to `issue edit --batch`)
- `jira sync push` — outbound sync, push state to Jira via REST
- `jira resolve <key> --keep-zhi|--keep-tracker` — resolve sync conflicts
- `jira enrich <milestone>` — augment historian issues with Jira metadata
- `project show <file>` — cross-repo project status with CCPM buffers
- `project next --actor <name> <file>` — cross-repo "what's next"
- `mermaid gantt` — Gantt chart from JSON stdin
- `mermaid dag` — DAG visualization from JSON stdin

### Worker Identity (`ZHI_ACTOR`)

A process declares who it is by exporting `ZHI_ACTOR`. Every transition it
records carries that identity, and `next` resolves per-worker rather than
globally, so two agents in one repository stop looking like the same worker.

```bash
export ZHI_ACTOR=agent:worker-3     # or human:chris
```

The value must name its type — `agent:` or `human:` — and is refused without
one. A bare name would be recorded as a human, silently.

Resolution order is `--actor` where a command has one (`next`,
`project next` — no write command takes it, see ADR 0004), then `ZHI_ACTOR`,
then git's `user.name`. With nothing declared, behaviour is exactly what it was
before the variable existed. Commit authorship is untouched either way: blame
records who wrote the code, the chain records who moved the work.

Two properties worth knowing before relying on it.

It is **ambient**. An exported value applies to every git-zhi process in that
shell and does not appear in the command that ran. To record one action under a
different identity, prefix the invocation rather than exporting:

```bash
ZHI_ACTOR=human:chris git zhi issue edit <id> --state done
```

It is **unverified**. `ZHI_ACTOR=agent:someone-else` is accepted as given. That
is correct for coordinating workers and wrong for anything else — nothing
should ever read an actor to decide what a worker is permitted to do.

### Issue Identity

UUIDv7 — time-ordered, globally unique. Displayed truncated (8 chars).
Resolvable by UUID prefix, tag name, or unambiguous title substring.

### Subcommand Discovery

Any executable named `git-zhi-<name>` on `$PATH` is invocable as `git zhi <name>`.

## Implementation Phases

- **v0.1** — Core: storage, graph, all 10 commands, telemetry, sync, lazy init (done)
- **v0.2** — Scaling: quality gates, parallelization forecast, lineage, verify/sanbao/docs plugins, Crochet intelligence layer (done)
- **v0.3** — Enterprise: labels, multiplayer, historian, Jira sync, cross-repo projects, Mermaid visualization (done)
- **v0.4** — Future: web UI, real-time sync
