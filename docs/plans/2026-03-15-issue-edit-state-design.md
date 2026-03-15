# Issue 5: `issue edit --state` — Design

## Scope

State machine validation and measurement bookmarks. The `--state` flag on `issue edit` with values: `start`, `pause`, `resume`, `done`, `cancel`.

## State Machine

| `--state` value | From state | To state | Measurement |
|----------------|------------|----------|-------------|
| `start` | pending | in-progress | Opens new session (records HEAD SHA) |
| `pause` | in-progress | in-progress | Closes current session (records HEAD SHA, counts commits) |
| `resume` | in-progress (paused) | in-progress | Opens new session (records HEAD SHA) |
| `done` | in-progress | done | Closes current session (records HEAD SHA, counts commits) |
| `cancel` | pending or in-progress | cancelled | No measurement change |

Invalid transitions return a clear error (e.g., "cannot start issue: state is in-progress, not pending").

## Measurement Bookmarks

Each `start`/`resume` opens a measurement session recording the repo's HEAD SHA. Each `pause`/`done` closes the session, recording the end SHA. The commit count between start and end is computed by walking the git commit log between the two SHAs.

Sessions are stored in the Issue struct's `Sessions []Session` field and persisted in YAML frontmatter.

## Commit Counting

To count commits between two SHAs, we add a method to Store:

`CountCommits(startSHA, endSHA string) (int, error)` — walks the commit log from endSHA backward until startSHA is reached, counting commits. Uses go-git's log iterator.

## Files

- Create: `internal/issue/state.go` — ValidateTransition, state machine rules
- Create: `internal/issue/state_test.go` — transition validation tests
- Create: `internal/cli/issue_edit.go` — runIssueEdit with --state handling
- Create: `internal/cli/issue_edit_test.go` — end-to-end state transition tests
- Modify: `internal/cli/issue.go` — wire edit stub to real impl
- Modify: `internal/storage/store.go` — add CountCommits and RepoHEAD methods
- Modify: `internal/storage/store_test.go` — tests for CountCommits and RepoHEAD

## Testing

**State machine** (6 tests): Valid transitions (start, pause, resume, done, cancel), invalid transition (start on in-progress).

**Storage** (2 tests): CountCommits between known SHAs, RepoHEAD returns current HEAD.

**CLI** (5 tests): Start sets in-progress + opens session, pause closes session with commit count, resume opens new session, done completes issue, cancel from pending.
