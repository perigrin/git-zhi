# 0001. Record architecture decisions

Date: 2026-09-19

## Status

Accepted

## Context

git-zhi has `docs/PRD/` and `docs/plans/` — eleven requirement documents and
seventeen plans. Both describe what the project intends to do. Neither records
why a particular mechanism was chosen over the alternatives that were live at
the time, so a decision's reasoning survives only in commit messages, or not
at all.

That gap has a cost the project has already paid. The postmortem was specified
as a `postmortem.md` blob on the milestone ref and shipped as a frontmatter
field instead; nothing recorded the change, so the specification and the code
disagreed silently until someone read both. The reason was good — `WriteEntity`
builds a tree with exactly one entry, so a second file would have displaced
`milestone.yaml` — but the reason lived nowhere.

A decision log is also what lets another project state a requirement without
specifying its mechanism: the requirement arrives, the decision about how to
meet it is recorded here, and the two stay distinguishable.

## Decision

Architecture decisions are recorded as ADRs in `docs/decisions/`, numbered
sequentially from 0001, in the format described by Michael Nygard in
"Documenting Architecture Decisions".

Decisions are append-only. A decision that no longer holds is superseded by a
new one that says so and links back; it is never edited into agreement with
the present or deleted. `git zhi docs check` validates that the numbering has
no gaps, and treats this directory as exempt from the reachability check —
ADRs are indexed by their numbering, not by links from `CONTRIBUTING.md`.

An ADR is warranted when a choice constrains future work, when a reasonable
alternative was rejected for a reason worth remembering, or when the
implementation diverges from what a PRD or plan asserts. Routine
implementation choices do not need one.

## Consequences

Decisions become reviewable as decisions, separately from the code that
implements them, and a superseded one remains readable alongside its successor
rather than vanishing.

The cost is discipline: an ADR written after the fact is a reconstruction, and
reconstructions are less honest than notes taken at the time. The numbering is
also a shared sequence, so two branches adding decisions concurrently will
collide on a number and one will have to renumber before merging.
