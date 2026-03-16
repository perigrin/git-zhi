# Issue 6: `issue edit` (remaining flags) — Design

## Scope

Non-interactive edit flags for `issue edit`: `--block`, `--unblock`, `--milestone`, `--tag`, `--untag`, `--before`, `--after`. Interactive flags (`--split`, `--merge`, `--purge`, bare edit with $EDITOR) return "not yet implemented" — they require interactive prompts that are out of scope for the non-interactive-first approach.

## Flags Implemented

| Flag | Effect | Implementation |
|------|--------|----------------|
| `--block <ref>` | Add forward dependency (this blocks ref) | Resolve ref, add to Blocks/BlockedBy on both issues |
| `--unblock <ref>` | Remove forward dependency | Resolve ref, remove from Blocks/BlockedBy on both issues |
| `--milestone <name>` | Move to different milestone | Update Milestone field |
| `--tag <name>` | Add tag | Create tag ref pointing at issue |
| `--untag <name>` | Remove tag | Delete tag ref |
| `--before <ref>` | Position before another issue (this blocks ref) | Add blocks edge: this.Blocks += ref, ref.BlockedBy += this |
| `--after <ref>` | Position after another issue (ref blocks this) | Add blocked_by edge: this.BlockedBy += ref, ref.Blocks += this |

## Deferred (return "not yet implemented")

- `--split` — requires $EDITOR
- `--merge <ref>` — requires dependency rewiring + measurement combining
- `--purge` — requires confirmation prompt
- Bare `issue edit <ref>` without flags — requires $EDITOR

## Tags

Tags are lightweight refs: `refs/zhi/_/tags/<name>` stores a pointer to `refs/zhi/_/issues/<uuid>`. The Store already supports this via WriteEntity/RefExists. Tag content is a simple text file containing the target ref path.

## Files

- Modify: `internal/cli/issue_edit.go` — add flag handling for block/unblock/milestone/tag/untag/before/after
- Create: `internal/cli/issue_edit_flags_test.go` — tests for each flag
- Modify: `internal/storage/store.go` — add DeleteRef method (for --untag)

## Testing

- `TestIssueEdit_Block` — block another issue, verify both issues updated
- `TestIssueEdit_Unblock` — unblock, verify edges removed from both
- `TestIssueEdit_Milestone` — change milestone, verify persisted
- `TestIssueEdit_Tag` — tag an issue, verify tag ref created
- `TestIssueEdit_Untag` — untag, verify tag ref removed
- `TestIssueEdit_Before` — position before, verify blocked_by added
- `TestIssueEdit_After` — position after, verify blocks added
