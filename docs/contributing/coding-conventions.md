---
stability: 2
covers:
  - internal
  - cmd
---

## File Headers

Every `.go` file opens with a two-line comment, both lines prefixed
`ABOUTME: `, saying what the file does. It is greppable on purpose.

## Code Style

Match the surrounding code. Consistency within a file matters more than any
external style guide. Prefer the simple, readable solution over the clever one:
correctness first, performance second — a correct implementation can be made
faster, and a fast one cannot always be made correct.

Build with `make build`. The Makefile is the only place version, commit and
build time are defined, and the release workflow calls it too.

## Naming

Names are evergreen. Nothing is called `new`, `improved` or `enhanced` —
what is new today is old later, and the name outlives the comparison.

## Shared Helpers

Reach for what exists rather than re-deriving it:

- `issue.LoadAllIssues(store)` is the shared issue loader.
- `issue.RefPrefix` and `milestone.RefPrefix` name the ref namespaces; do not
  write the string literals.
- `actor.TypeHuman` and `actor.TypeAgent` are the actor-type constants.
- `slices.Contains` and `slices.DeleteFunc` for UUID slice work.
- `issue.ValidateLabelName(name)` before storing any label.

## Error Handling

Return errors, do not panic. Wrap with context: `fmt.Errorf("read config: %w", err)`.

An error a caller can act on beats one that only reports failure. Where a
command documents an override, make sure the failure path actually reaches
the check that override guards — a documented escape hatch behind an earlier
fatal return is worse than none.

## Filters and Derived Views

The graph is built from every issue on purpose, so that blockers outside a
filter still constrain readiness. Anything derived from the graph — a
topological sort, a ready set — therefore comes back unfiltered, and every
display filter has to be re-applied to it. A filter computed and not
re-applied is silently ignored.

## Testing

Write the failing test first, run it, and read the failure text — not just
the FAIL line. A test that never ran produces no error at all, and an
assertion that only knows how to fail cannot tell a broken subject from an
unreachable one. Pair every guard that fails today with one that passes
today.

Tests use real data: real on-disk git repos via `t.TempDir()`, never mocks.
