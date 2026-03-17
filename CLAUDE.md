# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

For architecture, conventions, build commands, and testing instructions, see [CONTRIBUTORS.md](CONTRIBUTORS.md).

## Project Overview

git-zhi is a git-native task graph that manages development work as a dependency DAG with built-in telemetry. It is a standalone Go binary invoked as `git zhi` (git discovers `git-zhi` on `$PATH`). All state lives in git refs under `refs/zhi/`, requiring no external services. PRDs are at `docs/PRD/` (v0.1, v0.2, v0.3).

## Build & Test

```bash
go build -o git-zhi ./cmd/git-zhi/
go build -o git-zhi-verify ./cmd/git-zhi-verify/
go build -o git-zhi-sanbao ./cmd/git-zhi-sanbao/
go build -o git-zhi-docs ./cmd/git-zhi-docs/
go test ./...
go test -race ./...
```

## Claude-Specific Guidance

- Read [CONTRIBUTORS.md](CONTRIBUTORS.md) for architecture, package structure, invariants, and conventions before making changes.
- Every `.go` file must start with a 2-line `ABOUTME:` comment.
- Follow TDD: write a failing test before implementation.
- No mocks — tests use real on-disk git repos via `t.TempDir()`.
- Default branch is `pu`.
- The ref namespace is `refs/zhi/_/` — issue refs at `refs/zhi/_/issues/<uuid>`, milestones at `refs/zhi/_/milestones/<name>`.
- `issue.RefPrefix` and `milestone.RefPrefix` constants exist — use them instead of string literals.
- `--format` is a persistent flag on the root command. Read it via `cmd.Root().PersistentFlags().GetString("format")`.
- `uuids.ContainsUUID` and `uuids.RemoveUUID` are shared utilities — don't duplicate them.
- `issue.LoadAllIssues(store)` is the shared loader — don't reimplement issue scanning.
- `actor.TypeHuman` and `actor.TypeAgent` constants — use them instead of string literals for actor types.
- `Store.AuthorInfo()` returns git author name/email — use with `actor.DeriveActor()`.
- `Store.DiffNameOnly(from, to)` returns changed files between two SHAs.
- Issue `Transitions` field records state changes with actor identity — append on every state change.
- Issue `ObservedPaths` field records files touched during sessions — set at `--state done`.
- Milestone format is now YAML frontmatter + markdown body (backward-compatible with pure YAML).
- `graph.Head(actor)` accepts optional actor string for per-worker HEAD resolution.
- Plugin packages live under `internal/verify/`, `internal/sanbao/`, `internal/docs/`, `internal/lineage/`.
