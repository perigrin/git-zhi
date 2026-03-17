# Contributing to git-zhi

## Build & Run

Requires **Go 1.24+** and **git**.

```bash
go build -o git-zhi ./cmd/git-zhi/
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
internal/
  cli/                 Cobra commands, App struct, test helpers
  storage/             git ref CRUD (blob/tree/commit/ref operations)
  issue/               Issue domain model, parse/marshal, sections, loader
  milestone/           Milestone domain model, loader
  graph/               DAG, invariants, critical chain, topo sort
  telemetry/           MPG, speed, buffer, fever chart, time-in-chain
  config/              Chain configuration
  resolve/             Ref argument resolution (HEAD, tag, UUID prefix, title)
  uuids/               Shared UUID slice utilities
```

### Storage Model

State is stored as per-entity git refs under `refs/zhi/`. Each entity gets
its own ref whose commit history is an append-only event log. Current state
is derived by reading the latest commit. Events never delete information —
they only add facts (two-phase set pattern for dependencies).

### Core Domain Concepts

- **Chain**: The dependency DAG of issues. Default chain is `_`.
- **Issue**: A node in the DAG. UUIDv7 identity. States: pending, in-progress, done, cancelled.
- **Milestone**: A delivery grouping with optional due date and buffer.
- **Critical Chain**: Longest sequential path — determines what blocks delivery.
- **Buffer**: Absorbs variance. Starts at 50% of issue count, refines via telemetry.
- **HEAD**: Current in-progress issue, or next on critical chain.

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

### Issue Identity

UUIDv7 — time-ordered, globally unique. Displayed truncated (8 chars).
Resolvable by UUID prefix, tag name, or unambiguous title substring.

### Subcommand Discovery

Any executable named `git-zhi-<name>` on `$PATH` is invocable as `git zhi <name>`.

## Implementation Phases

- **v0.1** — Core: storage, graph, all 10 commands, telemetry, sync, lazy init (done)
- **v0.2** — Agentic: milestone resolution commands, validation, autonomous agent loop
- **v0.3** — Multi-chain, garbage collection
- **v0.4** — Multi-player, external tracker sync, UI
