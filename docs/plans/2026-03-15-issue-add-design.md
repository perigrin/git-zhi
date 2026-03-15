# Issue 2: `issue add` — Design

## Scope

First end-to-end vertical slice: storage write/read path, issue parse/marshal, UUIDv7 generation, lazy init with default milestone, and the `issue add` command reading from stdin with `---` batch support.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Frontmatter parsing | adrg/frontmatter | Small, focused library for YAML/body splitting |
| YAML library | goccy/go-yaml | Selected in v0.1 design; fast, good error messages |
| Commit author | Read from git config | Matches user's normal git identity |
| Batch creation | Included in this issue | PRD workflow; not covered by later issues |
| Test repos | t.TempDir() + git.PlainInit | Real on-disk repos; in-memory differs in ref behavior |

## Storage Layer

Four methods on `Store`, operating on per-entity refs:

- `WriteEntity(refPath, filename string, content []byte, message string) error` — creates blob, tree, commit, updates ref. Appends to existing ref if it exists (parent = current commit).
- `ReadEntity(refPath, filename string) ([]byte, error)` — reads named file from latest commit on ref.
- `ListRefs(prefix string) ([]string, error)` — returns all ref names under a prefix.
- `RefExists(refPath string) bool` — checks whether a ref exists.

Commit author read from git config (`user.name`, `user.email`) at Store construction time, stored as a field.

## Issue Parse/Marshal

- `Parse(raw []byte) (*Issue, error)` — uses `adrg/frontmatter` to split YAML from body, `goccy/go-yaml` for YAML unmarshaling. ID is not in frontmatter; caller sets it from the ref path.
- `Marshal(iss *Issue) ([]byte, error)` — serializes struct to YAML frontmatter + `---` + body.
- `SplitBatch(raw []byte) [][]byte` — splits multi-issue input on `\n---\n` boundaries, preserving each block's frontmatter.

## Lazy Init

Triggered by mutating commands. `App.EnsureInitialized()` checks for `refs/chain/_/config`:

1. If exists, return early
2. Write default config to `refs/chain/_/config`
3. Write default milestone to `refs/chain/_/milestones/v0.1`
4. Configure refspecs for `refs/chain/*` sync (if remote exists)

Repo opening is lazy — `PersistentPreRunE` defers opening until first command that needs it, so `--help` always works regardless of current directory.

## `issue add` Command Flow

1. Get App, call EnsureInitialized()
2. Read all stdin
3. SplitBatch → individual blocks
4. For each block: Parse, generate UUIDv7, set defaults (state=pending, milestone from config), Marshal, WriteEntity
5. Wire sequential dependencies for batch creates (N blocks N+1)
6. Output results (human-readable or JSON)

Flags: `--after`, `--before` (return "not yet implemented" until resolve package in issue 10), `--milestone` (overrides default).

## Testing

**Storage** (6 tests): WriteEntity creates/appends refs, ReadEntity round-trips, ReadEntity error on missing ref, ListRefs returns all under prefix, RefExists for existing and missing.

**Issue** (7 tests): Parse valid/minimal/invalid YAML, Marshal round-trip, SplitBatch single/multiple/empty.

**CLI** (5 tests): Single issue add, batch add with dependencies, JSON output, lazy init on fresh repo, default milestone applied. Each test uses a real temporary git repo.
