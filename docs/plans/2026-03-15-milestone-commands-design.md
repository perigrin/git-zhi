# Issue 8: Milestone Commands — Design

## Scope

`milestone add`, `milestone list`, `milestone show`, `milestone edit`. Milestone domain model already exists (Milestone struct, MarshalMilestone). Storage read/write already works. This issue wires up the four CLI commands.

## Commands

### `milestone add <name>`
- Create a milestone with the given name
- `--due <date>` flag (YYYY-MM-DD)
- Write to `refs/zhi/_/milestones/<name>`
- Error if milestone already exists

### `milestone list`
- List all milestones with issue counts and buffer status
- Show current milestone (first with pending issues)
- `--format json`

### `milestone show <ref>`
- Show milestone detail: name, due date, description, issue list with states
- Fever chart display (placeholder — telemetry is issue 10)
- `--format json`

### `milestone edit <ref>`
- `--due <date>` — set/update due date (`--due none` to clear)
- `--name <name>` — rename
- `--tag <name>` / `--untag <name>`

## Files

- Create: `internal/cli/milestone_add.go` + test
- Create: `internal/cli/milestone_list.go` + test
- Create: `internal/cli/milestone_show.go` + test
- Create: `internal/cli/milestone_edit.go` + test
- Modify: `internal/cli/milestone.go` — wire stubs to real impls
- Create: `internal/milestone/load.go` — LoadMilestone, LoadAllMilestones helpers
- Modify: `internal/milestone/milestone.go` — add UnmarshalMilestone

## Testing

- milestone add: create, duplicate error, with --due
- milestone list: multiple milestones with issue counts
- milestone show: detail display, JSON output
- milestone edit: --due, --name
