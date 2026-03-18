# Contributing to git-zhi

## Build & Run

Requires **Go 1.24+** and **git**.

```bash
go build -o git-zhi ./cmd/git-zhi/
go build -o git-zhi-verify ./cmd/git-zhi-verify/
go build -o git-zhi-sanbao ./cmd/git-zhi-sanbao/
go build -o git-zhi-docs ./cmd/git-zhi-docs/
go build -o git-zhi-historian ./cmd/git-zhi-historian/
go build -o git-zhi-jira ./cmd/git-zhi-jira/
go build -o git-zhi-project ./cmd/git-zhi-project/
go build -o git-zhi-mermaid ./cmd/git-zhi-mermaid/
go install github.com/perigrin/git-zhi@latest
```

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
cmd/git-zhi/           entry point
cmd/git-zhi-verify/    verify plugin entry point
cmd/git-zhi-sanbao/    sanbao observatory plugin entry point
cmd/git-zhi-docs/      documentation health plugin entry point
cmd/git-zhi-historian/ historian core extension entry point
cmd/git-zhi-jira/      Jira sync plugin entry point
cmd/git-zhi-project/   cross-repo project aggregation entry point
cmd/git-zhi-mermaid/   Mermaid visualization entry point
internal/
  cli/                 Cobra commands, App struct, test helpers
  storage/             git ref CRUD (blob/tree/commit/ref operations)
  issue/               Issue domain model, parse/marshal, sections, loader, label indexes
  milestone/           Milestone domain model, loader
  graph/               DAG, invariants, critical chain, topo sort
  telemetry/           MPG, speed, buffer, fever chart, time-in-chain
  config/              Chain configuration
  resolve/             Ref argument resolution (HEAD, tag, UUID prefix, title)
  uuids/               Shared UUID slice utilities
  actor/               Worker identity (human/agent type, parsing)
  verify/              AC command extraction, execution, prioritization, CLI
  sanbao/              Observatory plugin (report, cmd)
    dora/              DORA metrics (lead time, cycle time, rework rate)
    space/             SPACE metrics (agent autonomy, review cycles)
    calms/             CALMS indicators (verification coverage, WIP compliance)
    sentiment/         VADER commit sentiment analysis
    difficulty/        Composite issue difficulty scoring
    complexity/        Language-agnostic code complexity (churn, coupling)
    lsp/               LSP JSON-RPC client for language-specific metrics
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
    identity/          Actor-to-Jira user mapping
    credentials/       Env var and git config credential resolution
  project/             Cross-repo aggregation, CCPM buffers, capacity
  mermaid/             Gantt and DAG chart generation from JSON
  download/            Binary download utilities
  version/             Version information
  updater/             Auto-update manager
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
**Milestone:** `add`, `list`, `show`, `edit`

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
