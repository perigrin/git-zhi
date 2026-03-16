# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

For architecture, conventions, build commands, and testing instructions, see [CONTRIBUTORS.md](CONTRIBUTORS.md).

## Project Overview

git-zhi is a git-native task graph that manages development work as a dependency DAG with built-in telemetry. It is a standalone Go binary invoked as `git zhi` (git discovers `git-zhi` on `$PATH`). All state lives in git refs under `refs/zhi/`, requiring no external services. PRDs are at `docs/PRD/` (v0.1, v0.2).

## Build & Test

```bash
go build -o git-zhi ./cmd/git-zhi/
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
