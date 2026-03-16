# Issue 4: `issue list` — Design

## Scope

List all open issues sorted by creation time (UUIDv7 lexicographic order). Filter by `--milestone` and `--state`. Human-readable table and `--format json` output.

## Implementation

Load all issue refs via `store.ListRefs("refs/zhi/_/issues/")`, read and parse each, apply filters, sort by UUID (creation time), display.

**Human output** (matches PRD):
```
  019444a1  Implement lexer                 ● in-progress
  019444a2  Parse basic sub declarations     ○ pending (blocked by 019444a1)
  019444a3  Parse signatures                 ○ pending
```

State icons: `●` in-progress, `○` pending, `✓` done, `✗` cancelled.
Blocking info shown inline for pending issues with dependencies.

**JSON output**: Array of Issue objects (using existing json tags on Issue struct).

**Flags**:
- `--all` — include done/cancelled issues (default: only pending + in-progress)
- `--milestone <name>` — filter to specific milestone
- `--state <state>` — filter by state
- `--format json` — machine-readable output

## Files

- Create: `internal/cli/issue_list.go` — runIssueList, human/JSON formatters
- Create: `internal/cli/issue_list_test.go` — 5 end-to-end tests
- Modify: `internal/cli/issue.go` — wire list stub to real impl

## Testing

- `TestIssueList_Default` — create 3 issues (pending, in-progress, done), verify only pending+in-progress shown
- `TestIssueList_All` — same setup with `--all`, verify all 3 shown
- `TestIssueList_FilterMilestone` — 2 milestones, verify filter works
- `TestIssueList_FilterState` — verify `--state pending` shows only pending
- `TestIssueList_JsonOutput` — verify JSON array output
