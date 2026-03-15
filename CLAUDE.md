# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

git-chain is a git-native task graph that manages development work as a dependency DAG with built-in telemetry. It is a standalone Go binary invoked as `git chain` (git discovers `git-chain` on `$PATH`). All state lives in git refs under `refs/chain/`, requiring no external services. The full PRD is at `docs/PRD.md`.

## Build & Run

```bash
go build -o git-chain .
go install github.com/perigrin/git-chain@latest
```

Requires **Go 1.22+** and **git** at runtime.

## Testing

```bash
go test ./...                    # run all tests
go test ./internal/graph/...     # run tests for a specific package
go test -run TestCycleDetection  # run a single test by name
go test -v ./...                 # verbose output
go test -race ./...              # with race detector
```

## Architecture

### Storage Model: Event-Sourced Per-Entity Refs

State is stored as per-entity git refs under `refs/chain/`. Each issue and milestone gets its own ref whose commit history is an append-only event log. Current state is **derived** by reading the latest commit on each ref. Events never delete information — they only add facts (two-phase set pattern for dependencies).

```
refs/chain/
  _/                          # default chain (named "_")
    config                    # chain configuration
    issues/<uuidv7>           # one ref per issue, commit chain = audit trail
    milestones/<name>         # one ref per milestone
    tags/<name>               # lightweight named references to entities
```

Issues are markdown files with YAML frontmatter (`issue.md` blob inside each commit). Milestones are pure YAML. Reading state: `git show refs/chain/_/issues/<id>:issue.md`.

### Core Domain Concepts

- **Chain**: The dependency DAG of issues. Default chain is `_`.
- **Issue**: A node in the DAG. UUIDv7 identity. States: pending → in-progress → done/cancelled.
- **Milestone**: A delivery grouping of issues with optional due date and buffer.
- **Critical Chain**: The longest sequential path through the DAG — determines what's actually blocking delivery.
- **Buffer**: Absorbs variance. Starts at 50% of milestone issue count, refines dynamically via telemetry.
- **HEAD**: Built-in tag resolving to current in-progress issue, or next issue on critical chain by downstream dependency count.

### Telemetry (Observe, Don't Estimate)

Observed signals (commits, elapsed time, completions) drive derived indicators:
- **MPG** — rolling average commits per issue (are issues right-sized?)
- **Speed** — issues completed per calendar period (when will you finish?)
- **Fever chart** — % progress vs % buffer consumed (GREEN/YELLOW/RED)

### Graph Invariants (enforced on every write)

1. Acyclic — no cycles allowed
2. Referential integrity — no dangling dependency edges
3. Done/cancelled issues cannot gain new incoming dependencies
4. Split issues inherit downstream dependencies
5. Cancelled issues reconnect the graph

### Subcommand Discovery

Following git's convention, any executable named `git-chain-<name>` on `$PATH` is invocable as `git chain <name>`.

### CLI Commands (10 total)

**Top-level:** `list`, `config`
**Issue:** `add`, `list`, `show`, `edit` (includes --state, --split, --merge, --purge, --tag)
**Milestone:** `add`, `list`, `show`, `edit`
**Shortcut:** `next` (alias for `issue show HEAD`)

All commands support `--format json` for machine-readable output.

### Issue Identity

UUIDv7 — time-ordered, globally unique, stable across rebases. Displayed truncated (8 chars). Resolvable by UUID prefix, tag name, or unambiguous title substring.

## Implementation Phases

- **v0.1** — Core: storage, graph, all 10 commands, telemetry, sync, lazy init
- **v0.2** — Agentic: milestone resolution commands, validation, autonomous agent loop
- **v0.3** — Multi-chain, garbage collection
- **v0.4** — Multi-player, external tracker sync, UI

## Key Design Decisions

- **Per-entity refs** over single-file state: eliminates merge conflicts between concurrent edits to different issues
- **UUIDv7** over git OIDs or sequential numbers: stable across rebases, collision-free without coordination
- **Lazy init** over explicit `init` command: first mutating command bootstraps everything
- **Events as commits** rather than separate event records: the event type is implicit in the diff between commits on each entity ref
- **Markdown+YAML frontmatter** for issues (human-readable), **pure YAML** for milestones (metadata-only)
- **Critical Chain scheduling** (Goldratt): safety pooled into shared buffer rather than padded per-task
